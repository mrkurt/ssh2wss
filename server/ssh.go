package server

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/exec"
	"sync"

	"github.com/creack/pty"
	"golang.org/x/crypto/ssh"
	// NEVER switch from x/term without permission - it's the official Go package for terminal handling
)

// SSHServer handles SSH connections
type SSHServer struct {
	port      int
	sshConfig *ssh.ServerConfig
}

// NewSSHServer creates a new SSH server with the given host key
func NewSSHServer(port int, hostKey []byte) (*SSHServer, error) {
	signer, err := ssh.ParsePrivateKey(hostKey)
	if err != nil {
		return nil, fmt.Errorf("failed to parse host key: %v", err)
	}

	config := &ssh.ServerConfig{
		NoClientAuth: true,
	}
	config.AddHostKey(signer)

	return &SSHServer{
		port:      port,
		sshConfig: config,
	}, nil
}

// Start starts the SSH server
func (s *SSHServer) Start(ctx context.Context) error {
	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", s.port))
	if err != nil {
		return fmt.Errorf("failed to listen on port %d: %v", s.port, err)
	}
	defer listener.Close()

	log.Printf("SSH server listening on port %d", s.port)

	// Create a channel to signal shutdown
	shutdown := make(chan struct{})
	go func() {
		<-ctx.Done()
		log.Println("SSH server context canceled, closing listener")
		listener.Close()
		log.Println("SSH server listener closed")
		close(shutdown)
	}()

	for {
		conn, err := listener.Accept()
		if err != nil {
			select {
			case <-shutdown:
				return nil // Clean shutdown
			default:
				if !errors.Is(err, net.ErrClosed) {
					log.Printf("Failed to accept connection: %v", err)
				}
				continue
			}
		}

		go s.handleConnection(conn)
	}
}

func (s *SSHServer) handleConnection(conn net.Conn) {
	defer conn.Close()

	// Perform SSH handshake
	sshConn, chans, reqs, err := ssh.NewServerConn(conn, s.sshConfig)
	if err != nil {
		log.Printf("Failed SSH handshake: %v", err)
		return
	}
	defer sshConn.Close()

	// Handle incoming requests
	go ssh.DiscardRequests(reqs)

	// Service the incoming channel
	for newChannel := range chans {
		if newChannel.ChannelType() != "session" {
			newChannel.Reject(ssh.UnknownChannelType, "unknown channel type")
			continue
		}

		channel, requests, err := newChannel.Accept()
		if err != nil {
			log.Printf("Failed to accept channel: %v", err)
			continue
		}

		// Handle channel requests
		go s.handleChannelRequests(channel, requests)
	}
}

