package cli

import (
	"bufio"
	"cmp"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"

	"charm.land/huh/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/colorprofile"
	"github.com/charmbracelet/x/term"

	"templ/internal/catalog"
)

// huhPrompter drives the interactive Charmbracelet Huh forms.
type huhPrompter struct{}

func (huhPrompter) ChooseCategory(ctx context.Context, categories []catalog.Category) (catalog.Category, error) {
	return choose(ctx, "category selection", "Choose a category", categories,
		func(c catalog.Category) string { return c.Name },
		func(c catalog.Category) string { return c.Dir })
}

func (huhPrompter) ChooseTemplate(ctx context.Context, templates []*catalog.Template) (*catalog.Template, error) {
	return choose(ctx, "template selection", "Choose a template", templates,
		func(t *catalog.Template) string { return t.Name },
		func(t *catalog.Template) string { return t.Path })
}

// terminalTheme styles forms with the terminal's own 16-colour palette, so the
// prompts follow whatever colour scheme the user runs. Colours are ANSI indices
// rather than hex values, and body text keeps the terminal's default foreground.
func terminalTheme(isDark bool) *huh.Styles {
	s := huh.ThemeBase(isDark)
	accent := lipgloss.Color("4")    // blue
	highlight := lipgloss.Color("6") // cyan
	dim := lipgloss.Color("8")       // bright black
	failure := lipgloss.Color("1")   // red

	s.Focused.Base = s.Focused.Base.BorderForeground(dim)
	s.Focused.Card = s.Focused.Base
	s.Focused.Title = s.Focused.Title.Foreground(accent).Bold(true)
	s.Focused.NoteTitle = s.Focused.NoteTitle.Foreground(accent).Bold(true)
	s.Focused.Description = s.Focused.Description.Foreground(dim)
	s.Focused.SelectSelector = s.Focused.SelectSelector.Foreground(highlight)
	s.Focused.MultiSelectSelector = s.Focused.MultiSelectSelector.Foreground(highlight)
	s.Focused.NextIndicator = s.Focused.NextIndicator.Foreground(highlight)
	s.Focused.PrevIndicator = s.Focused.PrevIndicator.Foreground(highlight)
	s.Focused.SelectedOption = s.Focused.SelectedOption.Foreground(highlight)
	s.Focused.SelectedPrefix = lipgloss.NewStyle().Foreground(highlight).SetString("✓ ")
	s.Focused.UnselectedPrefix = lipgloss.NewStyle().Foreground(dim).SetString("• ")
	s.Focused.ErrorIndicator = s.Focused.ErrorIndicator.Foreground(failure)
	s.Focused.ErrorMessage = s.Focused.ErrorMessage.Foreground(failure)
	s.Focused.TextInput.Prompt = s.Focused.TextInput.Prompt.Foreground(highlight)
	s.Focused.TextInput.Placeholder = s.Focused.TextInput.Placeholder.Foreground(dim)

	s.Blurred = s.Focused
	s.Blurred.Base = s.Focused.Base.BorderStyle(lipgloss.HiddenBorder())
	s.Blurred.Card = s.Blurred.Base
	s.Blurred.NextIndicator = lipgloss.NewStyle()
	s.Blurred.PrevIndicator = lipgloss.NewStyle()

	s.Group.Title = s.Focused.Title
	s.Group.Description = s.Focused.Description
	return s
}

// choose runs a single-select prompt over items, showing them in order and
// resolving the answer back to the item whose key was selected. A lone item is
// picked without asking, except in accessible mode, where scripted answers are
// positional and must not shift when a second item is added.
func choose[T any](ctx context.Context, what, title string, items []T, label, key func(T) string) (T, error) {
	if len(items) == 1 && !accessibleMode() {
		return items[0], nil
	}
	var zero T
	byKey := make(map[string]T, len(items))
	options := make([]huh.Option[string], 0, len(items))
	for _, item := range items {
		byKey[key(item)] = item
		options = append(options, huh.NewOption(label(item), key(item)))
	}

	var chosen string
	form := huh.NewForm(huh.NewGroup(
		huh.NewSelect[string]().
			Title(title).
			Options(options...).
			Value(&chosen),
	)).WithShowHelp(false).WithTheme(huh.ThemeFunc(terminalTheme))
	if err := runPrompt(ctx, form, nil); err != nil {
		return zero, promptError(what, err)
	}

	item, ok := byKey[chosen]
	if !ok {
		return zero, fmt.Errorf("%s failed: no selection was made", what)
	}
	return item, nil
}

