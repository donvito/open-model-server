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
