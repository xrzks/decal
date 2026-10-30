//go:build !windows

package generate

// validateHostName reports no additional restrictions on non-Windows hosts;
// path separators and control characters are rejected by ValidateName.
func validateHostName(string) error { return nil }
