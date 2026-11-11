// Package catalog discovers template files and validates their metadata.
package catalog

import (
	"cmp"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// Category is a directory full of templates.
type Category struct {
	// Name is the directory name shown to the user.
	Name string
	// Dir is the category's absolute or root-relative directory.
	Dir string
	// Templates are the category's templates, sorted by display name.
	Templates []*Template
}

// Template is a single template file.
type Template struct {
	// Path is the path of the template file.
	Path string
	// FileName is the template file's base name.
	FileName string
	// Name is the display name from the front matter, falling back to FileName.
	Name string
	// Output is the unfilled output filename template.
	Output string
	// Body is the template body without front matter.
	Body string
	// Fields are the ordered form fields, empty when the template has no form.
	Fields []Field
	// OutputFields are the keys of the fields the output filename uses, in
	// order of first use.
	OutputFields []string
}

// Root resolves the template root directory. An explicit override wins, then
// $XDG_CONFIG_HOME/templ when it is absolute, then ~/.config/templ.
func Root(override string) (string, error) {
	if override != "" {
		return filepath.Abs(override)
	}
	// The XDG spec says to ignore relative values.
	if xdg := os.Getenv("XDG_CONFIG_HOME"); filepath.IsAbs(xdg) {
		return filepath.Join(xdg, "templ"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("cannot locate the home directory; pass --config-dir instead: %w", err)
	}
	return filepath.Join(home, ".config", "templ"), nil
}

// Load discovers categories and templates below root. Immediate child
// directories are categories; regular files directly inside them are
// templates. Symlinks are followed, so dotfile managers like GNU stow work.
//
// One broken entry does not hide the others: a template or category that
// cannot be loaded is left out and reported in problems, and a category
// without templates is left out silently. err is set only when root cannot be
// read or nothing usable is left.
func Load(root string) (categories []Category, problems []error, err error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil, fmt.Errorf("template directory %s does not exist; create it or pass --config-dir", root)
		}
		if errors.Is(err, fs.ErrPermission) {
			return nil, nil, fmt.Errorf("cannot read template directory %s: permission denied", root)
		}
		return nil, nil, fmt.Errorf("cannot read template directory %s: %w", root, err)
	}

	for _, entry := range entries {
		if isHidden(entry.Name()) {
			continue
		}
		mode, err := entryMode(root, entry)
		if err != nil {
			problems = append(problems, err)
			continue
		}
		if !mode.IsDir() {
			continue
		}
		category, categoryProblems := loadCategory(entry.Name(), filepath.Join(root, entry.Name()))
		problems = append(problems, categoryProblems...)
		if len(category.Templates) > 0 {
			categories = append(categories, category)
		}
	}
	if len(categories) == 0 {
		if len(problems) > 0 {
			return nil, problems, fmt.Errorf("no usable templates in %s", root)
		}
		return nil, nil, fmt.Errorf("no templates found in %s; add one directory per category with templates in it", root)
	}

	// Directory names are unique, so Name alone is a total order.
	slices.SortFunc(categories, func(a, b Category) int { return strings.Compare(a.Name, b.Name) })
	return categories, problems, nil
}

// isHidden reports whether name belongs to a dot entry. Discovery ignores
// those so that editor, Finder and version-control metadata is not parsed as a
// template.
func isHidden(name string) bool { return strings.HasPrefix(name, ".") }

// entryMode returns the type of the file entry names, resolving symlinks.
func entryMode(dir string, entry fs.DirEntry) (fs.FileMode, error) {
	if entry.Type()&fs.ModeSymlink == 0 {
		return entry.Type(), nil
	}
	path := filepath.Join(dir, entry.Name())
	info, err := os.Stat(path)
	if err != nil {
		return 0, fmt.Errorf("cannot follow symlink %s: %w", path, err)
	}
	return info.Mode(), nil
}

// loadCategory returns the category with every template that loaded, and a
// problem for each one that did not.
func loadCategory(name, dir string) (Category, []error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return Category{}, []error{fmt.Errorf("cannot read category %q in %s: %w", name, filepath.Dir(dir), err)}
	}

	category := Category{Name: name, Dir: dir}
	var problems []error
	for _, entry := range entries {
		if isHidden(entry.Name()) {
			continue
		}
		mode, err := entryMode(dir, entry)
		if err != nil {
			problems = append(problems, err)
			continue
		}
		if !mode.IsRegular() {
			continue
		}
		template, err := loadTemplate(filepath.Join(dir, entry.Name()))
		if err != nil {
			problems = append(problems, err)
			continue
		}
		category.Templates = append(category.Templates, template)
	}

	// Display names may collide, so fall back to the filename.
	slices.SortFunc(category.Templates, func(a, b *Template) int {
		return cmp.Or(strings.Compare(a.Name, b.Name), strings.Compare(a.FileName, b.FileName))
	})
	return category, problems
}

func loadTemplate(path string) (*Template, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("template %s: cannot read: %w", path, err)
	}
	return Parse(path, data)
}
