package server

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"

	"github.com/creack/pty"
	"golang.org/x/crypto/ssh"
)

// Session handles an SSH session
type Session struct {
	channel ssh.Channel
	ptmx    *os.File
	cmd     *exec.Cmd
}

// NewSession creates a new session
func NewSession(channel ssh.Channel) *Session {
	return &Session{
		channel: channel,
	}
}

// handleRequest processes a single request
func (s *Session) handleRequest(req *ssh.Request) error {
	switch req.Type {
	case "pty-req":
		return s.handlePTY(req)
	case "shell":
		return s.handleShell(req)
	case "exec":
		return s.handleExec(req)
	case "window-change":
		return s.handleWindowChange(req)
	default:
		if req.WantReply {
			req.Reply(false, nil)
		}
		return nil
	}
}

// handlePTY handles a PTY request
func (s *Session) handlePTY(req *ssh.Request) error {
	if s.ptmx != nil {
		return fmt.Errorf("PTY already allocated")
	}

	var ptyReq struct {
		Term   string
		Width  uint32
		Height uint32
		Modes  []byte
	}
	if err := ssh.Unmarshal(req.Payload, &ptyReq); err != nil {
		log.Printf("Failed to parse PTY payload: %v", err)
		if req.WantReply {
			req.Reply(false, nil)
		}
		return err
	}

	// Create command
	shell := getDefaultShell()
	cmd := exec.Command(shell, "-l") // Add -l flag for login shell
	cmd.Env = append(os.Environ(), fmt.Sprintf("TERM=%s", ptyReq.Term))

	// Start command with PTY
	ptmx, err := pty.Start(cmd)
	if err != nil {
		log.Printf("Failed to start command with PTY: %v", err)
		if req.WantReply {
			req.Reply(false, nil)
		}
		return err
	}

	s.ptmx = ptmx
	s.cmd = cmd

	// Set initial window size
	if err := pty.Setsize(ptmx, &pty.Winsize{
		Rows: uint16(ptyReq.Height),
		Cols: uint16(ptyReq.Width),
	}); err != nil {
		log.Printf("Failed to set initial window size: %v", err)
	}

	if req.WantReply {
		req.Reply(true, nil)
	}
	return nil
}

// handleShell handles a shell request
func (s *Session) handleShell(req *ssh.Request) error {
	if req.WantReply {
		req.Reply(true, nil)
	}

	if s.ptmx == nil {
		// Non-PTY session
		cmd := exec.Command(getDefaultShell())
		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr

		stdin, err := cmd.StdinPipe()
		if err != nil {
			return fmt.Errorf("failed to create stdin pipe: %w", err)
		}

		if err := cmd.Start(); err != nil {
			return fmt.Errorf("failed to start command: %w", err)
		}

		// Handle I/O in goroutines
		go io.Copy(stdin, s.channel)
		go io.Copy(s.channel, &stdout)
		go io.Copy(s.channel.Stderr(), &stderr)

		// Wait for command to finish and send exit status
		go func() {
			err := cmd.Wait()
			exitCode := uint32(0)
			if err != nil {
				if exitErr, ok := err.(*exec.ExitError); ok {
					if status, ok := exitErr.Sys().(syscall.WaitStatus); ok {
						exitCode = uint32(status.ExitStatus())
					}
				}
			}
			s.channel.SendRequest("exit-status", false, ssh.Marshal(struct{ Status uint32 }{exitCode}))
			s.channel.Close()
		}()

		return nil
	}

	// PTY session - I/O is already set up by pty.Start
	var once sync.Once
	closeSession := func() {
		s.ptmx.Close()
		s.channel.Close()
	}

	// Copy from client to PTY
	go func() {
		io.Copy(s.ptmx, s.channel)
		once.Do(closeSession)
	}()

	// Copy from PTY to client
	go func() {
		io.Copy(s.channel, s.ptmx)
		once.Do(closeSession)
	}()

	// Wait for command to finish and send exit status
	go func() {
		err := s.cmd.Wait()
		exitCode := uint32(0)
		if err != nil {
			if exitErr, ok := err.(*exec.ExitError); ok {
				if status, ok := exitErr.Sys().(syscall.WaitStatus); ok {
					exitCode = uint32(status.ExitStatus())
				}
			}
		}
		s.channel.SendRequest("exit-status", false, ssh.Marshal(struct{ Status uint32 }{exitCode}))
		once.Do(closeSession)
	}()

	return nil
}