func (s *SSHServer) handleChannelRequests(channel ssh.Channel, requests <-chan *ssh.Request) {
	var cmd *exec.Cmd
	var ptyReq bool
	var ptmx *os.File
	shell := getDefaultShell()

	defer func() {
		if cmd != nil && cmd.Process != nil {
			cmd.Process.Kill()
		}
		channel.Close()
	}()

	for req := range requests {
		ok := false

		switch req.Type {
		case "pty-req":
			ptyReq = true
			ptyPayload := struct {
				Term     string
				Columns  uint32
				Rows     uint32
				Width    uint32
				Height   uint32
				Modelist string
			}{}
			if err := ssh.Unmarshal(req.Payload, &ptyPayload); err != nil {
				log.Printf("Failed to parse PTY payload: %v", err)
				ok = false
			} else {
				ok = true
			}

		case "shell":
			if req.WantReply {
				req.Reply(true, nil)
			}
			if ptyReq {
				cmd = exec.Command(shell)
				cmd.Env = append(os.Environ(), "TERM=xterm")

				// Set up process attributes before starting PTY
				setupProcessAttributes(cmd, true)

				// Create PTY
				ptmx, err := pty.Start(cmd)
				if err != nil {
					log.Printf("Failed to start command with PTY: %v", err)
					return
				}
				defer ptmx.Close()

				// Copy PTY input/output
				go func() {
					io.Copy(ptmx, channel)
				}()
				go func() {
					io.Copy(channel, ptmx)
				}()

				// Wait for command to finish
				err = cmd.Wait()
				if err != nil {
					if exitErr, ok := err.(*exec.ExitError); ok {
						if status, ok := getExitStatus(exitErr); ok {
							channel.SendRequest("exit-status", false, ssh.Marshal(struct{ Status uint32 }{status}))
						}
					}
				} else {
					channel.SendRequest("exit-status", false, ssh.Marshal(struct{ Status uint32 }{0}))
				}
				return
			} else {
				cmd = exec.Command(shell, getShellArgs(true)...)
			}
			s.handleShell(channel, cmd)
			return

		case "exec":
			cmdStruct := struct{ Command string }{}
			if err := ssh.Unmarshal(req.Payload, &cmdStruct); err != nil {
				log.Printf("Failed to parse exec payload: %v", err)
				if req.WantReply {
					req.Reply(false, nil)
				}
				continue
			}
			args := getCommandArgs(cmdStruct.Command)
			cmd = exec.Command(shell, args...)
			if req.WantReply {
				req.Reply(true, nil)
			}
			s.handleShell(channel, cmd)
			return

		case "window-change":
			if ptyReq && cmd != nil && cmd.Process != nil {
				winChReq := struct {
					Width  uint32
					Height uint32
					X      uint32
					Y      uint32
				}{}
				if err := ssh.Unmarshal(req.Payload, &winChReq); err == nil {
					if err := setWinsize(ptmx, int(winChReq.Width), int(winChReq.Height)); err != nil {
						log.Printf("Failed to set window size: %v", err)
					}
					ok = true
				}
			}
		}

		if req.WantReply {
			req.Reply(ok, nil)
		}
	}
}

func (s *SSHServer) handleShell(channel ssh.Channel, cmd *exec.Cmd) {
	var stdin io.WriteCloser
	var stdout, stderr io.ReadCloser
	var err error

	// Set up platform-specific process attributes (non-PTY mode)
	setupProcessAttributes(cmd, false)

	// Only create pipes if they haven't been set
	if cmd.Stdin == nil {
		stdin, err = cmd.StdinPipe()
		if err != nil {
			log.Printf("Failed to create stdin pipe: %v", err)
			channel.SendRequest("exit-status", false, ssh.Marshal(struct{ Status uint32 }{1}))
			return
		}
	}
	if cmd.Stdout == nil {
		stdout, err = cmd.StdoutPipe()
		if err != nil {
			log.Printf("Failed to create stdout pipe: %v", err)
			channel.SendRequest("exit-status", false, ssh.Marshal(struct{ Status uint32 }{1}))
			return
		}
	}
	if cmd.Stderr == nil {
		stderr, err = cmd.StderrPipe()
		if err != nil {
			log.Printf("Failed to create stderr pipe: %v", err)
			channel.SendRequest("exit-status", false, ssh.Marshal(struct{ Status uint32 }{1}))
			return
		}
	}

	// Start the command
	if err := cmd.Start(); err != nil {
		log.Printf("Failed to start command: %v", err)
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

	var exitCode uint32
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			// Try to get exit code from shell first
			if shell, ok := cmd.Stdin.(interface{ GetExitCode() int }); ok {
				code := shell.GetExitCode()
				if code >= 0 {
					exitCode = uint32(code)
				} else {
					// Fallback to process state
					exitCode = uint32(exitErr.ExitCode())
				}
			} else {
				// No shell interface, use process state
				exitCode = uint32(exitErr.ExitCode())
			}
		} else {
			// Non-exit error, use code 1
			exitCode = 1
		}
	} else {
		// Command succeeded, try shell exit code first
		if shell, ok := cmd.Stdin.(interface{ GetExitCode() int }); ok {
			code := shell.GetExitCode()
			if code >= 0 {
				exitCode = uint32(code)
			} else {
				// Fallback to process state
				exitCode = uint32(cmd.ProcessState.ExitCode())
			}
		} else {
			// No shell interface, use process state
			exitCode = uint32(cmd.ProcessState.ExitCode())
		}
	}

	// Send the exit status
	channel.SendRequest("exit-status", false, ssh.Marshal(struct{ Status uint32 }{exitCode}))

	// Wait for all goroutines to finish
	wg.Wait()

	// Close the channel to ensure clean shutdown
	channel.Close()
}
