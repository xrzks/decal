package catalog

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	texttemplate "text/template"
	"text/template/parse"
	"unicode/utf8"

	"gopkg.in/yaml.v3"
)

// FieldType is a supported form field type.
type FieldType string

const (
	FieldInput       FieldType = "input"
	FieldText        FieldType = "text"
	FieldSelect      FieldType = "select"
	FieldMultiSelect FieldType = "multiselect"
	FieldConfirm     FieldType = "confirm"
)

// Option is one choice of a select or multiselect field.
type Option struct {
	Label string
	Value string
}

// Field is one form field declared in a template's front matter.
type Field struct {
	Key      string
	Type     FieldType
	Label    string
	Required bool
	Options  []Option
	// Default is nil when undeclared, otherwise a string for input, text and
	// select, a []string for multiselect, and a bool for confirm.
	Default any
}

// DefaultString returns the field's default for string-valued fields.
func (f Field) DefaultString() string { s, _ := f.Default.(string); return s }

// DefaultStrings returns the field's default selection for multiselect fields.
func (f Field) DefaultStrings() []string { values, _ := f.Default.([]string); return values }

// DefaultBool returns the field's default for confirm fields.
func (f Field) DefaultBool() bool { b, _ := f.Default.(bool); return b }

func (f Field) hasOption(value string) bool {
	for _, option := range f.Options {
		if option.Value == value {
			return true
		}
	}
	return false
}

type rawTemplate struct {
	Name   *string    `yaml:"name"`
	Output string     `yaml:"output"`
	Fields []rawField `yaml:"fields"`
}

type rawField struct {
	Key      string      `yaml:"key"`
	Type     string      `yaml:"type"`
	Label    string      `yaml:"label"`
	Required bool        `yaml:"required"`
	Options  []rawOption `yaml:"options"`
	Default  yaml.Node   `yaml:"default"`
}

type rawOption struct {
	Label string `yaml:"label"`
	Value string `yaml:"value"`
}

// Parse reads a template file's front matter and body.
func Parse(path string, data []byte) (*Template, error) {
	fail := func(format string, args ...any) error {
		return fmt.Errorf("template %s: %s", path, fmt.Sprintf(format, args...))
	}
	if !utf8.Valid(data) {
		return nil, fail("file is not valid UTF-8")
	}

	front, body, err := splitFrontMatter(data)
	if err != nil {
		return nil, fail("%v", err)
	}

	raw := rawTemplate{}
	if len(bytes.TrimSpace(front)) > 0 {
		decoder := yaml.NewDecoder(bytes.NewReader(front))
		decoder.KnownFields(true)
		if err := decoder.Decode(&raw); err != nil && !errors.Is(err, io.EOF) {
			return nil, fail("invalid front matter: %v", err)
		}
	}

	if strings.TrimSpace(raw.Output) == "" {
		return nil, fail(`front matter is missing the required "output" filename`)
	}

	template := &Template{
		Path:     path,
		FileName: filepath.Base(path),
		Name:     filepath.Base(path),
		Output:   raw.Output,
		Body:     string(body),
	}
	if raw.Name != nil && strings.TrimSpace(*raw.Name) != "" {
		template.Name = strings.TrimSpace(*raw.Name)
	}

	// Catch syntax errors before the user fills in a form, not after.
	trees := make([]*parse.Tree, 0, 2)
	for _, part := range []struct{ name, source string }{
		{"output filename", template.Output},
		{"body", template.Body},
	} {
		parsed, err := texttemplate.New(part.name).Parse(part.source)
		if err != nil {
			return nil, fail("%s: cannot parse template: %v", part.name, err)
		}
		trees = append(trees, parsed.Tree)
	}

	fields, err := parseFields(raw.Fields)
	if err != nil {
		return nil, fail("%v", err)
	}
	template.Fields = fields

	// Catch misspelled field names too: rendering would only reject them after
	// the form has been filled in.
	declared := make([]string, 0, len(fields))
	for _, field := range fields {
		declared = append(declared, field.Key)
	}
	for i, tree := range trees {
		for _, ref := range fieldRefs(tree) {
			if !slices.Contains(declared, ref.key) {
				location, _ := tree.ErrorContext(ref.node)
				return nil, fail("%s: field %q is not declared%s", location, ref.key, declaredList(declared))
			}
			if i == 0 && !slices.Contains(template.OutputFields, ref.key) {
				template.OutputFields = append(template.OutputFields, ref.key)
			}
		}
	}
	return template, nil
}

