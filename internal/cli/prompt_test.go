package cli

import (
	"bufio"
	"context"
	"errors"
	"io"
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
			err := Run(context.Background(), root, huhPrompter{}, dir, &strings.Builder{}, io.Discard)
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
	addTemplate(t, root, "docs", "b.tmpl", "---\noutput: b.txt\n---\nbody\n")
	dir := t.TempDir()
	err := Run(context.Background(), root, huhPrompter{}, dir, &strings.Builder{}, io.Discard)
	if !errors.Is(err, ErrNoTerminal) {
		t.Fatalf("Run = %v, want ErrNoTerminal", err)
	}
	if names := listDir(t, dir); len(names) != 0 {
		t.Fatalf("non-terminal run left files behind: %v", names)
	}
}

func TestRealPrompterPicksLoneChoicesWithoutPrompting(t *testing.T) {
	// No prompt is needed, so no terminal is either.
	promptStdin(t, "")
	t.Setenv("TERM", "xterm")
	root := t.TempDir()
	addTemplate(t, root, "docs", "a.tmpl", "---\noutput: a.txt\n---\nbody\n")
	dir := t.TempDir()
	if err := Run(context.Background(), root, huhPrompter{}, dir, &strings.Builder{}, io.Discard); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "a.txt")); err != nil {
		t.Fatalf("output: %v", err)
	}
}

func TestRealPrompterAccessibleOutputIsPlainAndOnStderr(t *testing.T) {
	promptStdin(t, "")
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	original := os.Stderr
	os.Stderr = w
	t.Cleanup(func() { os.Stderr = original })
	root := t.TempDir()
	addTemplate(t, root, "docs", "a.tmpl", "---\noutput: a.txt\n---\nbody\n")
	_ = Run(context.Background(), root, huhPrompter{}, t.TempDir(), &strings.Builder{}, io.Discard)
	w.Close()
	data, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "Choose a category") || strings.Contains(string(data), "\x1b[") {
		t.Fatalf("prompt output = %q, want plain text", data)
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
	if err := Run(context.Background(), root, huhPrompter{}, dir, &strings.Builder{}, io.Discard); err != nil {
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
	if err := Run(context.Background(), root, huhPrompter{}, dir, &strings.Builder{}, io.Discard); err != nil {
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
	err := Run(context.Background(), root, huhPrompter{}, dir, &strings.Builder{}, io.Discard)
	if !errors.Is(err, errLineTooLong) {
		t.Fatalf("Run = %v, want errLineTooLong", err)
	}
	if names := listDir(t, dir); len(names) != 0 {
		t.Fatalf("overlong input left files behind: %v", names)
	}
}

func TestRealPrompterAccessibleMultiSelectBlankLine(t *testing.T) {
	const template = `---
output: a.txt
fields:
  - key: choices
    type: multiselect
    default: [a]
    options: [{value: a}, {value: b}]
  - key: after
    type: input
---
{{ range .choices }}{{ . }}{{ end }}:{{ .after }}
`
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "blank keeps the default", input: "1\n1\n\nx\n", want: "a:x\n"},
		{name: "blank confirms after toggling", input: "1\n1\n2\n\nx\n", want: "ab:x\n"},
		{name: "zero still confirms", input: "1\n1\n1\n0\nx\n", want: ":x\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			promptStdin(t, tt.input)
			root := t.TempDir()
			addTemplate(t, root, "docs", "a.tmpl", template)
			dir := t.TempDir()
			if err := Run(context.Background(), root, huhPrompter{}, dir, &strings.Builder{}, io.Discard); err != nil {
				t.Fatal(err)
			}
			data, err := os.ReadFile(filepath.Join(dir, "a.txt"))
			if err != nil || string(data) != tt.want {
				t.Fatalf("output = %q, err = %v, want %q", data, err, tt.want)
			}
		})
	}
}

func TestRealPrompterAccessibleRequiredMultiSelectRejectsBlankWithoutSelection(t *testing.T) {
	promptStdin(t, "1\n1\n\n")
	root := t.TempDir()
	addTemplate(t, root, "docs", "a.tmpl", "---\noutput: a.txt\nfields:\n  - key: x\n    type: multiselect\n    required: true\n    options: [{value: a}]\n---\nbody\n")
	dir := t.TempDir()
	err := Run(context.Background(), root, huhPrompter{}, dir, &strings.Builder{}, io.Discard)
	if !errors.Is(err, ErrCancelled) {
		t.Fatalf("Run = %v, want ErrCancelled once input ends after the rejected blank line", err)
	}
	if names := listDir(t, dir); len(names) != 0 {
		t.Fatalf("rejected selection left files behind: %v", names)
	}
}

func TestBlankConfirmsOnlyRewritesBlankLines(t *testing.T) {
	r := &blankConfirms{Reader: strings.NewReader("\n2\n\n12\n")}
	data, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	if want := "0\n2\n0\n12\n"; string(data) != want {
		t.Fatalf("read %q, want %q", data, want)
	}
}

func TestCommandCancellationReportsOnStderr(t *testing.T) {
	promptStdin(t, "")
	root := t.TempDir()
	addTemplate(t, root, "docs", "a.tmpl", "---\noutput: a.txt\n---\nbody\n")
	dir := t.TempDir()
	t.Chdir(dir)

	var stdout, stderr strings.Builder
	cmd := New("test")
	cmd.Writer, cmd.ErrWriter = &stdout, &stderr
	err := cmd.Run(context.Background(), []string{"decal", "--config-dir", root})
	if !errors.Is(err, ErrCancelled) {
		t.Fatalf("Run = %v, want ErrCancelled so main exits with ExitCancelled", err)
	}
	if !strings.Contains(stderr.String(), "Cancelled. No file was created.") {
		t.Fatalf("stderr = %q, want the cancellation notice", stderr.String())
	}
	if strings.Contains(stdout.String(), "Cancelled") {
		t.Fatalf("stdout = %q, want the cancellation notice on stderr only", stdout.String())
	}
	if names := listDir(t, dir); len(names) != 0 {
		t.Fatalf("cancelled command left files behind: %v", names)
	}
}
