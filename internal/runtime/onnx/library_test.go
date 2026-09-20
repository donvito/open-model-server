package onnx

import (
	"os"
	"path/filepath"
	goruntime "runtime"
	"strings"
	"testing"
)

func TestLibraryPathFindsLibraryOnPATH(t *testing.T) {
	dir := t.TempDir()
	lib := filepath.Join(dir, libraryFileName())
	if err := os.WriteFile(lib, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ONNXRUNTIME_SHARED_LIBRARY_PATH", "")
	t.Setenv("PATH", dir)

	r := New(Options{}, nil)
	got, err := r.LibraryPath()
	if err != nil {
		t.Fatal(err)
	}
	if got != lib {
		t.Fatalf("library = %s, want %s", got, lib)
	}
}

func TestLibraryFileNameMatchesPlatform(t *testing.T) {
	name := libraryFileName()
	switch goruntime.GOOS {
	case "windows":
		if name != "onnxruntime.dll" {
			t.Fatalf("name = %s", name)
		}
		// The Unix defaults must not leak into the Windows search.
		for _, d := range systemLibraryDirs() {
			if strings.HasPrefix(d, "/usr") || strings.HasPrefix(d, "/opt") {
				t.Fatalf("unix directory %s searched on windows", d)
			}
		}
	case "darwin":
		if name != "libonnxruntime.dylib" {
			t.Fatalf("name = %s", name)
		}
	default:
		if name != "libonnxruntime.so" {
			t.Fatalf("name = %s", name)
		}
	}
}

// System32 is on every Windows PATH and holds the Windows ML copy of
// onnxruntime.dll, which rejects the API version this binding asks for.
func TestSystemDirectoriesAreNotAutoDiscovered(t *testing.T) {
	if goruntime.GOOS != "windows" {
		t.Skip("windows-only")
	}
	root := os.Getenv("SystemRoot")
	if root == "" {
		t.Skip("SystemRoot not set")
	}
	t.Setenv("PATH", filepath.Join(root, "system32")+string(os.PathListSeparator)+root)
	for _, d := range searchDirs() {
		if skipAutoDiscovery(d) {
			t.Fatalf("system directory %s should not be searched", d)
		}
	}
	if skipAutoDiscovery(t.TempDir()) {
		t.Fatal("an ordinary directory should be searched")
	}
}
