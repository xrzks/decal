package cli

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/charmbracelet/huh"

	"templ/internal/catalog"
)

// huhPrompter drives the interactive Charmbracelet Huh forms.
type huhPrompter struct{}

func (huhPrompter) ChooseCategory(ctx context.Context, categories []catalog.Category) (catalog.Category, error) {
	byDir := make(map[string]catalog.Category, len(categories))
	options := make([]huh.Option[string], 0, len(categories))
	for _, category := range categories {
		byDir[category.Dir] = category
		options = append(options, huh.NewOption(category.Name, category.Dir))
	}

	var chosen string
	form := huh.NewForm(huh.NewGroup(
		huh.NewSelect[string]().
			Title("Choose a category").
			Options(options...).
			Value(&chosen),
	)).WithShowHelp(false)
	if err := form.RunWithContext(ctx); err != nil {
		return catalog.Category{}, promptError("category selection", err)
	}

	category, ok := byDir[chosen]
	if !ok {
		return catalog.Category{}, errors.New("category selection failed: no category was selected")
	}
	return category, nil
}

func (huhPrompter) ChooseTemplate(ctx context.Context, templates []*catalog.Template) (*catalog.Template, error) {
	byPath := make(map[string]*catalog.Template, len(templates))
	options := make([]huh.Option[string], 0, len(templates))
	for _, template := range templates {
		byPath[template.Path] = template
		options = append(options, huh.NewOption(template.Name, template.Path))
	}

	var chosen string
	form := huh.NewForm(huh.NewGroup(
		huh.NewSelect[string]().
			Title("Choose a template").
			Options(options...).
			Value(&chosen),
	)).WithShowHelp(false)
	if err := form.RunWithContext(ctx); err != nil {
		return nil, promptError("template selection", err)
	}

	template, ok := byPath[chosen]
	if !ok {
		return nil, errors.New("template selection failed: no template was selected")
	}
	return template, nil
}

func (huhPrompter) Form(ctx context.Context, template *catalog.Template) (map[string]any, error) {
	answers := make(map[string]any, len(template.Fields))
	groups := make([]*huh.Group, 0, len(template.Fields))
	var collect []func()

	for _, field := range template.Fields {
		field := field
		switch field.Type {
		case catalog.FieldInput:
			value := field.DefaultString()
			groups = append(groups, huh.NewGroup(huh.NewInput().
				Title(field.Label).
				Value(&value).
				Validate(requiredString(field))))
			collect = append(collect, func() { answers[field.Key] = value })

		case catalog.FieldText:
			value := field.DefaultString()
			groups = append(groups, huh.NewGroup(huh.NewText().
				Title(field.Label).
				Lines(5).
				Value(&value).
				Validate(requiredString(field))))
			collect = append(collect, func() { answers[field.Key] = value })

		case catalog.FieldSelect:
			value := field.DefaultString()
			if value == "" && len(field.Options) > 0 {
				value = field.Options[0].Value
			}
			groups = append(groups, huh.NewGroup(huh.NewSelect[string]().
				Title(field.Label).
				Options(selectOptions(field.Options)...).
				Value(&value)))
			collect = append(collect, func() { answers[field.Key] = value })

		case catalog.FieldMultiSelect:
			value := field.DefaultStrings()
			if value == nil {
				value = []string{}
			}
			groups = append(groups, huh.NewGroup(huh.NewMultiSelect[string]().
				Title(field.Label).
				Options(selectOptions(field.Options)...).
				Value(&value).
				Validate(requiredSelection(field))))
			collect = append(collect, func() { answers[field.Key] = value })

		case catalog.FieldConfirm:
			value := field.DefaultBool()
			groups = append(groups, huh.NewGroup(huh.NewConfirm().
				Title(field.Label).
				Value(&value)))
			collect = append(collect, func() { answers[field.Key] = value })

		default:
			return nil, fmt.Errorf("template %s: unsupported field type %q", template.Path, field.Type)
		}
	}

	if err := huh.NewForm(groups...).WithShowHelp(false).RunWithContext(ctx); err != nil {
		return nil, promptError("form", err)
	}

	for _, collectAnswers := range collect {
		collectAnswers()
	}
	return answers, nil
}

func selectOptions(options []catalog.Option) []huh.Option[string] {
	huhOptions := make([]huh.Option[string], 0, len(options))
	for _, option := range options {
		huhOptions = append(huhOptions, huh.NewOption(option.Label, option.Value))
	}
	return huhOptions
}

func requiredString(field catalog.Field) func(string) error {
	return func(value string) error {
		if field.Required && strings.TrimSpace(value) == "" {
			return errors.New("a value is required")
		}
		return nil
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
	return fmt.Errorf("%s failed: %w", what, err)
}
