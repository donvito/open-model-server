// Package process manages child processes and internal port allocation.
// Execution goes through the Runner interface so runtimes can be unit tested
// without spawning real binaries.
package process

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"time"
)

// Spec describes a process to start.
type Spec struct {
	Path string
	Args []string
	Env  []string
	Dir  string
	// OnLine receives each line of stdout/stderr. source is "stdout" or "stderr".
	OnLine func(source, line string)
}

// Handle is a running (or finished) process.
type Handle interface {
	PID() int
	// Done is closed once the process has exited.
	Done() <-chan struct{}
	// Err returns the exit error after Done is closed (nil for a clean exit).
	Err() error
	// Terminate asks the process to exit gracefully, escalating to a kill
	// after the timeout.
	Terminate(timeout time.Duration) error
}

// Runner starts processes.
type Runner interface {
	Start(ctx context.Context, spec Spec) (Handle, error)
}

// ExecRunner runs real OS processes.
type ExecRunner struct{}

func (ExecRunner) Start(ctx context.Context, spec Spec) (Handle, error) {
	cmd := exec.Command(spec.Path, spec.Args...)
	cmd.Dir = spec.Dir
	cmd.Env = append(os.Environ(), spec.Env...)
	setSysProcAttr(cmd)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start %s: %w", spec.Path, err)
	}
	h := &execHandle{cmd: cmd, done: make(chan struct{})}
	var wg sync.WaitGroup
	wg.Add(2)
	go pump(stdout, "stdout", spec.OnLine, &wg)
	go pump(stderr, "stderr", spec.OnLine, &wg)
	go func() {
		wg.Wait()
		err := cmd.Wait()
		h.mu.Lock()
		h.err = err
		h.mu.Unlock()
		close(h.done)
	}()
	return h, nil
}

func pump(r io.Reader, source string, onLine func(string, string), wg *sync.WaitGroup) {
	defer wg.Done()
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	for sc.Scan() {
		if onLine != nil {
			onLine(source, sc.Text())
		}
	}
}

type execHandle struct {
	cmd  *exec.Cmd
	done chan struct{}
	mu   sync.Mutex
	err  error
}

func (h *execHandle) PID() int              { return h.cmd.Process.Pid }
func (h *execHandle) Done() <-chan struct{} { return h.done }
func (h *execHandle) Err() error {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.err
}

func (h *execHandle) Terminate(timeout time.Duration) error {
	select {
	case <-h.done:
		return nil
	default:
	}
	graceful, err := signalStop(h.cmd)
	if err != nil && !errors.Is(err, os.ErrProcessDone) {
		return err
	}
	if graceful {
		select {
		case <-h.done:
			return nil
		case <-time.After(timeout):
		}
	}
	if err := h.cmd.Process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
		return err
	}
	<-h.done
	return nil
}

// ExitDescription renders a process exit error for humans.
func ExitDescription(err error) string {
	if err == nil {
		return "exited with status 0"
	}
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		if status, ok := ee.Sys().(syscall.WaitStatus); ok && status.Signaled() {
			return "terminated by signal " + status.Signal().String()
		}
		if note := exitCodeNote(ee.ExitCode()); note != "" {
			return fmt.Sprintf("exited with status %d (%s)", ee.ExitCode(), note)
		}
		return fmt.Sprintf("exited with status %d", ee.ExitCode())
	}
	return err.Error()
}
