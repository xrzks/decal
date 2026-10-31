package cli

import (
	"strings"
	"testing"
)

func TestRenderVariables(t *testing.T) {
	answers := map[string]any{
		"name":    "api",
		"enabled": true,
		"tags":    []string{"one", "two"},
	}

	body, err := renderPart("body", `package {{ .name }}
{{ if .enabled }}enabled{{ else }}disabled{{ end }}
{{ range .tags }}tag {{ . }}
{{ end }}`, answers)
	if err != nil {
		t.Fatalf("renderPart: %v", err)
	}

	want := "package api\nenabled\ntag one\ntag two\n"
	if body != want {
		t.Fatalf("body = %q, want %q", body, want)
	}
}

func TestRenderEmptyAnswers(t *testing.T) {
	body, err := renderPart("body", "static body\n", map[string]any{})
	if err != nil {
		t.Fatalf("renderPart: %v", err)
	}
	if body != "static body\n" {
		t.Fatalf("body = %q", body)
	}
}

func TestRenderMissingVariableIsAnError(t *testing.T) {
	_, err := renderPart("body", "package {{ .name }}", map[string]any{})
	if err == nil {
		t.Fatal("renderPart succeeded, want a missing-key error")
	}
	if !strings.Contains(err.Error(), "body") || !strings.Contains(err.Error(), "name") {
		t.Fatalf("err = %v, want it to name the body and the missing key", err)
	}
}

func TestRenderParseErrorNamesThePart(t *testing.T) {
	_, err := renderPart("output filename", "{{ .name", map[string]any{"name": "x"})
	if err == nil {
		t.Fatal("renderPart succeeded, want a parse error")
	}
	if !strings.Contains(err.Error(), "output filename") || !strings.Contains(err.Error(), "cannot parse template") {
		t.Fatalf("err = %v, want an output filename parse error", err)
	}
}

func TestRenderHasNoCustomBehavior(t *testing.T) {
	if _, err := renderPart("body", `{{ env "HOME" }}`, map[string]any{}); err == nil {
		t.Fatal("renderPart succeeded, want an unknown-function error")
	}
}