func (huhPrompter) Form(ctx context.Context, template *catalog.Template, checkOutput func(map[string]any) error) (map[string]any, error) {
	input := &promptInput{Reader: os.Stdin}
	accessible := accessibleMode()
	answers := make(map[string]any, len(template.Fields))
	groups := make([]*huh.Group, 0, len(template.Fields))
	var collect []func()

	// Check the output name on the last field it depends on: every answer it
	// uses is known there, and the user can change that answer in place. Huh
	// does not leave a field that fails validation, so checking earlier fields
	// against later defaults could block a valid choice. Accessible prompts
	// re-read a rejected answer from the next input line, which would shift
	// every later scripted answer, so they leave the check to the final write.
	nameKey := ""
	if !accessible && checkOutput != nil {
		for _, field := range template.Fields {
			if slices.Contains(template.OutputFields, field.Key) {
				nameKey = field.Key
			}
		}
	}
	checkName := func(key string, value any) error {
		if key != nameKey {
			return nil
		}
		for _, collectAnswer := range collect {
			collectAnswer()
		}
		answers[key] = value
		return checkOutput(answers)
	}

	for _, field := range template.Fields {
		switch field.Type {
		case catalog.FieldInput:
			value := field.DefaultString()
			groups = append(groups, huh.NewGroup(huh.NewInput().
				Title(field.Label).
				Value(&value).
				Validate(func(value string) error {
					return cmp.Or(requiredString(field, accessible)(value), checkName(field.Key, value))
				})))
			collect = append(collect, func() { answers[field.Key] = value })

		case catalog.FieldText:
			value := field.DefaultString()
			groups = append(groups, huh.NewGroup(huh.NewText().
				Title(field.Label).
				Lines(5).
				Value(&value).
				Validate(func(value string) error {
					return cmp.Or(requiredString(field, accessible)(value), checkName(field.Key, value))
				})))
			collect = append(collect, func() { answers[field.Key] = value })

		case catalog.FieldSelect:
			value := field.DefaultString()
			if value == "" && len(field.Options) > 0 {
				value = field.Options[0].Value
			}
			groups = append(groups, huh.NewGroup(huh.NewSelect[string]().
				Title(field.Label).
				Options(selectOptions(field.Options)...).
				Value(&value).
				Validate(func(value string) error { return checkName(field.Key, value) })))
			collect = append(collect, func() { answers[field.Key] = value })

		case catalog.FieldMultiSelect:
			value := field.DefaultStrings()
			if value == nil {
				value = []string{}
			}
			var multiSelect huh.Field = huh.NewMultiSelect[string]().
				Title(field.Label).
				Options(selectOptions(field.Options)...).
				Value(&value).
				Validate(func(values []string) error {
					// Let Huh exit its retry loop; runPrompt reports the read error.
					if input.err != nil {
						return nil
					}
					return cmp.Or(requiredSelection(field)(values), checkName(field.Key, values))
				})
			if accessible {
				multiSelect = blankConfirmsMultiSelect{multiSelect}
			}
			groups = append(groups, huh.NewGroup(multiSelect))
			collect = append(collect, func() { answers[field.Key] = value })

		case catalog.FieldConfirm:
			value := field.DefaultBool()
			groups = append(groups, huh.NewGroup(huh.NewConfirm().
				Title(field.Label).
				Value(&value).
				Validate(func(value bool) error { return checkName(field.Key, value) })))
			collect = append(collect, func() { answers[field.Key] = value })

		default:
			return nil, fmt.Errorf("template %s: unsupported field type %q", template.Path, field.Type)
		}
	}

	if err := runPrompt(ctx, huh.NewForm(groups...).WithShowHelp(false).WithTheme(huh.ThemeFunc(terminalTheme)), input); err != nil {
		return nil, promptError("form", err)
	}

	for _, collectAnswers := range collect {
		collectAnswers()
	}
	return answers, nil
}

// accessibleMode reports whether Huh answers prompts by reading lines from
// stdin instead of drawing a full-screen form.
func accessibleMode() bool { return os.Getenv("TERM") == "dumb" }

