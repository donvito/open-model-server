package process

import (
	"context"
	"os/exec"
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

func TestExecRunnerTerminate(t *testing.T) {
	if _, err := exec.LookPath("sleep"); err != nil {
		t.Skip("sleep not available")
	}
	h, err := ExecRunner{}.Start(context.Background(), Spec{Path: "sleep", Args: []string{"30"}})
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

func TestExecRunnerMissingBinary(t *testing.T) {
	_, err := ExecRunner{}.Start(context.Background(), Spec{Path: "/definitely/not/here"})
	if err == nil {
		t.Fatal("expected error for missing binary")
	}
}
