//go:build windows

package onnx

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"
)

var procSetDllDirectory = syscall.NewLazyDLL("kernel32.dll").NewProc("SetDllDirectoryW")

// prepareLibraryLoad adds the library's own directory to the DLL search path.
// Windows resolves a DLL's dependencies against the search path of the process
// that loads it, not against the DLL's directory, so without this an
// onnxruntime.dll outside our own folder fails to find the provider DLLs that
// ship beside it.
func prepareLibraryLoad(library string) {
	dir, err := filepath.Abs(filepath.Dir(library))
	if err != nil {
		return
	}
	p, err := syscall.UTF16PtrFromString(dir)
	if err != nil {
		return
	}
	procSetDllDirectory.Call(uintptr(unsafe.Pointer(p)))
}

// skipAutoDiscovery excludes the Windows system directories from the automatic
// search. They are on every machine's PATH and System32 holds the copy of
// onnxruntime.dll that ships with Windows ML, which is version-locked to the
// OS and rejects the API version we ask for. An explicitly configured library
// is still honoured, wherever it lives.
func skipAutoDiscovery(dir string) bool {
	root := os.Getenv("SystemRoot")
	if root == "" {
		root = `C:\Windows`
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return false
	}
	abs = strings.ToLower(filepath.Clean(abs))
	root = strings.ToLower(filepath.Clean(root))
	return abs == root || strings.HasPrefix(abs, root+string(os.PathSeparator))
}