func declaredList(keys []string) string {
	if len(keys) == 0 {
		return " (the template declares no fields)"
	}
	return " (declared: " + strings.Join(keys, ", ") + ")"
}

type fieldRef struct {
	key  string
	node parse.Node
}

// fieldRefs returns every use of a field of the template data in tree's main
// template, in source order: {{ .key }} wherever the dot is still the data,
// and {{ $.key }} anywhere. Inside range and with the dot is something else,
// and in {{ define }}d templates it is whatever the caller passed, so those
// uses are left to the renderer.
func fieldRefs(tree *parse.Tree) []fieldRef {
	var refs []fieldRef
	var walk func(node parse.Node, dotIsData bool)
	walk = func(node parse.Node, dotIsData bool) {
		switch n := node.(type) {
		case *parse.ListNode:
			if n == nil {
				return
			}
			for _, child := range n.Nodes {
				walk(child, dotIsData)
			}
		case *parse.ActionNode:
			walk(n.Pipe, dotIsData)
		case *parse.IfNode:
			walk(n.Pipe, dotIsData)
			walk(n.List, dotIsData)
			walk(n.ElseList, dotIsData)
		case *parse.RangeNode:
			walk(n.Pipe, dotIsData)
			walk(n.List, false)
			walk(n.ElseList, dotIsData)
		case *parse.WithNode:
			walk(n.Pipe, dotIsData)
			walk(n.List, false)
			walk(n.ElseList, dotIsData)
		case *parse.TemplateNode:
			walk(n.Pipe, dotIsData)
		case *parse.PipeNode:
			if n == nil {
				return
			}
			for _, command := range n.Cmds {
				walk(command, dotIsData)
			}
		case *parse.CommandNode:
			for _, arg := range n.Args {
				walk(arg, dotIsData)
			}
		case *parse.ChainNode:
			walk(n.Node, dotIsData)
		case *parse.FieldNode:
			if dotIsData {
				refs = append(refs, fieldRef{key: n.Ident[0], node: n})
			}
		case *parse.VariableNode:
			if n.Ident[0] == "$" && len(n.Ident) > 1 {
				refs = append(refs, fieldRef{key: n.Ident[1], node: n})
			}
		}
	}
	if tree != nil {
		walk(tree.Root, true)
	}
	return refs
}

// splitFrontMatter returns the YAML between the leading "---" line and the
// next "---" line, plus everything after that closing line.
func splitFrontMatter(data []byte) (front, body []byte, err error) {
	rest := data
	if bytes.HasPrefix(rest, []byte("\xef\xbb\xbf")) {
		rest = rest[3:]
	}

	line, next := nextLine(rest)
	if string(bytes.TrimRight(line, " \t\r")) != "---" {
		return nil, nil, errors.New(`file must start with a "---" front matter line`)
	}
	rest = next
	frontStart := len(data) - len(rest)

	for {
		if len(rest) == 0 {
			return nil, nil, errors.New(`front matter is not terminated by a "---" line`)
		}
		line, next := nextLine(rest)
		lineStart := len(data) - len(rest)
		if string(bytes.TrimRight(line, " \t\r")) == "---" {
			return data[frontStart:lineStart], next, nil
		}
		if len(next) == 0 {
			return nil, nil, errors.New(`front matter is not terminated by a "---" line`)
		}
		rest = next
	}
}

func nextLine(data []byte) (line, rest []byte) {
	if i := bytes.IndexByte(data, '\n'); i >= 0 {
		return data[:i], data[i+1:]
	}
	return data, nil
}

// keyPattern matches the names text/template accepts after a dot, so every
// answer is reachable as {{ .key }}.
var keyPattern = regexp.MustCompile(`^[\pL_][\pL\p{Nd}_]*$`)

