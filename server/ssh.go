package server

import (
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/exec"
	"sync"
	"syscall"

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
func (s *SSHServer) Start() error {
	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", s.port))
	if err != nil {
		return fmt.Errorf("failed to listen on port %d: %v", s.port, err)
	}
	defer listener.Close()

	log.Printf("SSH server listening on port %d", s.port)

	for {
		conn, err := listener.Accept()
		if err != nil {
			log.Printf("Failed to accept connection: %v", err)
			continue
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

	log.Printf("New SSH connection from %s", sshConn.RemoteAddr())

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
				log.Printf("PTY requested: term=%s, cols=%d, rows=%d", ptyPayload.Term, ptyPayload.Columns, ptyPayload.Rows)
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
					log.Printf("Command failed: %v", err)
					if exitErr, ok := err.(*exec.ExitError); ok {
						if status, ok := exitErr.Sys().(syscall.WaitStatus); ok {
							channel.SendRequest("exit-status", false, ssh.Marshal(struct{ Status uint32 }{uint32(status.ExitStatus())}))
						}
					}
				} else {
					log.Printf("Command completed successfully")
					channel.SendRequest("exit-status", false, ssh.Marshal(struct{ Status uint32 }{0}))
				}
				return
			} else {
				cmd = exec.Command(shell, getShellArgs(true)...)
			}
			log.Printf("Starting shell (%s) with PTY: %v", shell, ptyReq)
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
			log.Printf("Executing command: %s", cmdStruct.Command)
			args := getCommandArgs(cmdStruct.Command)
			cmd = exec.Command(shell, args...)
			if req.WantReply {
				req.Reply(true, nil)
			}
			s.handleShell(channel, cmd)
			return

		case "window-change":
			if cmd != nil && cmd.Process != nil {
				winChReq := struct {
					Width  uint32
					Height uint32
					X      uint32
					Y      uint32
				}{}
				if err := ssh.Unmarshal(req.Payload, &winChReq); err == nil {
					log.Printf("Window size changed: %dx%d", winChReq.Width, winChReq.Height)
					if err := setWinsize(os.Stdout, int(winChReq.Width), int(winChReq.Height)); err != nil {
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
	log.Printf("Starting shell handler")

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
			return
		}
	}
	if cmd.Stdout == nil {
		stdout, err = cmd.StdoutPipe()
		if err != nil {
			log.Printf("Failed to create stdout pipe: %v", err)
			return
		}
	}
	if cmd.Stderr == nil {
		stderr, err = cmd.StderrPipe()
		if err != nil {
			log.Printf("Failed to create stderr pipe: %v", err)
			return
		}
	}

	// Start the command
	if err := cmd.Start(); err != nil {
		log.Printf("Failed to start command: %v", err)
		return
	}

	// Copy data between pipes and SSH channel
	var wg sync.WaitGroup
	wg.Add(2)

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
		io.Copy(channel.Stderr(), stderr)
	}()

	// Wait for command to finish
	err = cmd.Wait()
	if err != nil {
		log.Printf("Command failed: %v", err)
		if exitErr, ok := err.(*exec.ExitError); ok {
			if status, ok := exitErr.Sys().(syscall.WaitStatus); ok {
				channel.SendRequest("exit-status", false, ssh.Marshal(struct{ Status uint32 }{uint32(status.ExitStatus())}))
			}
		}
	} else {
		log.Printf("Command completed successfully")
		channel.SendRequest("exit-status", false, ssh.Marshal(struct{ Status uint32 }{0}))
	}

	wg.Wait()
}
