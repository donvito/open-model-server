//go:build windows

package process

import (
	"os/exec"
	"syscall"
)

const createNewProcessGroup = 0x00000200

var (
	kernel32                    = syscall.NewLazyDLL("kernel32.dll")
	procGenerateConsoleCtrlEvnt = kernel32.NewProc("GenerateConsoleCtrlEvent")
)

// setSysProcAttr puts the child in its own process group. Windows has no
// signals, but a new group means Ctrl-C in our console does not race our
// graceful shutdown, and it lets us address the child with a console event.
func setSysProcAttr(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: createNewProcessGroup}
}

// signalStop sends CTRL_BREAK to the child's process group, which llama-server
// handles as a request to shut down. It only works when the server itself owns
// a console; when it does not (a service, or a GUI-subsystem parent) we report
// that no graceful stop was possible so the caller kills the process instead.
func signalStop(cmd *exec.Cmd) (bool, error) {
	pid := cmd.Process.Pid
	if pid <= 0 {
		// Group 0 means "every process in our own group" — never send that.
		return false, nil
	}
	// The child is its own group leader (see setSysProcAttr), so the group id
	// is the pid.
	r, _, _ := procGenerateConsoleCtrlEvnt.Call(uintptr(syscall.CTRL_BREAK_EVENT), uintptr(pid))
	return r != 0, nil
}

// exitCodeNote translates the NTSTATUS values Windows reports as exit codes
// when a child dies before it can print anything useful. Missing DLLs are by
// far the most common way a llama-server build fails to start here.
func exitCodeNote(code int) string {
	switch uint32(code) {
	case 0xC0000135:
		return "a required DLL was not found; check that the runtime DLLs ship next to the executable"
	case 0xC0000139:
		return "a DLL entry point was missing; the executable and its DLLs are mismatched versions"
	case 0xC000007B:
		return "a DLL has the wrong architecture; mixing 32-bit and 64-bit builds"
	case 0xC0000005:
		return "access violation"
	case 0xC000001D:
		return "illegal instruction; the build requires CPU features this machine lacks"
	case 0xC0000409:
		return "stack buffer overrun"
	case 0xC00000FD:
		return "stack overflow"
	}
	return ""
}
