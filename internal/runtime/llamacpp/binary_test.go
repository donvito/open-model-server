package llamacpp

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func notFound(string) (string, error) { return "", errors.New("not found") }

// binaryName is what an installed llama-server is called on this platform.
func binaryName() string {
	if runtime.GOOS == "windows" {
		return "llama-server.exe"
	}
	return "llama-server"
}

func writeExecutable(t *testing.T, path string) {
	t.Helper()
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
}

func TestBinaryPathExplicit(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, binaryName())
	writeExecutable(t, bin)

	// Both separators name a path, on every platform.
	for _, given := range []string{bin, strings.ReplaceAll(bin, `\`, "/")} {
		r := New(Options{Binary: given, LookPath: notFound}, nil, nil, nil)
		got, err := r.BinaryPath()
		if err != nil {
			t.Fatalf("%s: %v", given, err)
		}
		if !strings.EqualFold(got, bin) {
			t.Fatalf("%s: resolved to %s, want %s", given, got, bin)
		}
	}
}

// A path configured without ".exe" must still find llama-server.exe on Windows.
func TestBinaryPathAddsExecutableExtension(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("windows-only extension handling")
	}
	dir := t.TempDir()
	writeExecutable(t, filepath.Join(dir, "llama-server.exe"))
	r := New(Options{Binary: filepath.Join(dir, "llama-server"), LookPath: notFound}, nil, nil, nil)
	got, err := r.BinaryPath()
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(dir, "llama-server.exe"); got != want {
		t.Fatalf("resolved to %s, want %s", got, want)
	}
}

func TestBinaryPathMissing(t *testing.T) {
	r := New(Options{Binary: filepath.Join(t.TempDir(), "llama-server"), LookPath: notFound}, nil, nil, nil)
	if _, err := r.BinaryPath(); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("err = %v", err)
	}
	bare := New(Options{Binary: "llama-server", LookPath: notFound}, nil, nil, nil)
	if _, err := bare.BinaryPath(); err == nil || !strings.Contains(err.Error(), "PATH") {
		t.Fatalf("err = %v", err)
	}
}

func TestIsPathLike(t *testing.T) {
	paths := []string{"./llama-server", "bin/llama-server", `bin\llama-server`, `C:\tools\llama-server.exe`, "C:/tools/llama-server.exe", "/usr/bin/llama-server"}
	for _, p := range paths {
		if !isPathLike(p) {
			t.Errorf("%s should be treated as a path", p)
		}
	}
	for _, p := range []string{"llama-server", "llama-server.exe"} {
		if isPathLike(p) {
			t.Errorf("%s should be looked up in PATH", p)
		}
	}
}
