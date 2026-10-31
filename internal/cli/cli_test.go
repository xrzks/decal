package cli

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"charm.land/huh/v2"

	"templ/internal/catalog"
)

type fakePrompter struct {
	categoryName string
	templateName string
	answers      map[string]any
	err          error
	errAt        string

	categoryCalls int
	templateCalls int
	formCalls     int
}

func (f *fakePrompter) fail(stage string) error {
	if f.errAt == stage {
		return f.err
	}
	return nil
}

func (f *fakePrompter) ChooseCategory(_ context.Context, categories []catalog.Category) (catalog.Category, error) {
	f.categoryCalls++
	if err := f.fail("category"); err != nil {
		return catalog.Category{}, err
	}
	for _, category := range categories {
		if category.Name == f.categoryName {
			return category, nil
		}
	}
	return catalog.Category{}, errors.New("category not found")
}

func (f *fakePrompter) ChooseTemplate(_ context.Context, templates []*catalog.Template) (*catalog.Template, error) {
	f.templateCalls++
	if err := f.fail("template"); err != nil {
		return nil, err
	}
	for _, template := range templates {
		if template.Name == f.templateName {
			return template, nil
		}
	}
	return nil, errors.New("template not found")
}

func (f *fakePrompter) Form(_ context.Context, _ *catalog.Template) (map[string]any, error) {
	f.formCalls++
	if err := f.fail("form"); err != nil {
		return nil, err
	}
	answers := make(map[string]any, len(f.answers))
	for key, value := range f.answers {
		answers[key] = value
	}
	return answers, nil
}

func newRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	return root
}

func addTemplate(t *testing.T, root, category, file, content string) {
	t.Helper()
	dir := filepath.Join(root, category)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, file), []byte(content), 0o644); err != nil {
		t.Fatalf("write template: %v", err)
	}
}

func listDir(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read dir: %v", err)
	}
	var names []string
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	return names
}

func TestRunWithoutForm(t *testing.T) {
	root := newRoot(t)
	addTemplate(t, root, "notes", "meeting.md", "---\nname: Meeting notes\noutput: meeting-notes.md\n---\n# Notes\n")

	dir := t.TempDir()
	out := &strings.Builder{}
	prompter := &fakePrompter{categoryName: "notes", templateName: "Meeting notes"}

	if err := Run(context.Background(), root, prompter, dir, out); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if prompter.formCalls != 0 {
		t.Fatalf("form was shown %d times, want 0", prompter.formCalls)
	}

	path := filepath.Join(dir, "meeting-notes.md")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	if string(data) != "# Notes\n" {
		t.Fatalf("content = %q, want the body without front matter", data)
	}
	if !strings.Contains(out.String(), path) {
		t.Fatalf("output = %q, want it to report %s", out.String(), path)
	}
}

