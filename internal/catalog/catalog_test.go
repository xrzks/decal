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

	// Neither a root-level file nor symlinks are categories or templates.
	writeTemplate(t, filepath.Join(root, "stray.md"), simpleTemplate("stray.md", "stray"))
	if err := os.Symlink(filepath.Join(root, "alpha"), filepath.Join(root, "linked")); err != nil {
		t.Fatalf("symlink category: %v", err)
	}
	if err := os.Symlink(filepath.Join(root, "alpha", "a.txt"), filepath.Join(root, "alpha", "link.txt")); err != nil {
		t.Fatalf("symlink template: %v", err)
	}

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
