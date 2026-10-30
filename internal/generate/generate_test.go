package generate

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateName(t *testing.T) {
	valid := []string{
		"main.go",
		"README.md",
		"notes (draft).txt",
		".gitignore",
		"a",
		"with space.md",
	}
	for _, name := range valid {
		t.Run("valid "+name, func(t *testing.T) {
			if err := ValidateName(name); err != nil {
				t.Fatalf("ValidateName(%q) = %v, want nil", name, err)
			}
		})
	}

	invalid := map[string]string{
		"":               "empty",
		"   ":            "empty",
		".":              "not a file name",
		"..":             "not a file name",
		"/etc/passwd":    "absolute path",
		"../escape.md":   "path separators",
		"nested/file.md": "path separators",
		`nested\file.md`: "path separators",
		"bad\nname.md":   "control characters",
	}
	for name, want := range invalid {
		t.Run("invalid "+name, func(t *testing.T) {
			err := ValidateName(name)
			if err == nil {
				t.Fatalf("ValidateName(%q) = nil, want an error containing %q", name, want)
			}
			if !strings.Contains(err.Error(), want) {
				t.Fatalf("ValidateName(%q) = %v, want it to contain %q", name, err, want)
			}
		})
	}
}

func TestWriteCreatesFile(t *testing.T) {
	dir := t.TempDir()
	path, err := Write(dir, "out.txt", []byte("hello\n"))
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	if want := filepath.Join(dir, "out.txt"); path != want {
		t.Fatalf("path = %q, want %q", path, want)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(data) != "hello\n" {
		t.Fatalf("data = %q", data)
	}
}

func TestWriteRefusesExistingDestinations(t *testing.T) {
	t.Run("file", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "out.txt")
		if err := os.WriteFile(path, []byte("original"), 0o644); err != nil {
			t.Fatalf("seed: %v", err)
		}
		if _, err := Write(dir, "out.txt", []byte("new")); !errors.Is(err, ErrExists) {
			t.Fatalf("err = %v, want ErrExists", err)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read: %v", err)
		}
		if string(data) != "original" {
			t.Fatalf("existing file was modified: %q", data)
		}
	})

	t.Run("directory", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.Mkdir(filepath.Join(dir, "out.txt"), 0o755); err != nil {
			t.Fatalf("seed: %v", err)
		}
		if _, err := Write(dir, "out.txt", []byte("new")); !errors.Is(err, ErrExists) {
			t.Fatalf("err = %v, want ErrExists", err)
		}
	})

	t.Run("symlink", func(t *testing.T) {
		dir := t.TempDir()
		target := filepath.Join(dir, "target.txt")
		if err := os.WriteFile(target, []byte("original"), 0o644); err != nil {
			t.Fatalf("seed: %v", err)
		}
		if err := os.Symlink(target, filepath.Join(dir, "out.txt")); err != nil {
			t.Fatalf("symlink: %v", err)
		}
		if _, err := Write(dir, "out.txt", []byte("new")); !errors.Is(err, ErrExists) {
			t.Fatalf("err = %v, want ErrExists", err)
		}
		data, err := os.ReadFile(target)
		if err != nil {
			t.Fatalf("read: %v", err)
		}
		if string(data) != "original" {
			t.Fatalf("symlink target was modified: %q", data)
		}
	})

	t.Run("dangling symlink", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.Symlink(filepath.Join(dir, "missing"), filepath.Join(dir, "out.txt")); err != nil {
			t.Fatalf("symlink: %v", err)
		}
		if _, err := Write(dir, "out.txt", []byte("new")); !errors.Is(err, ErrExists) {
			t.Fatalf("err = %v, want ErrExists", err)
		}
	})
}

func TestWriteRejectsUnsafeName(t *testing.T) {
	dir := t.TempDir()
	if _, err := Write(dir, "../escape.txt", []byte("x")); err == nil {
		t.Fatal("Write succeeded, want a validation error")
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(dir), "escape.txt")); err == nil {
		t.Fatal("Write created a file outside the target directory")
	}
}

func TestWriteCreationFailure(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "missing")
	if _, err := Write(dir, "out.txt", []byte("x")); err == nil {
		t.Fatal("Write succeeded, want a create error")
	}
	if _, err := os.Stat(filepath.Join(dir, "out.txt")); err == nil {
		t.Fatal("a file was left behind")
	}
}

func TestWriteRemovesIncompleteFile(t *testing.T) {
	original := writeFile
	t.Cleanup(func() { writeFile = original })
	writeFile = func(*os.File, []byte) error { return errors.New("disk full") }

	dir := t.TempDir()
	if _, err := Write(dir, "out.txt", []byte("x")); err == nil {
		t.Fatal("Write succeeded, want a write error")
	}
	if _, err := os.Stat(filepath.Join(dir, "out.txt")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("incomplete file was not removed: %v", err)
	}
}
