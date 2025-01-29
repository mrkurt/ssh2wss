package server

import (
	"io"
	"os"
	"os/exec"

	"github.com/creack/pty"
	"golang.org/x/crypto/ssh"
)

// terminalConfig holds terminal configuration settings
type terminalConfig struct {
	Term    string
	Columns uint32
	Rows    uint32
	Width   uint32
	Height  uint32
}

// ptyRequestMsg represents a PTY request message
type ptyRequestMsg struct {
	Term     string
	Columns  uint32
	Rows     uint32
	Width    uint32
	Height   uint32
	Modelist string
}

// setupPTY sets up a PTY for command execution
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

// handlePTYRequest handles a PTY request
func (s *SSHServer) handlePTYRequest(req *ssh.Request) (terminalConfig, error) {
	var ptyRequest ptyRequestMsg
	if err := ssh.Unmarshal(req.Payload, &ptyRequest); err != nil {
		s.logger.Printf("Failed to parse PTY payload: %v", err)
		return terminalConfig{}, err
	}

	config := terminalConfig{
		Term:    ptyRequest.Term,
		Columns: ptyRequest.Columns,
		Rows:    ptyRequest.Rows,
		Width:   ptyRequest.Width,
		Height:  ptyRequest.Height,
	}

	s.logger.Printf("PTY requested: term=%s, cols=%d, rows=%d", config.Term, config.Columns, config.Rows)
	return config, nil
}

// handleWindowChange handles a window change request
func (s *SSHServer) handleWindowChange(req *ssh.Request, ptmx *os.File) error {
	winChReq := struct {
		Width  uint32
		Height uint32
		X      uint32
		Y      uint32
	}{}

	if err := ssh.Unmarshal(req.Payload, &winChReq); err != nil {
		return err
	}

	if err := setWinsize(ptmx, winChReq.Width, winChReq.Height); err != nil {
		s.logger.Printf("Failed to set window size: %v", err)
		return err
	}

	return nil
}
