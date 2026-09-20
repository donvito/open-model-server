package process

import (
	"context"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"sync"
	"testing"
	"time"
)

func TestExecRunnerCapturesOutputAndExit(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("sh not available")
	}
	var mu sync.Mutex
	var got []string
	h, err := ExecRunner{}.Start(context.Background(), Spec{
		Path: "sh",
		Args: []string{"-c", "echo out; echo err 1>&2; exit 3"},
		OnLine: func(source, line string) {
			mu.Lock()
			got = append(got, source+":"+line)
			mu.Unlock()
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-h.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("process did not exit")
	}
	mu.Lock()
	sort.Strings(got)
	mu.Unlock()
	if len(got) != 2 || got[0] != "stderr:err" || got[1] != "stdout:out" {
		t.Fatalf("captured lines = %v", got)
	}
	if desc := ExitDescription(h.Err()); desc != "exited with status 3" {
		t.Fatalf("exit description = %q", desc)
	}
}

// longRunning returns a command that stays alive long enough to be terminated.
func longRunning(t *testing.T) (string, []string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		// ping is always present and sleeps a second between attempts.
		return "ping", []string{"-n", "30", "127.0.0.1"}
	}
	if _, err := exec.LookPath("sleep"); err != nil {
		t.Skip("sleep not available")
	}
	return "sleep", []string{"30"}
}

func TestExecRunnerTerminate(t *testing.T) {
	path, args := longRunning(t)
	h, err := ExecRunner{}.Start(context.Background(), Spec{Path: path, Args: args})
	if err != nil {
		t.Fatal(err)
	}
	if h.PID() <= 0 {
		t.Fatal("expected pid")
	}
	if err := h.Terminate(2 * time.Second); err != nil {
		t.Fatal(err)
	}
	select {
	case <-h.Done():
	case <-time.After(3 * time.Second):
		t.Fatal("process did not stop after Terminate")
	}
}

// A child that ignores (or cannot receive) the graceful stop must still be
// killed once the timeout expires.
func TestExecRunnerTerminateEscalatesToKill(t *testing.T) {
	path, args := longRunning(t)
	h, err := ExecRunner{}.Start(context.Background(), Spec{Path: path, Args: args})
	if err != nil {
		t.Fatal(err)
	}
	if err := h.Terminate(0); err != nil {
		t.Fatal(err)
	}
	select {
	case <-h.Done():
	case <-time.After(3 * time.Second):
		t.Fatal("process did not stop after Terminate")
	}
	if err := h.Terminate(time.Second); err != nil {
		t.Fatalf("terminating an exited process should be a no-op: %v", err)
	}
}

func TestExecRunnerMissingBinary(t *testing.T) {
	_, err := ExecRunner{}.Start(context.Background(), Spec{Path: filepath.Join(t.TempDir(), "definitely-not-here")})
	if err == nil {
		t.Fatal("expected error for missing binary")
	}
}
