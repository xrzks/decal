package render

import (
	"errors"
	"strings"
	"testing"
)

func TestBodyVariables(t *testing.T) {
	answers := map[string]any{
		"name":    "api",
		"enabled": true,
		"tags":    []string{"one", "two"},
	}

	body, err := Body(`package {{ .name }}
{{ if .enabled }}enabled{{ else }}disabled{{ end }}
{{ range .tags }}tag {{ . }}
{{ end }}`, answers)
	if err != nil {
		t.Fatalf("Body: %v", err)
	}

	want := "package api\nenabled\ntag one\ntag two\n"
	if body != want {
		t.Fatalf("body = %q, want %q", body, want)
	}
}

func TestOutputNameRendering(t *testing.T) {
	name, err := OutputName("{{ .name }}_{{ if .enabled }}on{{ else }}off{{ end }}.txt", map[string]any{
		"name":    "svc",
		"enabled": false,
	})
	if err != nil {
		t.Fatalf("OutputName: %v", err)
	}
	if name != "svc_off.txt" {
		t.Fatalf("name = %q", name)
	}
}

func TestEmptyAnswers(t *testing.T) {
	body, err := Body("static body\n", map[string]any{})
	if err != nil {
		t.Fatalf("Body: %v", err)
	}
	if body != "static body\n" {
		t.Fatalf("body = %q", body)
	}
}

func TestMissingVariableIsAnError(t *testing.T) {
	_, err := Body("package {{ .name }}", map[string]any{})
	if err == nil {
		t.Fatal("Body succeeded, want a missing-key error")
	}
	if !strings.Contains(err.Error(), "body") || !strings.Contains(err.Error(), "name") {
		t.Fatalf("err = %v, want it to name the body and the missing key", err)
	}

	var renderErr *Error
	if !errors.As(err, &renderErr) || renderErr.Part != BodyPart {
		t.Fatalf("err = %v, want a body Error", err)
	}
}

func TestParseErrorNamesThePart(t *testing.T) {
	_, err := OutputName("{{ .name", map[string]any{"name": "x"})
	if err == nil {
		t.Fatal("OutputName succeeded, want a parse error")
	}

	var renderErr *Error
	if !errors.As(err, &renderErr) || renderErr.Part != OutputPart {
		t.Fatalf("err = %v, want an output filename Error", err)
	}
	if !strings.Contains(err.Error(), "cannot parse template") {
		t.Fatalf("err = %v, want a parse error", err)
	}
}

func TestCustomBehaviorIsNotAvailable(t *testing.T) {
	_, err := Body(`{{ env "HOME" }}`, map[string]any{})
	if err == nil {
		t.Fatal("Body succeeded, want an unknown-function error")
	}
}
