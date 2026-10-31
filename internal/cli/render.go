package cli

import (
	"fmt"
	"strings"
	"text/template"
)

// renderPart executes source as a Go text template. Missing keys are errors, so
// a typo in a template fails loudly instead of rendering empty.
func renderPart(part, source string, answers map[string]any) (string, error) {
	tmpl, err := template.New(part).Option("missingkey=error").Parse(source)
	if err != nil {
		return "", fmt.Errorf("%s: cannot parse template: %w", part, err)
	}

	var out strings.Builder
	if err := tmpl.Execute(&out, answers); err != nil {
		return "", fmt.Errorf("%s: cannot execute template: %w", part, err)
	}
	return out.String(), nil
}
