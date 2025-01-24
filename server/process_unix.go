//go:build !windows
// +build !windows

package server

import (
	"os/exec"
	"syscall"
)

func setupProcessAttributes(cmd *exec.Cmd, isPty bool) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}

	if isPty {
		cmd.SysProcAttr.Setsid = true
		cmd.SysProcAttr.Setctty = true
		cmd.SysProcAttr.Ctty = 0
	}
}
