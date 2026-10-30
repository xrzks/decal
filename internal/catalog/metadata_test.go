package catalog

import (
	"strings"
	"testing"
)

func TestParseMetadata(t *testing.T) {
	source := `---
name: Go service
output: "{{ .name }}.go"
fields:
  - key: name
    type: input
    label: Service name
    required: true
    default: myservice
  - key: description
    type: text
  - key: license
    type: select
    options:
      - label: MIT
        value: mit
      - value: apache
    default: apache
  - key: features
    type: multiselect
    options:
      - label: Metrics
        value: metrics
      - value: logs
    default:
      - logs
  - key: docker
    type: confirm
    label: Add a Dockerfile?
    default: true
---
package {{ .name }}
`

	template, err := Parse("service.go.tmpl", []byte(source))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	if template.Name != "Go service" {
		t.Errorf("Name = %q, want %q", template.Name, "Go service")
	}
	if template.FileName != "service.go.tmpl" {
		t.Errorf("FileName = %q", template.FileName)
	}
	if template.Output != "{{ .name }}.go" {
		t.Errorf("Output = %q", template.Output)
	}
	if template.Body != "package {{ .name }}\n" {
		t.Errorf("Body = %q", template.Body)
	}

	if len(template.Fields) != 5 {
		t.Fatalf("got %d fields, want 5", len(template.Fields))
	}

	name := template.Fields[0]
	if name.Key != "name" || name.Type != FieldInput || name.Label != "Service name" || !name.Required {
		t.Errorf("field 0 = %+v", name)
	}
	if got := name.DefaultString(); got != "myservice" {
		t.Errorf("name default = %q", got)
	}

	description := template.Fields[1]
	if description.Label != "description" {
		t.Errorf("label fallback = %q, want the key", description.Label)
	}
	if description.Default != nil {
		t.Errorf("description default = %v, want nil", description.Default)
	}

	license := template.Fields[2]
	if license.Options[0].Label != "MIT" || license.Options[1].Label != "apache" {
		t.Errorf("options = %+v", license.Options)
	}
	if got := license.DefaultString(); got != "apache" {
		t.Errorf("license default = %q", got)
	}

	features := template.Fields[3]
	if got := features.DefaultStrings(); len(got) != 1 || got[0] != "logs" {
		t.Errorf("features default = %v", got)
	}

	docker := template.Fields[4]
	if !docker.DefaultBool() {
		t.Errorf("docker default = false, want true")
	}
}

func TestParseNameFallsBackToFileName(t *testing.T) {
	template, err := Parse("notes.md", []byte("---\noutput: notes.md\n---\n# Notes\n"))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if template.Name != "notes.md" {
		t.Fatalf("Name = %q, want notes.md", template.Name)
	}
	if len(template.Fields) != 0 {
		t.Fatalf("got %d fields, want none", len(template.Fields))
	}
}

func TestParseCRLF(t *testing.T) {
	source := "---\r\nname: CRLF\r\noutput: crlf.txt\r\n---\r\nbody\r\n"
	template, err := Parse("crlf.txt", []byte(source))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if template.Name != "CRLF" || template.Output != "crlf.txt" || template.Body != "body\r\n" {
		t.Fatalf("template = %+v", template)
	}
}

