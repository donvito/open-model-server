//go:build !windows

package llamacpp

// withExecExtensions returns extra candidate names for an executable path.
// Unix executables carry no extension, so there are none.
func withExecExtensions(string) []string { return nil }
