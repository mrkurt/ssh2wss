//go:build windows
// +build windows

package server

import (
	"os/exec"
	"syscall"
)

func setupProcessAttributes(cmd *exec.Cmd, isPty bool) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{
			// Windows process attributes for console handling
			CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP,
			// HideWindow ensures the process doesn't create a visible window
			HideWindow: true,
		}
	}
}
