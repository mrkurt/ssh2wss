//go:build !windows
// +build !windows

package server

import (
	"os"
)

func getDefaultShell() string {
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/bash"
	}
	return shell
}

func getShellArgs(isLogin bool) []string {
	if isLogin {
		return []string{"-l"}
	}
	return []string{}
}

func getCommandArgs(command string) []string {
	// No need to escape quotes since we're using exec.Command directly
	return []string{"-l", "-i", "-c", command}
}