func TestRunWithForm(t *testing.T) {
	root := newRoot(t)
	addTemplate(t, root, "service", "service.tmpl", `---
name: Service
output: "{{ .name }}{{ if .docker }}.docker{{ end }}.{{ .kind }}"
fields:
  - key: name
    type: input
    required: true
  - key: kind
    type: select
    options:
      - value: go
      - value: rust
  - key: features
    type: multiselect
    options:
      - value: metrics
      - value: logs
  - key: docker
    type: confirm
---
package {{ .name }}
{{ range .features }}feature {{ . }}
{{ end }}{{ if .docker }}docker
{{ end }}`)

	dir := t.TempDir()
	prompter := &fakePrompter{
		categoryName: "service",
		templateName: "Service",
		answers: map[string]any{
			"name":     "api",
			"kind":     "go",
			"features": []string{"metrics", "logs"},
			"docker":   true,
		},
	}

	if err := Run(context.Background(), root, prompter, dir, &strings.Builder{}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if prompter.formCalls != 1 {
		t.Fatalf("form was shown %d times, want 1", prompter.formCalls)
	}

	path := filepath.Join(dir, "api.docker.go")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	want := "package api\nfeature metrics\nfeature logs\ndocker\n"
	if string(data) != want {
		t.Fatalf("content = %q, want %q", data, want)
	}
}

func TestRunCancellationLeavesNoFile(t *testing.T) {
	for _, stage := range []string{"category", "template", "form"} {
		t.Run(stage, func(t *testing.T) {
			root := newRoot(t)
			addTemplate(t, root, "docs", "a.md", "---\noutput: a.md\nfields:\n  - key: x\n    type: input\n---\nbody\n")

			dir := t.TempDir()
			prompter := &fakePrompter{
				categoryName: "docs",
				templateName: "a.md",
				errAt:        stage,
				err:          ErrCancelled,
			}

			err := Run(context.Background(), root, prompter, dir, &strings.Builder{})
			if !errors.Is(err, ErrCancelled) {
				t.Fatalf("err = %v, want ErrCancelled", err)
			}
			if names := listDir(t, dir); len(names) != 0 {
				t.Fatalf("cancellation left files behind: %v", names)
			}
		})
	}
}

func TestRunRejectsMalformedMetadataBeforePrompting(t *testing.T) {
	root := newRoot(t)
	addTemplate(t, root, "docs", "a.md", "---\nname: Broken\n---\nbody\n")

	prompter := &fakePrompter{categoryName: "docs", templateName: "Broken"}
	err := Run(context.Background(), root, prompter, t.TempDir(), &strings.Builder{})
	if err == nil {
		t.Fatal("Run succeeded, want a metadata error")
	}
	if prompter.categoryCalls != 0 {
		t.Fatalf("category prompt was shown %d times, want 0", prompter.categoryCalls)
	}
}

func TestRunReportsEmptyConfiguration(t *testing.T) {
	root := newRoot(t)
	prompter := &fakePrompter{}
	err := Run(context.Background(), root, prompter, t.TempDir(), &strings.Builder{})
	if err == nil || !strings.Contains(err.Error(), "no categories found") {
		t.Fatalf("err = %v, want a no-categories error", err)
	}
	if prompter.categoryCalls != 0 {
		t.Fatalf("category prompt was shown %d times, want 0", prompter.categoryCalls)
	}
}

func TestRunRefusesExistingDestination(t *testing.T) {
	root := newRoot(t)
	addTemplate(t, root, "docs", "a.md", "---\noutput: a.md\n---\nnew content\n")

	dir := t.TempDir()
	path := filepath.Join(dir, "a.md")
	if err := os.WriteFile(path, []byte("original"), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}

	prompter := &fakePrompter{categoryName: "docs", templateName: "a.md"}
	err := Run(context.Background(), root, prompter, dir, &strings.Builder{})
	if err == nil || !strings.Contains(err.Error(), "destination already exists") {
		t.Fatalf("err = %v, want a destination error", err)
	}
	data, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatalf("read: %v", readErr)
	}
	if string(data) != "original" {
		t.Fatalf("existing file was modified: %q", data)
	}
}

func TestRunRejectsUnsafeRenderedOutput(t *testing.T) {
	root := newRoot(t)
	addTemplate(t, root, "docs", "a.md", "---\noutput: \"../escape.md\"\n---\nbody\n")

	dir := filepath.Join(t.TempDir(), "work")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	prompter := &fakePrompter{categoryName: "docs", templateName: "a.md"}
	err := Run(context.Background(), root, prompter, dir, &strings.Builder{})
	if err == nil || !strings.Contains(err.Error(), "path separators") {
		t.Fatalf("err = %v, want a path separator error", err)
	}
	if names := listDir(t, dir); len(names) != 0 {
		t.Fatalf("unsafe output created files: %v", names)
	}
	if _, statErr := os.Stat(filepath.Join(filepath.Dir(dir), "escape.md")); statErr == nil {
		t.Fatal("a file was created outside the working directory")
	}
}

