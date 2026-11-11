package catalog

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRoot(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	t.Run("override", func(t *testing.T) {
		t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg"))
		root, err := Root("relative/templates")
		if err != nil {
			t.Fatalf("Root: %v", err)
		}
		if !filepath.IsAbs(root) || !strings.HasSuffix(root, filepath.Join("relative", "templates")) {
			t.Fatalf("Root = %q, want an absolute path ending in relative/templates", root)
		}
	})

	t.Run("xdg config home", func(t *testing.T) {
		xdg := filepath.Join(home, "xdg")
		t.Setenv("XDG_CONFIG_HOME", xdg)
		root, err := Root("")
		if err != nil {
			t.Fatalf("Root: %v", err)
		}
		if want := filepath.Join(xdg, "templ"); root != want {
			t.Fatalf("Root = %q, want %q", root, want)
		}
	})

	t.Run("relative xdg config home is ignored", func(t *testing.T) {
		t.Setenv("XDG_CONFIG_HOME", "relative")
		root, err := Root("")
		if err != nil {
			t.Fatalf("Root: %v", err)
		}
		if want := filepath.Join(home, ".config", "templ"); root != want {
			t.Fatalf("Root = %q, want %q", root, want)
		}
	})

	t.Run("home fallback", func(t *testing.T) {
		t.Setenv("XDG_CONFIG_HOME", "")
		root, err := Root("")
		if err != nil {
			t.Fatalf("Root: %v", err)
		}
		if want := filepath.Join(home, ".config", "templ"); root != want {
			t.Fatalf("Root = %q, want %q", root, want)
		}
	})
}

func writeTemplate(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write template: %v", err)
	}
}

func simpleTemplate(output, body string) string {
	return "---\noutput: " + output + "\n---\n" + body
}

func TestLoadDiscoversAndSorts(t *testing.T) {
	root := t.TempDir()
	writeTemplate(t, filepath.Join(root, "zeta", "z.txt"), simpleTemplate("z.txt", "zeta"))
	writeTemplate(t, filepath.Join(root, "alpha", "b.txt"), "---\nname: Same\noutput: b.txt\n---\nb")
	writeTemplate(t, filepath.Join(root, "alpha", "a.txt"), "---\nname: Same\noutput: a.txt\n---\na")
	writeTemplate(t, filepath.Join(root, "alpha", "c.txt"), "---\nname: First\noutput: c.txt\n---\nc")

	// A root-level file is not a category.
	writeTemplate(t, filepath.Join(root, "stray.md"), simpleTemplate("stray.md", "stray"))

	categories, err := Load(root)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(categories) != 2 {
		t.Fatalf("got %d categories, want 2", len(categories))
	}
	if categories[0].Name != "alpha" || categories[1].Name != "zeta" {
		t.Fatalf("categories = %q, %q; want alpha, zeta", categories[0].Name, categories[1].Name)
	}

	var names []string
	for _, template := range categories[0].Templates {
		names = append(names, template.Name+"|"+template.FileName)
	}
	want := []string{"First|c.txt", "Same|a.txt", "Same|b.txt"}
	if strings.Join(names, ",") != strings.Join(want, ",") {
		t.Fatalf("templates = %v, want %v", names, want)
	}
	if got := categories[0].Templates[0].Body; got != "c" {
		t.Fatalf("body = %q, want %q", got, "c")
	}
}

