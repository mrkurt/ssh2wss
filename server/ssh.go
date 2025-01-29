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
	logger    *log.Logger
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
		logger:    log.New(os.Stderr, "", log.LstdFlags),
	}, nil
}

// SetLogger sets a custom logger for the SSH server
func (s *SSHServer) SetLogger(logger *log.Logger) {
	s.logger = logger
}

// Start starts the SSH server
func (s *SSHServer) Start() error {
	// If port is 0, don't start listening - this server will only handle passed connections
	if s.port == 0 {
		return nil
	}

	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", s.port))
	if err != nil {
		return fmt.Errorf("failed to listen on port %d: %v", s.port, err)
	}
	defer listener.Close()

	s.logger.Printf("SSH server listening on port %d", s.port)

	for {
		conn, err := listener.Accept()
		if err != nil {
			s.logger.Printf("Failed to accept connection: %v", err)
			continue
		}

		go s.handleConnection(conn)
	}
}

// handleConnection handles a new SSH connection
func (s *SSHServer) handleConnection(conn net.Conn) {
	defer conn.Close()

	// Perform SSH handshake
	sshConn, chans, reqs, err := ssh.NewServerConn(conn, s.sshConfig)
	if err != nil {
		s.logger.Printf("Failed SSH handshake: %v", err)
		return
	}
	defer sshConn.Close()

	s.logger.Printf("New SSH connection from %s", sshConn.RemoteAddr())

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
			s.logger.Printf("Failed to accept channel: %v", err)
			continue
		}

		// Handle channel requests
		go s.handleChannelRequests(channel, requests)
	}
}

func (s *SSHServer) setupPTY(cmd *exec.Cmd, channel ssh.Channel, config terminalConfig) (*os.File, error) {
	// Set up process attributes for PTY
	setupProcessAttributes(cmd, true)

	// Create PTY
	ptmx, err := pty.Start(cmd)
	if err != nil {
		s.logger.Printf("Failed to start command with PTY: %v", err)
		return nil, err
	}

	// Set initial window size
	if config.Columns > 0 && config.Rows > 0 {
		if err := setWinsize(ptmx, config.Columns, config.Rows); err != nil {
			s.logger.Printf("Failed to set initial window size: %v", err)
			// Don't fail on window size error
		}
	} else {
		// Set default size for tests
		if err := setWinsize(ptmx, 80, 24); err != nil {
			s.logger.Printf("Failed to set default window size: %v", err)
		}
	}

	// Set up PTY copying
	go func() {
		io.Copy(ptmx, channel)
		ptmx.Close()
	}()
	go func() {
		io.Copy(channel, ptmx)
	}()

	return ptmx, nil
}

type terminalConfig struct {
	Term    string
	Columns uint32
	Rows    uint32
	Width   uint32
	Height  uint32
}

type ptyRequestMsg struct {
	Term     string
	Columns  uint32
	Rows     uint32
	Width    uint32
	Height   uint32
	Modelist string
}

func (s *SSHServer) handleChannelRequests(channel ssh.Channel, requests <-chan *ssh.Request) {
	var cmd *exec.Cmd
	var ptyReq bool
	var ptmx *os.File
	shell := getDefaultShell()
	var config struct {
		Term    string
		Columns uint32
		Rows    uint32
		Width   uint32
		Height  uint32
	}

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
			ptyReq = true
			var ptyRequest ptyRequestMsg
			if err := ssh.Unmarshal(req.Payload, &ptyRequest); err != nil {
				s.logger.Printf("Failed to parse PTY payload: %v", err)
				ok = false
			} else {
				config = struct {
					Term    string
					Columns uint32
					Rows    uint32
					Width   uint32
					Height  uint32
				}{
					Term:    ptyRequest.Term,
					Columns: ptyRequest.Columns,
					Rows:    ptyRequest.Rows,
					Width:   ptyRequest.Width,
					Height:  ptyRequest.Height,
				}
				ok = true
				s.logger.Printf("PTY requested: term=%s, cols=%d, rows=%d", config.Term, config.Columns, config.Rows)
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
				shellCmd := prepareCommand(command, terminalConfig{
					Term:    config.Term,
					Columns: config.Columns,
					Rows:    config.Rows,
					Width:   config.Width,
					Height:  config.Height,
				})
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
				ptmx, err = s.setupPTY(cmd, channel, terminalConfig{
					Term:    config.Term,
					Columns: config.Columns,
					Rows:    config.Rows,
					Width:   config.Width,
					Height:  config.Height,
				})
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
				winChReq := struct {
					Width  uint32
					Height uint32
					X      uint32
					Y      uint32
				}{}
				if err := ssh.Unmarshal(req.Payload, &winChReq); err == nil {
					if err := setWinsize(ptmx, winChReq.Width, winChReq.Height); err != nil {
						s.logger.Printf("Failed to set window size: %v", err)
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
	wg.Add(3) // Changed from 2 to 3 to include stderr

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

type execPayload struct {
	Command string
}

func prepareCommand(command string, termConfig terminalConfig) string {
	// Properly escape the command for shell execution
	return fmt.Sprintf("export TERM=%s LANG=en_US.UTF-8 LC_ALL=en_US.UTF-8 COLUMNS=%d LINES=%d && %s",
		termConfig.Term,
		termConfig.Columns,
		termConfig.Rows,
		command)
}

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