func TestParseErrors(t *testing.T) {
	tests := []struct {
		name   string
		source string
		want   string
	}{
		{
			name:   "no front matter",
			source: "just a body\n",
			want:   `must start with a "---"`,
		},
		{
			name:   "unterminated front matter",
			source: "---\noutput: a.txt\nbody\n",
			want:   "not terminated",
		},
		{
			name:   "missing output",
			source: "---\nname: x\n---\nbody\n",
			want:   `missing the required "output"`,
		},
		{
			name:   "blank output",
			source: "---\noutput: \"  \"\n---\nbody\n",
			want:   `missing the required "output"`,
		},
		{
			name:   "unknown front matter key",
			source: "---\noutput: a.txt\nauthor: me\n---\nbody\n",
			want:   "field author not found",
		},
		{
			name:   "unknown field key",
			source: "---\noutput: a.txt\nfields:\n  - key: a\n    type: input\n    hint: nope\n---\nbody\n",
			want:   "field hint not found",
		},
		{
			name:   "invalid yaml",
			source: "---\noutput: [a.txt\n---\nbody\n",
			want:   "invalid front matter",
		},
		{
			name:   "invalid utf8",
			source: "---\noutput: a.txt\n---\n\xff\n",
			want:   "not valid UTF-8",
		},
		{
			name:   "missing field key",
			source: "---\noutput: a.txt\nfields:\n  - type: input\n---\nbody\n",
			want:   "key is required",
		},
		{
			name:   "duplicate field key",
			source: "---\noutput: a.txt\nfields:\n  - key: a\n    type: input\n  - key: a\n    type: text\n---\nbody\n",
			want:   `duplicate key "a"`,
		},
		{
			name:   "missing type",
			source: "---\noutput: a.txt\nfields:\n  - key: a\n---\nbody\n",
			want:   "type is required",
		},
		{
			name:   "unsupported type",
			source: "---\noutput: a.txt\nfields:\n  - key: a\n    type: checkbox\n---\nbody\n",
			want:   `unsupported type "checkbox"`,
		},
		{
			name:   "options on input",
			source: "---\noutput: a.txt\nfields:\n  - key: a\n    type: input\n    options:\n      - value: x\n---\nbody\n",
			want:   "does not support options",
		},
		{
			name:   "select without options",
			source: "---\noutput: a.txt\nfields:\n  - key: a\n    type: select\n---\nbody\n",
			want:   "requires at least one option",
		},
		{
			name:   "option without value",
			source: "---\noutput: a.txt\nfields:\n  - key: a\n    type: select\n    options:\n      - label: X\n---\nbody\n",
			want:   "value is required",
		},
		{
			name:   "duplicate option value",
			source: "---\noutput: a.txt\nfields:\n  - key: a\n    type: select\n    options:\n      - value: x\n      - value: x\n---\nbody\n",
			want:   `duplicate value "x"`,
		},
		{
			name:   "required confirm",
			source: "---\noutput: a.txt\nfields:\n  - key: a\n    type: confirm\n    required: true\n---\nbody\n",
			want:   "confirm fields cannot be required",
		},
		{
			name:   "input default not a string",
			source: "---\noutput: a.txt\nfields:\n  - key: a\n    type: input\n    default: 42\n---\nbody\n",
			want:   "default must be a string",
		},
		{
			name:   "confirm default not a bool",
			source: "---\noutput: a.txt\nfields:\n  - key: a\n    type: confirm\n    default: maybe\n---\nbody\n",
			want:   "default must be a boolean",
		},
		{
			name:   "select default outside options",
			source: "---\noutput: a.txt\nfields:\n  - key: a\n    type: select\n    options:\n      - value: x\n    default: y\n---\nbody\n",
			want:   `default "y" is not one of the declared options`,
		},
		{
			name:   "multiselect default not a list",
			source: "---\noutput: a.txt\nfields:\n  - key: a\n    type: multiselect\n    options:\n      - value: x\n    default: x\n---\nbody\n",
			want:   "default must be a list of strings",
		},
		{
			name:   "multiselect default outside options",
			source: "---\noutput: a.txt\nfields:\n  - key: a\n    type: multiselect\n    options:\n      - value: x\n    default: [y]\n---\nbody\n",
			want:   `default "y" is not one of the declared options`,
		},
		{
			name:   "multiselect duplicate default",
			source: "---\noutput: a.txt\nfields:\n  - key: a\n    type: multiselect\n    options:\n      - value: x\n    default: [x, x]\n---\nbody\n",
			want:   `default lists "x" more than once`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := Parse("template.md", []byte(test.source))
			if err == nil {
				t.Fatalf("Parse succeeded, want an error containing %q", test.want)
			}
			if !strings.Contains(err.Error(), test.want) {
				t.Fatalf("err = %v, want it to contain %q", err, test.want)
			}
			if !strings.Contains(err.Error(), "template.md") {
				t.Fatalf("err = %v, want it to name the template", err)
			}
		})
	}
}

func TestParseFrontMatterIsNotPartOfBody(t *testing.T) {
	template, err := Parse("x.md", []byte("---\noutput: x.md\nfields: []\n---\nline one\n---\nline two\n"))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if template.Body != "line one\n---\nline two\n" {
		t.Fatalf("Body = %q", template.Body)
	}
}
