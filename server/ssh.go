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
	"time"
	"unsafe"

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

func getDefaultShell() string {
	if shell := os.Getenv("SHELL"); shell != "" {
		return shell
	}
	return "/bin/bash"
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

				// Create PTY
				ptmx, err := pty.Start(cmd)
				if err != nil {
					log.Printf("Failed to start command with PTY: %v", err)
					return
				}
				defer ptmx.Close()

				// Handle window size changes
				go func() {
					for req := range requests {
						if req.Type == "window-change" {
							w := &struct {
								Width  uint32
								Height uint32
								X      uint32
								Y      uint32
							}{}
							if err := ssh.Unmarshal(req.Payload, w); err != nil {
								log.Printf("Failed to parse window-change payload: %v", err)
								continue
							}
							if err := pty.Setsize(ptmx, &pty.Winsize{
								Rows: uint16(w.Height),
								Cols: uint16(w.Width),
								X:    uint16(w.X),
								Y:    uint16(w.Y),
							}); err != nil {
								log.Printf("Failed to set window size: %v", err)
							}
						}
					}
				}()

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
				cmd = exec.Command(shell, "-l")
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
			cmd = exec.Command("/bin/bash", "-c", cmdStruct.Command)
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
					setTerminalSize(os.Stdout, int(winChReq.Width), int(winChReq.Height))
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
	log.Printf("Command started successfully")

	// Create a WaitGroup to ensure all copies complete
	var wg sync.WaitGroup
	wg.Add(2) // Only wait for stdout/stderr

	// Copy stdin from SSH to command in a separate goroutine
	if stdin != nil {
		go func() {
			io.Copy(stdin, channel)
			stdin.Close()
			log.Printf("Stdin copy complete")
		}()
	}

	// Copy stdout from command to SSH
	if stdout != nil {
		go func() {
			defer func() {
				wg.Done()
				log.Printf("Stdout copy complete")
			}()
			io.Copy(channel, stdout)
		}()
	} else {
		wg.Done()
	}

	// Copy stderr from command to SSH
	if stderr != nil {
		go func() {
			defer func() {
				wg.Done()
				log.Printf("Stderr copy complete")
			}()
			io.Copy(channel.Stderr(), stderr)
		}()
	} else {
		wg.Done()
	}

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

	// Wait for stdout/stderr copies to complete with timeout
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		log.Printf("All copies complete")
	case <-time.After(100 * time.Millisecond):
		log.Printf("Copy timeout, closing channel")
	}
}

func setTerminalSize(f *os.File, w, h int) {
	ws := struct {
		rows    uint16
		cols    uint16
		xpixels uint16
		ypixels uint16
	}{
		rows: uint16(h),
		cols: uint16(w),
	}
	syscall.Syscall(syscall.SYS_IOCTL, f.Fd(), uintptr(syscall.TIOCSWINSZ), uintptr(unsafe.Pointer(&ws)))
}
