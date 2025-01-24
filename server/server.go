package server

import (
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"syscall"

	"github.com/creack/pty"
	"golang.org/x/crypto/ssh"
)

type SSHServer struct {
	port     int
	hostKey  ssh.Signer
	config   *ssh.ServerConfig
	listener io.Closer
}

func NewSSHServer(port int, hostKey ssh.Signer) (*SSHServer, error) {
	config := &ssh.ServerConfig{
		NoClientAuth: true,
	}
	config.AddHostKey(hostKey)

	return &SSHServer{
		port:    port,
		hostKey: hostKey,
		config:  config,
	}, nil
}

func (s *SSHServer) Start() error {
	listener, err := startListener(s.port)
	if err != nil {
		return fmt.Errorf("failed to start listener: %v", err)
	}
	s.listener = listener
	defer listener.Close()

	for {
		conn, err := acceptConnection(listener)
		if err != nil {
			log.Printf("Failed to accept connection: %v", err)
			continue
		}

		go s.handleConnection(conn)
	}
}

func (s *SSHServer) handleConnection(conn ssh.ConnMetadata) {
	// SSH handshake
	sshConn, chans, reqs, err := ssh.NewServerConn(conn, s.config)
	if err != nil {
		log.Printf("Failed SSH handshake: %v", err)
		return
	}
	defer sshConn.Close()

	log.Printf("New SSH connection from %s", sshConn.RemoteAddr())

	// Service incoming requests
	go ssh.DiscardRequests(reqs)

	// Service the incoming channels
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

		go s.handleChannel(channel, requests)
	}
}

func (s *SSHServer) handleChannel(channel ssh.Channel, requests <-chan *ssh.Request) {
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

	// Set up process attributes
	setupProcessAttributes(cmd, false)

	// Connect stdin/stdout
	stdin, err := cmd.StdinPipe()
	if err != nil {
		log.Printf("Failed to get stdin pipe: %v", err)
		return
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		log.Printf("Failed to get stdout pipe: %v", err)
		return
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		log.Printf("Failed to get stderr pipe: %v", err)
		return
	}

	// Start the command
	if err := cmd.Start(); err != nil {
		log.Printf("Failed to start command: %v", err)
		return
	}

	// Copy data in both directions
	go func() {
		io.Copy(stdin, channel)
		stdin.Close()
	}()

	go func() {
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
}
