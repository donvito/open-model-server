//go:build windows

package process

import "os/exec"

func setSysProcAttr(cmd *exec.Cmd) {}
