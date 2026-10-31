// Package cli wires template discovery, prompts, rendering and file creation
// into the templ command.
package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/urfave/cli/v3"

	"templ/internal/catalog"
	"templ/internal/generate"
)

// ErrCancelled reports that the user aborted a prompt.
var ErrCancelled = errors.New("cancelled")

// ErrNoTerminal reports that the prompts cannot run without a terminal.
var ErrNoTerminal = errors.New("templ needs an interactive terminal; set TERM=dumb to answer prompts from stdin")

// Prompter collects a category, a template and a template's form answers.
type Prompter interface {
	ChooseCategory(ctx context.Context, categories []catalog.Category) (catalog.Category, error)
	ChooseTemplate(ctx context.Context, templates []*catalog.Template) (*catalog.Template, error)
	Form(ctx context.Context, template *catalog.Template) (map[string]any, error)
}

// New builds the templ command.
func New() *cli.Command {
	return &cli.Command{
		Name:  "templ",
		Usage: "Generate a file from a reusable template.",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:  "config-dir",
				Usage: "template root directory (default: $XDG_CONFIG_HOME/templ or ~/.config/templ)",
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			root, err := catalog.Root(cmd.String("config-dir"))
			if err != nil {
				return err
			}
			dir, err := os.Getwd()
			if err != nil {
				return fmt.Errorf("cannot determine the working directory: %w", err)
			}

			out := cmd.Root().Writer
			err = Run(ctx, root, huhPrompter{}, dir, out)
			if errors.Is(err, ErrCancelled) {
				fmt.Fprintln(out, "Cancelled. No file was created.")
				return nil
			}
			return err
		},
	}
}

// Run drives the whole workflow: discover, prompt, render, validate, create.
func Run(ctx context.Context, root string, prompter Prompter, dir string, out io.Writer) error {
	categories, err := catalog.Load(root)
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
	if template == nil {
		return fmt.Errorf("no template selected in category %q", category.Name)
	}

	answers := map[string]any{}
	if len(template.Fields) > 0 {
		if answers, err = prompter.Form(ctx, template); err != nil {
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
