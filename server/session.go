package server

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"syscall"

	"golang.org/x/crypto/ssh"
)

// handleChannelRequests handles requests on an SSH channel
func (s *SSHServer) handleChannelRequests(channel ssh.Channel, requests <-chan *ssh.Request) {
	var cmd *exec.Cmd
	var ptyReq bool
	var ptmx *os.File
	shell := getDefaultShell()
	var config terminalConfig

	defer func() {
		if cmd != nil && cmd.Process != nil {
			cmd.Process.Kill()
		}
		if ptmx != nil {
			ptmx.Close()
		}
		channel.Close()
	}()

	for req := range requests {
		ok := false

		switch req.Type {
		case "pty-req":
			var err error
			config, err = s.handlePTYRequest(req)
			if err != nil {
				ok = false
			} else {
				ptyReq = true
				ok = true
			}

		case "shell", "exec":
			if req.WantReply {
				req.Reply(true, nil)
			}

			var command string
			if req.Type == "exec" {
				cmdStruct := struct{ Command string }{}
				if err := ssh.Unmarshal(req.Payload, &cmdStruct); err != nil {
					s.logger.Printf("Failed to parse exec payload: %v", err)
					continue
				}
				command = cmdStruct.Command
				s.logger.Printf("Executing command: %s", command)
			}

			// Set up command
			if req.Type == "shell" {
				cmd = exec.Command(shell, getShellArgs(true)...)
			} else {
				shellCmd := prepareCommand(command, config)
				cmd = exec.Command(shell, getCommandArgs(shellCmd)...)
			}

			// Set up environment
			cmd.Env = append(os.Environ(),
				fmt.Sprintf("TERM=%s", config.Term),
				"LANG=en_US.UTF-8",
				"LC_ALL=en_US.UTF-8",
				"PATH=/usr/local/bin:/usr/bin:/bin",
				"SHELL="+shell)

			if ptyReq {
				var err error
				ptmx, err = s.setupPTY(cmd, channel, config)
				if err != nil {
					channel.SendRequest("exit-status", false, ssh.Marshal(struct{ Status uint32 }{1}))
					return
				}

				// Wait for command to finish
				err = cmd.Wait()
				s.logger.Printf("Command finished with error: %v", err)
				if err != nil {
					if exitErr, ok := err.(*exec.ExitError); ok {
						if status, ok := exitErr.Sys().(syscall.WaitStatus); ok {
							exitCode := uint32(status.ExitStatus())
							s.logger.Printf("Sending exit status: %d", exitCode)
							channel.SendRequest("exit-status", false, ssh.Marshal(struct{ Status uint32 }{exitCode}))
						}
					} else {
						s.logger.Printf("Non-exit error: %v", err)
						channel.SendRequest("exit-status", false, ssh.Marshal(struct{ Status uint32 }{1}))
					}
				} else {
					channel.SendRequest("exit-status", false, ssh.Marshal(struct{ Status uint32 }{0}))
				}
				return
			} else {
				s.handleShell(channel, cmd)
			}
			return

		case "window-change":
			if ptyReq && ptmx != nil {
				s.handleWindowChange(req, ptmx)
				ok = true
			}
		}

		if req.WantReply {
			req.Reply(ok, nil)
		}
	}
}