func parseFields(raw []rawField) ([]Field, error) {
	fields := make([]Field, 0, len(raw))
	seen := make(map[string]bool, len(raw))

	for i, rawField := range raw {
		key := strings.TrimSpace(rawField.Key)
		if key == "" {
			return nil, fmt.Errorf("fields[%d]: key is required", i)
		}
		if !keyPattern.MatchString(key) {
			return nil, fmt.Errorf("fields[%d]: key %q must be letters, digits and underscores, not starting with a digit", i, key)
		}
		if seen[key] {
			return nil, fmt.Errorf("fields[%d]: duplicate key %q", i, key)
		}
		seen[key] = true

		field := Field{
			Key:      key,
			Type:     FieldType(rawField.Type),
			Label:    strings.TrimSpace(rawField.Label),
			Required: rawField.Required,
		}
		if field.Label == "" {
			field.Label = key
		}

		switch field.Type {
		case FieldInput, FieldText, FieldConfirm:
			if len(rawField.Options) > 0 {
				return nil, fmt.Errorf("field %q: type %s does not support options", key, field.Type)
			}
		case FieldSelect, FieldMultiSelect:
			if len(rawField.Options) == 0 {
				return nil, fmt.Errorf("field %q: type %s requires at least one option", key, field.Type)
			}
			seenValues := make(map[string]bool, len(rawField.Options))
			for j, rawOption := range rawField.Options {
				if rawOption.Value == "" {
					return nil, fmt.Errorf("field %q: options[%d]: value is required", key, j)
				}
				if seenValues[rawOption.Value] {
					return nil, fmt.Errorf("field %q: options[%d]: duplicate value %q", key, j, rawOption.Value)
				}
				seenValues[rawOption.Value] = true

				label := strings.TrimSpace(rawOption.Label)
				if label == "" {
					label = rawOption.Value
				}
				field.Options = append(field.Options, Option{Label: label, Value: rawOption.Value})
			}
		case "":
			return nil, fmt.Errorf("field %q: type is required", key)
		default:
			return nil, fmt.Errorf("field %q: unsupported type %q (supported: input, text, select, multiselect, confirm)", key, field.Type)
		}

		if field.Type == FieldConfirm && field.Required {
			return nil, fmt.Errorf("field %q: confirm fields cannot be required; use a default instead", key)
		}

		defaultValue, err := parseDefault(field, rawField.Default)
		if err != nil {
			return nil, err
		}
		field.Default = defaultValue
		fields = append(fields, field)
	}
	return fields, nil
}

func parseDefault(field Field, node yaml.Node) (any, error) {
	if node.Kind == 0 {
		return nil, nil
	}

	switch field.Type {
	case FieldInput, FieldText:
		return decodeDefault[string](field, node, "!!str", "a string")

	case FieldConfirm:
		return decodeDefault[bool](field, node, "!!bool", "a boolean")

	case FieldSelect:
		value, err := decodeDefault[string](field, node, "!!str", "a string")
		if err != nil {
			return nil, err
		}
		if !field.hasOption(value) {
			return nil, fmt.Errorf("field %q: default %q is not one of the declared options", field.Key, value)
		}
		return value, nil

	case FieldMultiSelect:
		if node.Kind != yaml.SequenceNode {
			return nil, fmt.Errorf("field %q: default must be a list of strings", field.Key)
		}
		for _, item := range node.Content {
			if item.Tag != "!!str" {
				return nil, fmt.Errorf("field %q: default must be a list of strings", field.Key)
			}
		}
		var values []string
		if err := node.Decode(&values); err != nil {
			return nil, fmt.Errorf("field %q: default must be a list of strings", field.Key)
		}
		seen := make(map[string]bool, len(values))
		for _, value := range values {
			if !field.hasOption(value) {
				return nil, fmt.Errorf("field %q: default %q is not one of the declared options", field.Key, value)
			}
			if seen[value] {
				return nil, fmt.Errorf("field %q: default lists %q more than once", field.Key, value)
			}
			seen[value] = true
		}
		return values, nil
	}

	// Unreachable: parseFields rejects every type not handled above.
	return nil, nil
}

// decodeDefault decodes a default of the given YAML tag into T. yaml.v3 coerces
// numbers and booleans into strings, so the tag is checked explicitly.
func decodeDefault[T any](field Field, node yaml.Node, tag, want string) (T, error) {
	var value T
	if node.Tag != tag {
		return value, fmt.Errorf("field %q: default must be %s", field.Key, want)
	}
	if err := node.Decode(&value); err != nil {
		return value, fmt.Errorf("field %q: default must be %s", field.Key, want)
	}
	return value, nil
}
