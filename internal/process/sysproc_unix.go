//go:build !windows

package process

import (
	"os/exec"
	"syscall"
)

// setSysProcAttr puts the child in its own process group so signals sent to
// the server's group (e.g. Ctrl-C in a terminal) do not race our graceful shutdown.
func setSysProcAttr(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

// signalStop asks the child to exit gracefully. Returning false means no
// graceful mechanism was available and the caller should kill immediately.
func signalStop(cmd *exec.Cmd) (bool, error) {
	if err := cmd.Process.Signal(syscall.SIGTERM); err != nil {
		return false, err
	}
	return true, nil
}

// exitCodeNote explains an exit status; nothing to add on Unix.
func exitCodeNote(int) string { return "" }