// Huh's accessible mode swallows read errors, including EOF. Track them so
// defaults are never accepted when input ends before the user submits a prompt.
func runPrompt(ctx context.Context, form *huh.Form, input *promptInput) error {
	if !accessibleMode() {
		// Huh's full-screen mode needs a terminal: without one Bubble Tea's
		// input loop stalls until its two-minute timeout. Prompts are drawn on
		// stderr, keeping stdout for the created path.
		if !term.IsTerminal(os.Stdin.Fd()) || !term.IsTerminal(os.Stderr.Fd()) {
			return ErrNoTerminal
		}
		return form.WithOutput(os.Stderr).RunWithContext(ctx)
	}
	if input == nil {
		input = &promptInput{Reader: os.Stdin}
	}
	err := runAccessible(ctx, form, input)
	if errors.Is(input.err, io.EOF) {
		return huh.ErrUserAborted
	}
	if input.err != nil {
		return input.err
	}
	return err
}

// Huh's accessible Select indexes its options with the integer returned by its
// prompt helper, which is out of range when EOF follows invalid input. Recover
// that panic; runPrompt turns the tracked read error into a cancellation.
func runAccessible(ctx context.Context, form *huh.Form, input *promptInput) (err error) {
	defer func() {
		recovered := recover()
		if recovered == nil {
			return
		}
		if input.err == nil {
			panic(recovered)
		}
		err = input.err
	}()
	// Prompts go to stderr, keeping stdout for the created path. Downsample
	// Huh's styled text so TERM=dumb and pipes get plain text.
	output := colorprofile.NewWriter(os.Stderr, os.Environ())
	return form.WithInput(input).WithOutput(output).RunWithContext(ctx)
}

type promptInput struct {
	io.Reader
	err     error
	lineLen int
}

// maxLineLen matches the buffer bufio.Scanner grows to in Huh's accessible
// prompts. Huh ignores the scanner's error, so a longer line would silently be
// dropped and the field would fall back to its default without validation.
const maxLineLen = bufio.MaxScanTokenSize

var errLineTooLong = fmt.Errorf("input line is longer than the %d-byte limit", maxLineLen)

func (r *promptInput) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	if r.err != nil {
		return 0, r.err
	}
	// Each accessible field uses a fresh scanner. Prevent it from buffering
	// answers belonging to later prompts.
	n, err := r.Reader.Read(p[:1])
	if n > 0 {
		if p[0] == '\n' {
			r.lineLen = 0
		} else if r.lineLen++; r.lineLen >= maxLineLen {
			r.err = errLineTooLong
			return n, nil
		}
	}
	r.err = err
	return n, err
}

// Huh's accessible MultiSelect only confirms on "0" and rejects a blank line,
// unlike every other prompt where a blank line takes the default. Translate a
// blank line into "0" so it confirms the current selection, which starts out
// as the field's default.
type blankConfirmsMultiSelect struct{ huh.Field }

func (m blankConfirmsMultiSelect) RunAccessible(w io.Writer, r io.Reader) error {
	return m.Field.RunAccessible(w, &blankConfirms{Reader: r})
}

type blankConfirms struct {
	io.Reader
	midLine        bool
	pendingNewline bool
}

func (r *blankConfirms) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	if r.pendingNewline {
		r.pendingNewline = false
		p[0] = '\n'
		return 1, nil
	}
	// Read one byte at a time, like promptInput, so nothing beyond the current
	// line is buffered.
	n, err := r.Reader.Read(p[:1])
	if n > 0 {
		if p[0] == '\n' && !r.midLine {
			p[0] = '0'
			r.pendingNewline = true
		} else {
			r.midLine = p[0] != '\n'
		}
	}
	return n, err
}

func selectOptions(options []catalog.Option) []huh.Option[string] {
	huhOptions := make([]huh.Option[string], 0, len(options))
	for _, option := range options {
		huhOptions = append(huhOptions, huh.NewOption(option.Label, option.Value))
	}
	return huhOptions
}

func requiredString(field catalog.Field, accessible bool) func(string) error {
	return func(value string) error {
		if !field.Required || strings.TrimSpace(value) != "" {
			return nil
		}
		// Accessible prompts validate the raw line and only then substitute the
		// default, so a blank answer stands for a non-blank default.
		if accessible && strings.TrimSpace(field.DefaultString()) != "" {
			return nil
		}
		return errors.New("a value is required")
	}
}

func requiredSelection(field catalog.Field) func([]string) error {
	return func(values []string) error {
		if field.Required && len(values) == 0 {
			return errors.New("select at least one option")
		}
		return nil
	}
}

func promptError(what string, err error) error {
	if errors.Is(err, huh.ErrUserAborted) {
		return ErrCancelled
	}
	if errors.Is(err, ErrNoTerminal) {
		return ErrNoTerminal
	}
	return fmt.Errorf("%s failed: %w", what, err)
}
