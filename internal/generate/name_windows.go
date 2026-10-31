//go:build windows

package generate

import (
	"fmt"
	"strings"
)

// validateHostName applies the Windows filename restrictions that
// filepath.IsLocal does not: reserved device names are already rejected there.
func validateHostName(name string) error {
	if strings.ContainsAny(name, `<>:"|?*`) {
		return fmt.Errorf("output filename %q contains characters that are not valid on Windows", name)
	}
	if strings.HasSuffix(name, ".") || strings.HasSuffix(name, " ") {
		return fmt.Errorf("output filename %q must not end with a dot or a space on Windows", name)
	}
	return nil
}