func TestLoadFollowsSymlinks(t *testing.T) {
	// GNU stow and similar tools link categories or single templates into
	// the root from a dotfiles repository.
	dotfiles := t.TempDir()
	writeTemplate(t, filepath.Join(dotfiles, "linked", "a.txt"), simpleTemplate("a.txt", "a"))
	writeTemplate(t, filepath.Join(dotfiles, "b.txt"), simpleTemplate("b.txt", "b"))

	root := t.TempDir()
	if err := os.Symlink(filepath.Join(dotfiles, "linked"), filepath.Join(root, "linked")); err != nil {
		t.Fatalf("symlink category: %v", err)
	}
	if err := os.Mkdir(filepath.Join(root, "plain"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.Symlink(filepath.Join(dotfiles, "b.txt"), filepath.Join(root, "plain", "b.txt")); err != nil {
		t.Fatalf("symlink template: %v", err)
	}

	categories, err := Load(root)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(categories) != 2 || categories[0].Templates[0].Body != "a" || categories[1].Templates[0].Body != "b" {
		t.Fatalf("categories = %+v, want linked/a.txt and plain/b.txt", categories)
	}
}

func TestLoadIgnoresDotEntries(t *testing.T) {
	root := t.TempDir()
	writeTemplate(t, filepath.Join(root, "docs", "a.md"), simpleTemplate("a.md", "a"))
	// Editor, Finder and version-control metadata must not be parsed.
	writeTemplate(t, filepath.Join(root, ".git", "HEAD"), "ref: refs/heads/main\n")
	writeTemplate(t, filepath.Join(root, "docs", ".DS_Store"), "\x00\x01binary")

	categories, err := Load(root)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(categories) != 1 || categories[0].Name != "docs" {
		t.Fatalf("categories = %+v, want only docs", categories)
	}
	if len(categories[0].Templates) != 1 || categories[0].Templates[0].FileName != "a.md" {
		t.Fatalf("templates = %+v, want only a.md", categories[0].Templates)
	}
}

func TestLoadErrors(t *testing.T) {
	t.Run("missing root", func(t *testing.T) {
		_, err := Load(filepath.Join(t.TempDir(), "missing"))
		if err == nil || !strings.Contains(err.Error(), "does not exist") {
			t.Fatalf("err = %v, want a does-not-exist error", err)
		}
	})

	t.Run("no categories", func(t *testing.T) {
		root := t.TempDir()
		writeTemplate(t, filepath.Join(root, "stray.md"), simpleTemplate("stray.md", "x"))
		_, err := Load(root)
		if err == nil || !strings.Contains(err.Error(), "no categories found") {
			t.Fatalf("err = %v, want a no-categories error", err)
		}
	})

	t.Run("empty category", func(t *testing.T) {
		root := t.TempDir()
		if err := os.Mkdir(filepath.Join(root, "empty"), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		_, err := Load(root)
		if err == nil || !strings.Contains(err.Error(), "contains no templates") {
			t.Fatalf("err = %v, want a contains-no-templates error", err)
		}
	})

	t.Run("unreadable template", func(t *testing.T) {
		root := t.TempDir()
		path := filepath.Join(root, "docs", "a.md")
		writeTemplate(t, path, simpleTemplate("a.md", "a"))
		if err := os.Chmod(path, 0o000); err != nil {
			t.Fatalf("chmod: %v", err)
		}
		t.Cleanup(func() { _ = os.Chmod(path, 0o644) })
		if _, err := os.ReadFile(path); err == nil {
			t.Skip("filesystem still allows reading a 000 file")
		}
		_, err := Load(root)
		if err == nil || !strings.Contains(err.Error(), "cannot read") {
			t.Fatalf("err = %v, want a cannot-read error", err)
		}
	})

	t.Run("dangling symlink", func(t *testing.T) {
		root := t.TempDir()
		writeTemplate(t, filepath.Join(root, "docs", "a.md"), simpleTemplate("a.md", "a"))
		link := filepath.Join(root, "docs", "gone.md")
		if err := os.Symlink(filepath.Join(root, "missing"), link); err != nil {
			t.Fatalf("symlink: %v", err)
		}
		_, err := Load(root)
		if err == nil || !strings.Contains(err.Error(), "cannot follow symlink "+link) {
			t.Fatalf("err = %v, want a symlink error naming %s", err, link)
		}
	})

	t.Run("malformed metadata names the template", func(t *testing.T) {
		root := t.TempDir()
		path := filepath.Join(root, "docs", "a.md")
		writeTemplate(t, path, "---\nname: Broken\n---\nbody")
		_, err := Load(root)
		if err == nil || !strings.Contains(err.Error(), path) || !strings.Contains(err.Error(), `"output"`) {
			t.Fatalf("err = %v, want a metadata error naming %s", err, path)
		}
	})
}

func TestLoadExamples(t *testing.T) {
	categories, err := Load(filepath.Join("..", "..", "examples", "templates"))
	if err != nil {
		t.Fatalf("Load examples: %v", err)
	}
	seen := map[string]bool{}
	for _, category := range categories {
		seen[category.Name] = true
		for _, template := range category.Templates {
			if template.Output == "" {
				t.Errorf("template %s has no output", template.Path)
			}
		}
	}
	if !seen["notes"] || !seen["service"] {
		t.Fatalf("categories = %v, want notes and service", seen)
	}
}
