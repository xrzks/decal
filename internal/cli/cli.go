// Package cli wires template discovery, prompts, rendering and file creation
// into the decal command.
package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/urfave/cli/v3"

	"github.com/xrzks/decal/internal/catalog"
	"github.com/xrzks/decal/internal/generate"
)

// ErrCancelled reports that the user aborted a prompt.
var ErrCancelled = errors.New("cancelled")

// ExitCancelled is the exit status after a cancelled run: 128 + SIGINT, the
// conventional status for a command interrupted with ctrl+c.
const ExitCancelled = 130

// ErrNoTerminal reports that the prompts cannot run without a terminal.
var ErrNoTerminal = errors.New("decal needs an interactive terminal on stdin and stderr; set TERM=dumb to answer prompts from stdin")

// Prompter collects a category, a template and a template's form answers.
type Prompter interface {
	ChooseCategory(ctx context.Context, categories []catalog.Category) (catalog.Category, error)
	ChooseTemplate(ctx context.Context, templates []*catalog.Template) (*catalog.Template, error)
	// Form asks for the template's answers. checkOutput reports why a set of
	// answers names an output file that cannot be created, so the form can
	// flag it while the user can still change those answers.
	Form(ctx context.Context, template *catalog.Template, checkOutput func(answers map[string]any) error) (map[string]any, error)
}

// New builds the decal command. version is reported by --version.
func New(version string) *cli.Command {
	return &cli.Command{
		Name:    "decal",
		Usage:   "generate a file from a reusable template",
		Version: version,
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:  "config-dir",
				Usage: "template root directory (default: $XDG_CONFIG_HOME/decal or ~/.config/decal)",
			},
		},
		// Return usage errors instead of printing them with the full help; main
		// prints the returned error, so this keeps it to a single line.
		OnUsageError: func(_ context.Context, _ *cli.Command, err error, _ bool) error {
			return fmt.Errorf("%w (see decal --help)", err)
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			if cmd.Args().Present() {
				return fmt.Errorf("unexpected argument %q (see decal --help)", cmd.Args().First())
			}
			root, err := catalog.Root(cmd.String("config-dir"))
			if err != nil {
				return err
			}
			dir, err := os.Getwd()
			if err != nil {
				return fmt.Errorf("cannot determine the working directory: %w", err)
			}

			err = Run(ctx, root, huhPrompter{}, dir, cmd.Root().Writer, cmd.Root().ErrWriter)
			if errors.Is(err, ErrCancelled) {
				// Report it here; the caller only maps it to ExitCancelled.
				fmt.Fprintln(cmd.Root().ErrWriter, "Cancelled. No file was created.")
			}
			return err
		},
	}
}

// Run drives the whole workflow: discover, prompt, render, validate, create.
// The created path goes to out; templates skipped as broken go to errOut.
func Run(ctx context.Context, root string, prompter Prompter, dir string, out, errOut io.Writer) error {
	categories, problems, err := catalog.Load(root)
	for _, problem := range problems {
		fmt.Fprintf(errOut, "warning: skipped %v\n", problem)
	}
	if err != nil {
		return err
	}

	category, err := prompter.ChooseCategory(ctx, categories)
	if err != nil {
		return err
	}

	template, err := prompter.ChooseTemplate(ctx, category.Templates)
	if err != nil {
		return err
	}

	// A fixed output name can be checked before the form, so a clash does not
	// cost the user their answers.
	if !strings.Contains(template.Output, "{{") {
		if err := generate.Check(dir, template.Output); err != nil {
			return fmt.Errorf("template %s: %w", template.Path, err)
		}
	}

	answers := map[string]any{}
	if len(template.Fields) > 0 {
		checkOutput := func(answers map[string]any) error { return checkOutputName(template, answers, dir) }
		if answers, err = prompter.Form(ctx, template, checkOutput); err != nil {
			return err
		}
	}

	path, err := produce(template, answers, dir)
	if err != nil {
		return fmt.Errorf("template %s: %w", template.Path, err)
	}

	fmt.Fprintf(out, "Created %s\n", path)
	return nil
}

// checkOutputName reports why answers render to an output name that cannot be
// created in dir. Rendering errors are left to produce: no answer fixes them.
func checkOutputName(template *catalog.Template, answers map[string]any, dir string) error {
	name, err := renderPart("output filename", template.Output, answers)
	if err != nil {
		return nil
	}
	if err := generate.Check(dir, name); err != nil {
		if errors.Is(err, generate.ErrExists) {
			return fmt.Errorf("%s already exists here", name)
		}
		return err
	}
	return nil
}

// produce renders a template and writes its output file. Both parts are
// rendered in full before the destination is touched; generate.Write validates
// the resulting name and never overwrites.
func produce(template *catalog.Template, answers map[string]any, dir string) (string, error) {
	name, err := renderPart("output filename", template.Output, answers)
	if err != nil {
		return "", err
	}
	body, err := renderPart("body", template.Body, answers)
	if err != nil {
		return "", err
	}
	return generate.Write(dir, name, []byte(body))
}
