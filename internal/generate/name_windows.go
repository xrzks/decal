//go:build windows

package generate

import (
	"fmt"
	"strings"
)

// validateHostName applies Windows filename restrictions.
func validateHostName(name string) error {
	if strings.ContainsAny(name, `<>:"|?*`) {
		return fmt.Errorf("output filename %q contains characters that are not valid on Windows", name)
	}
	if strings.HasSuffix(name, ".") || strings.HasSuffix(name, " ") {
		return fmt.Errorf("output filename %q must not end with a dot or a space on Windows", name)
	}

	base := name
	if i := strings.IndexByte(base, '.'); i >= 0 {
		base = base[:i]
	}
	switch strings.ToUpper(base) {
	case "CON", "PRN", "AUX", "NUL",
		"COM1", "COM2", "COM3", "COM4", "COM5", "COM6", "COM7", "COM8", "COM9",
		"LPT1", "LPT2", "LPT3", "LPT4", "LPT5", "LPT6", "LPT7", "LPT8", "LPT9":
		return fmt.Errorf("output filename %q is a reserved device name on Windows", name)
	}
	return nil
}
