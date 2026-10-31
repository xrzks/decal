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
}

// Root resolves the template root directory. An explicit override wins, then
// $XDG_CONFIG_HOME/templ, then ~/.config/templ.
func Root(override string) (string, error) {
	if override != "" {
		return filepath.Abs(override)
	}
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
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
// templates. Symlinks are not followed.
func Load(root string) ([]Category, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, fmt.Errorf("template directory %s does not exist; create it or pass --config-dir", root)
		}
		if errors.Is(err, fs.ErrPermission) {
			return nil, fmt.Errorf("cannot read template directory %s: permission denied", root)
		}
		return nil, fmt.Errorf("cannot read template directory %s: %w", root, err)
	}

	categories := make([]Category, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() || isHidden(entry.Name()) {
			continue
		}
		category, err := loadCategory(entry.Name(), filepath.Join(root, entry.Name()))
		if err != nil {
			return nil, err
		}
		categories = append(categories, category)
	}
	if len(categories) == 0 {
		return nil, fmt.Errorf("no categories found in %s; add one directory per category", root)
	}

	// Directory names are unique, so Name alone is a total order.
	slices.SortFunc(categories, func(a, b Category) int { return strings.Compare(a.Name, b.Name) })
	return categories, nil
}

// isHidden reports whether name belongs to a dot entry. Discovery ignores
// those so that editor, Finder and version-control metadata is not parsed as a
// template.
func isHidden(name string) bool { return strings.HasPrefix(name, ".") }

func loadCategory(name, dir string) (Category, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return Category{}, fmt.Errorf("cannot read category %q in %s: %w", name, filepath.Dir(dir), err)
	}

	category := Category{Name: name, Dir: dir}
	for _, entry := range entries {
		if !entry.Type().IsRegular() || isHidden(entry.Name()) {
			continue
		}
		template, err := loadTemplate(filepath.Join(dir, entry.Name()))
		if err != nil {
			return Category{}, err
		}
		category.Templates = append(category.Templates, template)
	}
	if len(category.Templates) == 0 {
		return Category{}, fmt.Errorf("category %q in %s contains no templates", name, filepath.Dir(dir))
	}

	// Display names may collide, so fall back to the filename.
	slices.SortFunc(category.Templates, func(a, b *Template) int {
		return cmp.Or(strings.Compare(a.Name, b.Name), strings.Compare(a.FileName, b.FileName))
	})
	return category, nil
}

func loadTemplate(path string) (*Template, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("template %s: cannot read: %w", path, err)
	}
	return Parse(path, data)
}
