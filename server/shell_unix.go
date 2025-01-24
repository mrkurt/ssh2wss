//go:build !windows
// +build !windows

package server

import (
	"os"
)

func getDefaultShell() string {
	if shell := os.Getenv("SHELL"); shell != "" {
		return shell
	}
	return "/bin/bash"
}

func getShellArgs(isLogin bool) []string {
	if isLogin {
		return []string{"-l"}
	}
	return nil
}

func getCommandArgs(command string) []string {
	return []string{"-c", command}
}