// handleShell handles a non-PTY shell session
func (s *SSHServer) handleShell(channel ssh.Channel, cmd *exec.Cmd) {
	s.logger.Printf("Starting shell handler")

	var stdin io.WriteCloser
	var stdout, stderr io.ReadCloser
	var err error

	// Set up platform-specific process attributes (non-PTY mode)
	setupProcessAttributes(cmd, false)

	// Only create pipes if they haven't been set
	if cmd.Stdin == nil {
		stdin, err = cmd.StdinPipe()
		if err != nil {
			s.logger.Printf("Failed to create stdin pipe: %v", err)
			channel.SendRequest("exit-status", false, ssh.Marshal(struct{ Status uint32 }{1}))
			return
		}
	}
	if cmd.Stdout == nil {
		stdout, err = cmd.StdoutPipe()
		if err != nil {
			s.logger.Printf("Failed to create stdout pipe: %v", err)
			channel.SendRequest("exit-status", false, ssh.Marshal(struct{ Status uint32 }{1}))
			return
		}
	}
	if cmd.Stderr == nil {
		stderr, err = cmd.StderrPipe()
		if err != nil {
			s.logger.Printf("Failed to create stderr pipe: %v", err)
			channel.SendRequest("exit-status", false, ssh.Marshal(struct{ Status uint32 }{1}))
			return
		}
	}

	// Start the command
	if err := cmd.Start(); err != nil {
		s.logger.Printf("Failed to start command: %v", err)
		channel.SendRequest("exit-status", false, ssh.Marshal(struct{ Status uint32 }{1}))
		return
	}

	// Copy data between pipes and SSH channel
	var wg sync.WaitGroup
	wg.Add(3)

	go func() {
		defer wg.Done()
		io.Copy(stdin, channel)
		stdin.Close()
	}()

	go func() {
		defer wg.Done()
		io.Copy(channel, stdout)
	}()

	go func() {
		defer wg.Done()
		io.Copy(channel.Stderr(), stderr)
	}()

	// Wait for command to finish
	err = cmd.Wait()
	s.logger.Printf("Command finished, checking exit status")

	var exitCode uint32
	if err != nil {
		s.logger.Printf("Command returned error: %v", err)
		if exitErr, ok := err.(*exec.ExitError); ok {
			code, ok := getExitStatus(exitErr)
			if ok {
				exitCode = code
				s.logger.Printf("Got exit code from error: %d", exitCode)
			} else {
				exitCode = 1
				s.logger.Printf("Could not get exit code from error, using: %d", exitCode)
			}
		} else {
			exitCode = 1
			s.logger.Printf("Non-exit error occurred, using exit code: %d", exitCode)
		}
	} else {
		// Try to get exit code from shell if available
		if shell, ok := cmd.Stdin.(interface{ GetExitCode() (uint32, error) }); ok {
			if code, err := shell.GetExitCode(); err == nil {
				exitCode = code
				s.logger.Printf("Got exit code from shell: %d", exitCode)
			} else {
				exitCode = uint32(cmd.ProcessState.ExitCode())
				s.logger.Printf("Using ProcessState exit code: %d", exitCode)
			}
		} else {
			exitCode = uint32(cmd.ProcessState.ExitCode())
			s.logger.Printf("Using ProcessState exit code: %d", exitCode)
		}
	}

	// Send the exit status
	s.logger.Printf("Sending exit status: %d", exitCode)
	channel.SendRequest("exit-status", false, ssh.Marshal(struct{ Status uint32 }{exitCode}))

	// Wait for all goroutines to finish
	wg.Wait()

	// Close the channel to ensure clean shutdown
	channel.Close()
	s.logger.Printf("Shell handler completed")
}

// execPayload represents the payload for an exec request
type execPayload struct {
	Command string
}

// handleExec handles an exec request
func (s *SSHServer) handleExec(channel ssh.Channel, req *ssh.Request, isPty bool, config terminalConfig) {
	var payload execPayload
	if err := ssh.Unmarshal(req.Payload, &payload); err != nil {
		s.logger.Printf("Failed to unmarshal exec payload: %v", err)
		channel.Close()
		return
	}

	s.logger.Printf("Executing command: %s", payload.Command)

	// Set up environment variables
	env := []string{
		fmt.Sprintf("TERM=%s", config.Term),
		"LANG=en_US.UTF-8",
		"LC_ALL=en_US.UTF-8",
		fmt.Sprintf("SHELL=%s", getDefaultShell()),
		fmt.Sprintf("PATH=%s", os.Getenv("PATH")),
	}

	// Create command with proper shell
	shell := getDefaultShell()
	shellCmd := prepareCommand(payload.Command, config)
	args := []string{"-l", "-i", "-c", shellCmd}
	cmd := exec.Command(shell, args...)
	cmd.Env = env

	// Set up process attributes
	setupProcessAttributes(cmd, isPty)

	// Set up I/O
	cmd.Stdin = channel
	cmd.Stdout = channel
	cmd.Stderr = channel

	// Start command
	if err := cmd.Start(); err != nil {
		s.logger.Printf("Failed to start command: %v", err)
		channel.SendRequest("exit-status", false, ssh.Marshal(struct{ Status uint32 }{Status: 1}))
		channel.Close()
		return
	}

	// Wait for command to complete
	err := cmd.Wait()
	s.logger.Printf("Command finished, checking exit status")

	var exitStatus uint32
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			status, _ := getExitStatus(exitErr)
			exitStatus = status
			s.logger.Printf("Got exit code from error: %d", exitStatus)
		} else {
			s.logger.Printf("Command returned error: %v", err)
			exitStatus = 1
		}
	}

	s.logger.Printf("Sending exit status: %d", exitStatus)
	channel.SendRequest("exit-status", false, ssh.Marshal(struct{ Status uint32 }{Status: exitStatus}))
	channel.Close()
}

// prepareCommand prepares a command for execution with terminal settings
func prepareCommand(command string, termConfig terminalConfig) string {
	// Properly escape the command for shell execution
	return fmt.Sprintf("export TERM=%s LANG=en_US.UTF-8 LC_ALL=en_US.UTF-8 COLUMNS=%d LINES=%d && %s",
		termConfig.Term,
		termConfig.Columns,
		termConfig.Rows,
		command)
}
