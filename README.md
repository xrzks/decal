# templ

`templ` generates one file from a reusable, single-file template. Run it in the
directory that should receive the file, pick a category and a template, answer
the template's form if it has one, and the file is rendered and written there.

```console
$ cd ~/src/new-project
$ templ
```

Categories and templates are discovered from your template root and presented
with [Charmbracelet Huh](https://github.com/charmbracelet/huh) select prompts;
a category or template that is the only choice is picked without asking. The
template's form (if any) follows, then the output filename and body are
rendered with Go's [`text/template`](https://pkg.go.dev/text/template).

## Build and install

```console
$ go build -o templ .
$ install -m 0755 templ ~/.local/bin/templ   # or anywhere on $PATH
```

Requires Go 1.27 or newer.

`templ --version` (or `-v`) prints the build version. A plain `go build` or
`go install` reports the module or VCS version Go records in the binary; to
stamp a release version, set it at build time:

```console
$ go build -ldflags "-X main.version=v1.0.0" -o templ .
```

## Configuration layout

The template root is, in order of precedence:

1. `--config-dir <path>`
2. `$XDG_CONFIG_HOME/templ`, when `XDG_CONFIG_HOME` is an absolute path
3. `~/.config/templ`

Inside the root, immediate child directories are categories and regular files
directly inside a category are templates:

```
~/.config/templ/
├── notes/
│   └── meeting-notes.md
└── service/
    └── go-service.tmpl
```

Discovery is not recursive. Symlinked categories and templates are followed, so
a root managed with GNU stow or a similar tool works; a dangling symlink is an
error. Dot-prefixed
directories and files are ignored, so `.git` or `.DS_Store` entries never break
discovery. Categories and templates are listed sorted by display name, with the
filename as a tie-breaker.
A missing root, a root without categories, a category without templates, an
unreadable template, or malformed metadata all stop the command with a message
naming the offending path before any prompt or output file exists.

Try the bundled examples without touching your own configuration:

```console
$ templ --config-dir ./examples/templates
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
outside the declared options, and template syntax errors in `output` or the
body all fail early.

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

There are no custom functions, shell commands, or hooks. Undefined variables are
errors (`missingkey=error`), never silently empty. Both parts are parsed when
templates are discovered, before any prompt, and rendered entirely before the
destination file is touched. Errors name whether they came from the metadata,
the output filename, or the body.

## Non-interactive use

The prompts need a terminal. When stdin is not a terminal, `templ` stops with
an error instead of stalling, and suggests setting `TERM=dumb`. In that mode
[Huh](https://github.com/charmbracelet/huh) answers one line per prompt from
stdin, which makes the command scriptable:

```console
$ printf '2\n1\nmyservice\n\n\n\n\n' | TERM=dumb templ --config-dir ./examples/templates
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

Errors and the cancellation notice go to stderr; the `Created <path>` line goes
to stdout.

## Safety

- The rendered output must be a single filename directly in the current working
  directory. Absolute paths, `..`, `.`, nested paths, path separators, control
  characters, and host-invalid characters are rejected. Unsafe names are never
  normalized into a different destination.
- Existing destinations are never overwritten — files, directories, and symlinks
  (including dangling ones) are all refused. There is no overwrite mode.
- Creation uses exclusive creation, so a file that appears between validation
  and writing is not overwritten either.
- Cancelling any prompt, invalid metadata, failed form validation, and rendering
  errors leave no output behind. If writing fails after the file was created,
  the incomplete file is removed; pre-existing files are never removed.

## Manual terminal smoke test

Build the binary and work in a scratch directory with the bundled templates:

```console
$ go build -o /tmp/templ .
$ mkdir -p /tmp/templ-smoke && cd /tmp/templ-smoke
$ /tmp/templ --config-dir /path/to/templ/examples/templates
```

1. **Selection:** the category prompt lists `notes` and `service`. Navigate with
   the arrow keys or `j`/`k` and press `enter` on `notes`. `Meeting notes` is the
   category's only template, so it is picked without a prompt. No form appears; `meeting-notes.md` is created and its path is
   printed. Check that the file exists and does not contain `---` front matter.
2. **Form navigation:** run again and choose `service`.
   The form shows one field per screen, so press `enter` to advance to the next.
   Fill the input (try submitting it empty first to see the required error),
   submit the multiline text with `enter` (`alt+enter` or `ctrl+j` inserts a
   newline), change the select with `enter`, toggle multiselect options with
   `space` and confirm with `enter`, and answer the confirm with `y`/`n` or
   `enter`. The file is named from the `name` answer and contains the rendered
   body.
3. **Cancellation:** run again and press `ctrl+c` at the category prompt, then
   again inside the form. Each time the command
   prints `Cancelled. No file was created.` and `echo $?` shows `130`;
   `ls -A` shows nothing new.
4. **Conflict:** with `meeting-notes.md` already present, run the notes template
   again. The command reports `destination already exists` and the file is
   untouched.

## Development

```console
$ go test ./...
```

The packages are kept independently testable: `internal/catalog` (discovery and
metadata), `internal/generate` (filename validation and exclusive creation), and
`internal/cli` (template rendering and the Huh-driven workflow, tested through a
fake prompter).
