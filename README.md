# decal

`decal` generates one file from a reusable, single-file template. Run it in the
directory that should receive the file, pick a category and a template, answer
the template's form if it has one, and the file is rendered and written there.

## Install

```sh
go install github.com/xrzks/decal@latest
```

Requires Go 1.27 or newer.

## Usage

```text
decal [options]
```

```sh
cd ~/src/new-project
decal
```

Categories and templates are discovered from your template root and presented
with [Charmbracelet Huh](https://github.com/charmbracelet/huh) select prompts; a
category or template that is the only choice is picked without asking. The
template's form (if any) follows, then the output filename and body are rendered
with Go's [`text/template`](https://pkg.go.dev/text/template).

## Options

| Flag                  | Alias | Description                                                                  |
| --------------------- | ----- | ---------------------------------------------------------------------------- |
| `--config-dir <path>` |       | template root directory (default: $XDG_CONFIG_HOME/decal or ~/.config/decal) |
| `--help`              | `-h`  | show help                                                                    |
| `--version`           | `-v`  | print the version                                                            |

## Configuration layout

The template root is, in order of precedence:

1. `--config-dir <path>`
2. `$XDG_CONFIG_HOME/decal`, when `XDG_CONFIG_HOME` is an absolute path
3. `~/.config/decal`

Inside the root, immediate child directories are categories and regular files
directly inside a category are templates:

```
~/.config/decal/
├── notes/
│   └── meeting-notes.md
└── service/
    └── go-service.tmpl
```

Discovery is not recursive. Symlinked categories and templates are followed, so
a root managed with GNU stow or a similar tool works. Dot-prefixed directories
and files are ignored, so `.git` or `.DS_Store` entries never break discovery.
Categories and templates are listed sorted by display name, with the filename as
a tie-breaker.

One broken entry does not hide the rest. An unreadable or malformed template, an
unreadable category, or a dangling symlink is left out with a
`warning: skipped …` line on stderr naming the path and the problem, and the
remaining templates are offered as usual. A category without templates is left
out silently. The command stops before any prompt only when the root is missing
or unreadable, or when no usable template is left.

Try the bundled examples without touching your own configuration:

```sh
decal --config-dir ./examples/templates
```

## Authoring a template

A template is a UTF-8 text file that starts with YAML front matter delimited by
`---` lines, followed by the file body. The front matter is metadata only and is
never written to the output.

```markdown
---
name: Meeting notes
output: meeting-notes.md
---

# Meeting notes
```

### Front matter reference

| Key      | Required | Meaning                                                        |
| -------- | -------- | -------------------------------------------------------------- |
| `name`   | no       | Display name in the template prompt. Defaults to the filename. |
| `output` | yes      | Output filename. May contain template expressions.             |
| `fields` | no       | Ordered list of form fields. Omitted or empty skips the form.  |

Unknown keys are rejected, so typos fail loudly instead of being ignored.

### Fields

Each field entry has:

| Key        | Required                   | Meaning                                                                                                         |
| ---------- | -------------------------- | --------------------------------------------------------------------------------------------------------------- |
| `key`      | yes                        | Unique name used as `{{ .key }}`: letters, digits and `_`, not starting with a digit.                           |
| `type`     | yes                        | `input`, `text`, `select`, `multiselect`, or `confirm`.                                                         |
| `label`    | no                         | Prompt label. Defaults to the key.                                                                              |
| `default`  | no                         | Typed initial answer; must match the field type and its options.                                                |
| `required` | no                         | Reject blank input/text and empty selections. Not valid on `confirm`.                                           |
| `options`  | for `select`/`multiselect` | Ordered `{label, value}` choices. Values must be unique and non-empty; a missing label falls back to the value. |

Field types map to Huh controls:

| Type          | Huh control | Answer type in templates    |
| ------------- | ----------- | --------------------------- |
| `input`       | Input       | `string`                    |
| `text`        | Text        | `string`                    |
| `select`      | Select      | `string` (one option value) |
| `multiselect` | MultiSelect | `[]string` (option values)  |
| `confirm`     | Confirm     | `bool`                      |

`false` is a real answer for `confirm`, so `required` is rejected instead of
being interpreted as consent. Metadata is validated in full before the form is
shown: invalid or duplicate keys, unsupported types, options on non-choice
fields, duplicate or empty option values, incompatible defaults, defaults
outside the declared options, template syntax errors in `output` or the body,
and references to undeclared fields (`{{ .nmae }}` when the field is `name`) all
fail early.

A complete example:

```yaml
---
name: Go service skeleton
output: "{{ .name }}.go"
fields:
  - key: name
    type: input
    label: Package name
    required: true
    default: myservice
  - key: description
    type: text
    required: true
  - key: license
    type: select
    default: apache
    options:
      - label: Apache 2.0
        value: apache
      - label: MIT
        value: mit
  - key: features
    type: multiselect
    default: [logging]
    options:
      - label: Structured logging
        value: logging
      - label: Health endpoint
        value: health
  - key: dockerfile
    type: confirm
    default: true
---
package {{ .name }}
```

See `examples/templates/notes/meeting-notes.md` for a template without a form
and `examples/templates/service/go-service.tmpl` for a form-driven template with
a templated filename, conditionals, loops, and typed answers.

## Rendering

Both `output` and the body are executed as Go `text/template` templates with the
same map of typed answers, keyed by field key. Variables, `if`, `else`, `range`,
`with`, `printf`, and the other built-in functions are available. `{{ .name }}`
is the `name` field's answer; `{{ range .features }}` walks a multiselect
answer.

There are no custom functions, shell commands, or hooks. Both parts are parsed
when templates are discovered, before any prompt, and every `{{ .key }}` and
`{{ $.key }}` is checked against the declared fields there, with the line and
column of a misspelled one. Inside `range` and `with`, and in `define`d
templates, the dot is something other than the answers, so those uses are
checked when rendering instead: an undefined key is an error
(`missingkey=error`), never silently empty. Both parts are rendered entirely
before the destination file is touched. Errors name whether they came from the
metadata, the output filename, or the body.

## Non-interactive use

The prompts are drawn on stderr, so stdout carries only the `Created <path>`
line. They need a terminal on stdin and stderr; otherwise `decal` stops with an
error instead of stalling, and suggests setting `TERM=dumb`. In that mode
[Huh](https://github.com/charmbracelet/huh) answers one line per prompt from
stdin, which makes the command scriptable:

```sh
printf '2\n1\nmyservice\n\n\n\n\n' | TERM=dumb decal --config-dir ./examples/templates
```

That answers the category (`2`, service), the template (`1`), and the service
name (`myservice`), then takes the defaults for the remaining four fields.
Unlike the full-screen prompts, this mode still asks for a category or template
that is the only choice, so scripted answers keep their positions when a second
one is added. Prompt text is plain, without colour codes.

Each answer is one line. Selects take the option number. A multiselect prompt
takes one option number per line, each toggling that option, until `0` or a
blank line confirms the current selection. A blank line takes the field's
default everywhere, so a blank first answer to a multiselect keeps its default
selection. Input is validated as it is read: an end-of-input before the last
prompt answers it cancels the run, and an answer line longer than 64 KiB is
rejected rather than silently replaced by the default.

## Exit status

| Status | Meaning                                                                  |
| ------ | ------------------------------------------------------------------------ |
| `0`    | The file was created, or `--help`/`--version` was printed.               |
| `1`    | An error: usage, configuration, metadata, rendering, or the destination. |
| `130`  | A prompt was cancelled (`ctrl+c`, or input ended early). No file exists. |

Prompts, warnings, errors and the cancellation notice go to stderr; only the
`Created <path>` line goes to stdout.

## Safety

- The rendered output must be a single filename directly in the current working
  directory. Absolute paths, `..`, `.`, nested paths, path separators, control
  characters, and host-invalid characters are rejected. Unsafe names are never
  normalized into a different destination.
- Existing destinations are never overwritten — files, directories, and symlinks
  (including dangling ones) are all refused. There is no overwrite mode.
- A clash is reported as early as possible. A fixed `output` name is checked
  before the form is shown. A name built from answers is checked in the
  full-screen form when the last field it uses is submitted, so that answer can
  be changed in place. Line-by-line `TERM=dumb` prompts skip that in-form check,
  because a re-asked prompt would shift every later scripted answer, and fail at
  the end instead.
- Creation uses exclusive creation, so a file that appears between validation
  and writing is not overwritten either.
- Cancelling any prompt, invalid metadata, failed form validation, and rendering
  errors leave no output behind. If writing fails after the file was created,
  the incomplete file is removed; pre-existing files are never removed.

## Manual terminal smoke test

Build the binary and work in a scratch directory with the bundled templates:

```sh
go build -o /tmp/decal .
mkdir -p /tmp/decal-smoke && cd /tmp/decal-smoke
/tmp/decal --config-dir /path/to/decal/examples/templates
```

1. **Selection:** the category prompt lists `notes` and `service`. Navigate with
   the arrow keys or `j`/`k` and press `enter` on `notes`. `Meeting notes` is
   the category's only template, so it is picked without a prompt. No form
   appears; `meeting-notes.md` is created and its path is printed. Check that
   the file exists and does not contain `---` front matter.
2. **Form navigation:** run again and choose `service`. The form shows one field
   per screen, so press `enter` to advance to the next. Fill the input (try
   submitting it empty first to see the required error), submit the multiline
   text with `enter` (`alt+enter` or `ctrl+j` inserts a newline), change the
   select with `enter`, toggle multiselect options with `space` and confirm with
   `enter`, and answer the confirm with `y`/`n` or `enter`. The file is named
   from the `name` answer and contains the rendered body.
3. **Cancellation:** run again and press `ctrl+c` at the category prompt, then
   again inside the form. Each time the command prints
   `Cancelled. No file was created.` and `echo $?` shows `130`; `ls -A` shows
   nothing new.
4. **Conflict:** with `meeting-notes.md` already present, run the notes template
   again. The command reports `destination already exists` before any form, and
   the file is untouched. Then run the service template and accept the name
   `myservice` again: the name field shows `myservice.go already exists here`
   and does not advance until the name is changed.

## Development

```sh
go test ./...
go vet ./...
go build .
```

`decal --version` reports the version Go records at install time. To stamp a
version into a local build, use `go build -ldflags "-X main.version=v1.0.0" .`.

The root `main.go` is a thin entry point. The packages are kept independently
testable: `internal/catalog` (discovery and metadata), `internal/generate`
(filename validation and exclusive creation), and `internal/cli` (template
rendering and the Huh-driven workflow, tested through a fake prompter).

## License

MIT. See [LICENSE](LICENSE).
