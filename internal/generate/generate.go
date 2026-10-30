// Package generate validates output filenames and creates files exclusively.
package generate

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// ErrExists reports that the destination already exists.
var ErrExists = errors.New("destination already exists")

// ValidateName rejects any rendered output name that is not a single plain
// filename in the current directory.
func ValidateName(name string) error {
	if strings.TrimSpace(name) == "" {
		return errors.New("output filename is empty")
	}
	if name == "." || name == ".." {
		return fmt.Errorf("output filename %q is not a file name", name)
	}
	if filepath.IsAbs(name) {
		return fmt.Errorf("output filename %q must be a filename in the current directory, not an absolute path", name)
	}
	if strings.ContainsAny(name, `/\`) {
		return fmt.Errorf("output filename %q must not contain path separators", name)
	}
	if !filepath.IsLocal(name) {
		return fmt.Errorf("output filename %q must be a filename in the current directory", name)
	}
	if strings.ContainsFunc(name, func(r rune) bool { return r < 0x20 || r == 0x7f }) {
		return fmt.Errorf("output filename %q contains control characters", name)
	}
	return validateHostName(name)
}

// writeFile is a seam for tests that need a write failure.
var writeFile = func(file *os.File, data []byte) error {
	_, err := file.Write(data)
	return err
}

// Write creates name inside dir without ever overwriting an existing entry.
// It returns the full path of the created file. On failure it removes the file
// it created, if any.
func Write(dir, name string, data []byte) (string, error) {
	if err := ValidateName(name); err != nil {
		return "", err
	}

	path := filepath.Join(dir, name)
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		if errors.Is(err, fs.ErrExist) {
			return "", fmt.Errorf("%w: %s", ErrExists, path)
		}
		return "", fmt.Errorf("cannot create %s: %w", path, err)
	}

	if err := writeFile(file, data); err != nil {
		file.Close()
		removeCreated(path)
		return "", fmt.Errorf("cannot write %s: %w", path, err)
	}
	if err := file.Close(); err != nil {
		removeCreated(path)
		return "", fmt.Errorf("cannot write %s: %w", path, err)
	}
	return path, nil
}

func removeCreated(path string) {
	if info, err := os.Lstat(path); err == nil && info.Mode().IsRegular() {
		_ = os.Remove(path)
	}
}
