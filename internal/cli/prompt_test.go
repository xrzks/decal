package cli

import (
	"bufio"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func promptStdin(t *testing.T, input string) {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { r.Close() })
	// Write asynchronously: input larger than the pipe buffer would otherwise
	// block before anything reads it.
	go func() {
		defer w.Close()
		_, _ = w.WriteString(input)
	}()
	original := os.Stdin
	os.Stdin = r
	t.Cleanup(func() { os.Stdin = original })
	t.Setenv("TERM", "dumb")
}

func TestRealPrompterEOFLeavesNoFile(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		fields string
	}{
		{name: "category"},
		{name: "category invalid then EOF", input: "9\n"},
		{name: "template", input: "1\n"},
		{name: "template invalid then EOF", input: "1\n9\n"},
		{name: "input", input: "1\n1\n", fields: "  - key: x\n    type: input\n    required: true\n"},
		{name: "text", input: "1\n1\n", fields: "  - key: x\n    type: text\n    required: true\n"},
		{name: "select", input: "1\n1\n", fields: "  - key: x\n    type: select\n    options: [{value: a}]\n"},
		{name: "select invalid then EOF", input: "1\n1\n9\n", fields: "  - key: x\n    type: select\n    options: [{value: a}]\n"},
		{name: "multiselect", input: "1\n1\n", fields: "  - key: x\n    type: multiselect\n    required: true\n    options: [{value: a}]\n"},
		{name: "multiselect invalid then EOF", input: "1\n1\n9\n", fields: "  - key: x\n    type: multiselect\n    required: true\n    options: [{value: a}]\n"},
		{name: "confirm", input: "1\n1\n", fields: "  - key: x\n    type: confirm\n    default: true\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			promptStdin(t, tt.input)
			root := t.TempDir()
			addTemplate(t, root, "docs", "a.tmpl", "---\noutput: a.txt\nfields:\n"+tt.fields+"---\nbody\n")
			dir := t.TempDir()
			err := Run(context.Background(), root, huhPrompter{}, dir, &strings.Builder{})
			if !errors.Is(err, ErrCancelled) {
				t.Fatalf("Run = %v, want ErrCancelled", err)
			}
			if names := listDir(t, dir); len(names) != 0 {
				t.Fatalf("EOF left files behind: %v", names)
			}
		})
	}
}

func TestRealPrompterNoTerminal(t *testing.T) {
	promptStdin(t, "1\n1\n")
	t.Setenv("TERM", "xterm")
	root := t.TempDir()
	addTemplate(t, root, "docs", "a.tmpl", "---\noutput: a.txt\n---\nbody\n")
	dir := t.TempDir()
	err := Run(context.Background(), root, huhPrompter{}, dir, &strings.Builder{})
	if !errors.Is(err, ErrNoTerminal) {
		t.Fatalf("Run = %v, want ErrNoTerminal", err)
	}
	if names := listDir(t, dir); len(names) != 0 {
		t.Fatalf("non-terminal run left files behind: %v", names)
	}
}

func TestRealPrompterAccessibleSubmission(t *testing.T) {
	promptStdin(t, "1\n1\nhello\n1\n0\n")
	root := t.TempDir()
	addTemplate(t, root, "docs", "a.tmpl", `---
output: a.txt
fields:
  - key: title
    type: input
    required: true
  - key: choices
    type: multiselect
    required: true
    options: [{value: a}]
---
{{ .title }}:{{ range .choices }}{{ . }}{{ end }}
`)
	dir := t.TempDir()
	if err := Run(context.Background(), root, huhPrompter{}, dir, &strings.Builder{}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "a.txt"))
	if err != nil || string(data) != "hello:a\n" {
		t.Fatalf("output = %q, err = %v", data, err)
	}
}

func TestRealPrompterAccessibleUsesDefaultOnBlankLine(t *testing.T) {
	promptStdin(t, "1\n1\n\n")
	root := t.TempDir()
	addTemplate(t, root, "docs", "a.tmpl", `---
output: a.txt
fields:
  - key: name
    type: input
    required: true
    default: myservice
---
{{ .name }}
`)
	dir := t.TempDir()
	if err := Run(context.Background(), root, huhPrompter{}, dir, &strings.Builder{}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "a.txt"))
	if err != nil || string(data) != "myservice\n" {
		t.Fatalf("output = %q, err = %v", data, err)
	}
}

func TestRealPrompterRejectsOverlongLine(t *testing.T) {
	promptStdin(t, "1\n1\n"+strings.Repeat("a", bufio.MaxScanTokenSize+1)+"\n")
	root := t.TempDir()
	addTemplate(t, root, "docs", "a.tmpl", `---
output: a.txt
fields:
  - key: body
    type: text
    required: true
---
[{{ .body }}]
`)
	dir := t.TempDir()
	err := Run(context.Background(), root, huhPrompter{}, dir, &strings.Builder{})
	if !errors.Is(err, errLineTooLong) {
		t.Fatalf("Run = %v, want errLineTooLong", err)
	}
	if names := listDir(t, dir); len(names) != 0 {
		t.Fatalf("overlong input left files behind: %v", names)
	}
}