// handleExec handles an exec request
func (s *Session) handleExec(req *ssh.Request) error {
	cmdStruct := struct{ Command string }{}
	if err := ssh.Unmarshal(req.Payload, &cmdStruct); err != nil {
		if req.WantReply {
			req.Reply(false, nil)
		}
		return fmt.Errorf("failed to parse exec payload: %w", err)
	}

	log.Printf("Executing command: %s", cmdStruct.Command)

	if req.WantReply {
		req.Reply(true, nil)
	}

	// Split command into parts
	parts := strings.Fields(cmdStruct.Command)
	if len(parts) == 0 {
		return fmt.Errorf("empty command")
	}

	cmd := exec.Command(parts[0], parts[1:]...)
	cmd.Dir = "/Users/kurt/code/flyssh" // Use the workspace directory

	if s.ptmx != nil {
		s.cmd = cmd
		ptmx, err := pty.Start(cmd)
		if err != nil {
			log.Printf("Failed to start PTY command: %v", err)
			return fmt.Errorf("failed to start PTY command: %w", err)
		}
		s.ptmx = ptmx

		// Copy I/O
		go io.Copy(ptmx, s.channel)
		go io.Copy(s.channel, ptmx)

		// Wait for command to finish and send exit status
		go func() {
			err := cmd.Wait()
			exitCode := uint32(0)
			if err != nil {
				log.Printf("Command failed: %v", err)
				if exitErr, ok := err.(*exec.ExitError); ok {
					if status, ok := exitErr.Sys().(syscall.WaitStatus); ok {
						exitCode = uint32(status.ExitStatus())
					}
				}
			}
			log.Printf("Command completed with exit code: %d", exitCode)
			s.channel.SendRequest("exit-status", false, ssh.Marshal(struct{ Status uint32 }{exitCode}))
			s.channel.Close()
		}()

		return nil
	}

	// Non-PTY exec
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	// Start the command
	if err := cmd.Start(); err != nil {
		log.Printf("Failed to start command: %v", err)
		return fmt.Errorf("failed to start command: %w", err)
	}

	// Wait for command to finish and send output
	go func() {
		err := cmd.Wait()
		exitCode := uint32(0)
		if err != nil {
			if exitErr, ok := err.(*exec.ExitError); ok {
				if status, ok := exitErr.Sys().(syscall.WaitStatus); ok {
					exitCode = uint32(status.ExitStatus())
				}
			}
		}

		// Send output
		if stdout.Len() > 0 {
			s.channel.Write(stdout.Bytes())
		}
		if stderr.Len() > 0 {
			s.channel.Stderr().Write(stderr.Bytes())
		}

		// Send exit status and close
		s.channel.SendRequest("exit-status", false, ssh.Marshal(struct{ Status uint32 }{exitCode}))
		s.channel.Close()
	}()

	return nil
}

// handleWindowChange handles a window-change request
func (s *Session) handleWindowChange(req *ssh.Request) error {
	if s.ptmx == nil {
		return nil // Ignore if no PTY
	}

	var winSize struct {
		Width  uint32
		Height uint32
	}
	if err := ssh.Unmarshal(req.Payload, &winSize); err != nil {
		return fmt.Errorf("failed to parse window size: %w", err)
	}

	return pty.Setsize(s.ptmx, &pty.Winsize{
		Rows: uint16(winSize.Height),
		Cols: uint16(winSize.Width),
	})
}

// Close cleans up the session resources
func (s *Session) Close() error {
	if s.ptmx != nil {
		s.ptmx.Close()
	}
	if s.cmd != nil && s.cmd.Process != nil {
		s.cmd.Process.Kill()
	}
	if s.channel != nil {
		s.channel.Close()
	}
	return nil
}