func TestRunRenderErrorsCreateNothing(t *testing.T) {
	tests := map[string]string{
		"body":   "---\noutput: a.md\n---\n{{ .missing }}\n",
		"output": "---\noutput: \"{{ .missing }}.md\"\n---\nbody\n",
	}
	for part, source := range tests {
		t.Run(part, func(t *testing.T) {
			root := newRoot(t)
			addTemplate(t, root, "docs", "a.md", source)
			dir := t.TempDir()

			prompter := &fakePrompter{categoryName: "docs", templateName: "a.md"}
			err := Run(context.Background(), root, prompter, dir, &strings.Builder{})
			if err == nil || !strings.Contains(err.Error(), part) {
				t.Fatalf("err = %v, want it to name the %s", err, part)
			}
			if names := listDir(t, dir); len(names) != 0 {
				t.Fatalf("render error left files behind: %v", names)
			}
		})
	}
}

func TestRunExampleServiceTemplate(t *testing.T) {
	root := filepath.Join("..", "..", "examples", "templates")
	dir := t.TempDir()
	prompter := &fakePrompter{
		categoryName: "service",
		templateName: "Go service skeleton",
		answers: map[string]any{
			"name":        "myservice",
			"description": "Package myservice serves things.",
			"license":     "mit",
			"features":    []string{"logging", "health"},
			"dockerfile":  false,
		},
	}

	if err := Run(context.Background(), root, prompter, dir, &strings.Builder{}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "myservice.go"))
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	body := string(data)
	for _, want := range []string{
		"// Licensed under the MIT License.",
		"package myservice",
		"log/slog",
		"slog.Info(\"starting myservice\")",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("output is missing %q:\n%s", want, body)
		}
	}
	if strings.Contains(body, "Dockerfile") {
		t.Fatalf("dockerfile=false still rendered the Docker note:\n%s", body)
	}
	if strings.Contains(body, "Wait for SIGINT") {
		t.Fatalf("shutdown was not selected but rendered:\n%s", body)
	}
}

func TestRequiredValidators(t *testing.T) {
	requiredInput := catalog.Field{Key: "name", Type: catalog.FieldInput, Required: true}
	if err := requiredString(requiredInput, false)("   "); err == nil {
		t.Fatal("required input accepted a blank value")
	}
	if err := requiredString(catalog.Field{Key: "name"}, false)(""); err != nil {
		t.Fatalf("optional input rejected an empty value: %v", err)
	}

	defaulted := catalog.Field{Key: "name", Type: catalog.FieldInput, Required: true, Default: "myservice"}
	if err := requiredString(defaulted, false)(""); err == nil {
		t.Fatal("full-screen required input accepted a blank value with a default")
	}
	if err := requiredString(defaulted, true)(""); err != nil {
		t.Fatalf("accessible required input rejected a blank value that falls back to its default: %v", err)
	}
	blankDefault := catalog.Field{Key: "name", Type: catalog.FieldInput, Required: true, Default: "  "}
	if err := requiredString(blankDefault, true)(""); err == nil {
		t.Fatal("accessible required input accepted a blank line whose default is also blank")
	}

	validateSelection := requiredSelection(catalog.Field{Key: "features", Type: catalog.FieldMultiSelect, Required: true})
	if err := validateSelection(nil); err == nil {
		t.Fatal("required multiselect accepted no selection")
	}
	if err := validateSelection([]string{"logs"}); err != nil {
		t.Fatalf("required multiselect rejected a selection: %v", err)
	}
	if err := requiredSelection(catalog.Field{Key: "features"})(nil); err != nil {
		t.Fatalf("optional multiselect rejected no selection: %v", err)
	}
}

func TestPromptErrorMapsCancellation(t *testing.T) {
	if err := promptError("form", huh.ErrUserAborted); !errors.Is(err, ErrCancelled) {
		t.Fatalf("promptError(abort) = %v, want ErrCancelled", err)
	}
	other := errors.New("boom")
	if err := promptError("form", other); errors.Is(err, ErrCancelled) || !errors.Is(err, other) {
		t.Fatalf("promptError(other) = %v, want the original error", err)
	}
}
