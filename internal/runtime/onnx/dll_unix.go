//go:build !windows

package onnx

// prepareLibraryLoad is a no-op outside Windows: the dynamic loader resolves a
// library's own dependencies relative to that library.
func prepareLibraryLoad(string) {}

// skipAutoDiscovery excludes nothing on Unix.
func skipAutoDiscovery(string) bool { return false }
