// Package render executes Go text templates for template bodies and output
// filenames.
package render

import (
	"fmt"
	"strings"
	"text/template"
)

// Part identifies which part of a template failed to render.
type Part string

const (
	OutputPart Part = "output filename"
	BodyPart   Part = "body"
)

// Error reports which part of a template failed.
type Error struct {
	Part Part
	Err  error
}

func (e *Error) Error() string { return fmt.Sprintf("%s: %v", e.Part, e.Err) }

func (e *Error) Unwrap() error { return e.Err }

// OutputName renders a template's output filename.
func OutputName(source string, answers map[string]any) (string, error) {
	return render(OutputPart, source, answers)
}

// Body renders a template's body.
func Body(source string, answers map[string]any) (string, error) {
	return render(BodyPart, source, answers)
}

func render(part Part, source string, answers map[string]any) (string, error) {
	tmpl, err := template.New(string(part)).Option("missingkey=error").Parse(source)
	if err != nil {
		return "", &Error{Part: part, Err: fmt.Errorf("cannot parse template: %w", err)}
	}

	var out strings.Builder
	if err := tmpl.Execute(&out, answers); err != nil {
		return "", &Error{Part: part, Err: fmt.Errorf("cannot execute template: %w", err)}
	}
	return out.String(), nil
}
