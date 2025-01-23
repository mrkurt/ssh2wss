package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"sync"

	"golang.org/x/crypto/ssh"
	"golang.org/x/net/websocket"
)

type Bridge struct {
	sshPort    int
	wsEndpoint string
	sshConfig  *ssh.ServerConfig
}

func NewBridge(sshPort int, wsEndpoint string, hostKey []byte) (*Bridge, error) {
	signer, err := ssh.ParsePrivateKey(hostKey)
	if err != nil {
		return nil, fmt.Errorf("failed to parse host key: %v", err)
	}

	config := &ssh.ServerConfig{
		NoClientAuth: true, // Allow any client to connect
	}
	config.AddHostKey(signer)

	return &Bridge{
		sshPort:    sshPort,
		wsEndpoint: wsEndpoint,
		sshConfig:  config,
	}, nil
}

func (b *Bridge) Start() error {
	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", b.sshPort))
	if err != nil {
		return fmt.Errorf("failed to listen on port %d: %v", b.sshPort, err)
	}
	defer listener.Close()

	log.Printf("SSH server listening on port %d", b.sshPort)
	log.Printf("Forwarding connections to WebSocket endpoint: %s", b.wsEndpoint)

	for {
		conn, err := listener.Accept()
		if err != nil {
			log.Printf("Failed to accept connection: %v", err)
			continue
		}

		go b.handleConnection(conn)
	}
}

func (b *Bridge) handleConnection(conn net.Conn) {
	defer conn.Close()

	// Perform SSH handshake
	sshConn, chans, reqs, err := ssh.NewServerConn(conn, b.sshConfig)
	if err != nil {
		log.Printf("Failed SSH handshake: %v", err)
		return
	}
	defer sshConn.Close()

	// Connect to WebSocket server
	ws, err := websocket.Dial(b.wsEndpoint, "", "http://localhost/")
	if err != nil {
		log.Printf("Failed to connect to WebSocket server: %v", err)
		return
	}
	defer ws.Close()

	log.Printf("New SSH connection from %s", sshConn.RemoteAddr())
	log.Printf("Successfully connected to WebSocket server at %s", b.wsEndpoint)

	// Handle incoming requests
	go ssh.DiscardRequests(reqs)

	// Service the incoming channel
	for newChannel := range chans {
		log.Printf("New channel request of type: %s", newChannel.ChannelType())
		if newChannel.ChannelType() != "session" {
			newChannel.Reject(ssh.UnknownChannelType, "unknown channel type")
			continue
		}

		channel, requests, err := newChannel.Accept()
		if err != nil {
			log.Printf("Failed to accept channel: %v", err)
			continue
		}

		// Use WaitGroup to ensure both copies complete
		var wg sync.WaitGroup
		wg.Add(2)

		// Channel for signaling when to stop copying
		done := make(chan struct{})
		var once sync.Once

		// Helper function to safely close the done channel
		closeOnce := func() {
			once.Do(func() {
				close(done)
			})
		}

		// Handle channel requests
		go func(in <-chan *ssh.Request) {
			for req := range in {
				log.Printf("Channel request: %s", req.Type)
				switch req.Type {
				case "shell", "exec", "pty-req", "env":
					// Accept all common SSH requests
					if req.WantReply {
						req.Reply(true, nil)
					}
					if req.Type == "exec" {
						// For exec requests, we need to handle the command
						cmd := struct{ Command string }{}
						if err := ssh.Unmarshal(req.Payload, &cmd); err != nil {
							log.Printf("Failed to unmarshal exec payload: %v", err)
							continue
						}
						log.Printf("Executing command: %s", cmd.Command)
						// Forward the command to WebSocket
						if _, err := ws.Write([]byte(cmd.Command)); err != nil {
							log.Printf("Failed to write command to WebSocket: %v", err)
							if _, err := channel.SendRequest("exit-status", false, []byte{0, 0, 0, 1}); err != nil {
								log.Printf("Failed to send error exit status: %v", err)
							}
							closeOnce()
						}
					}
				default:
					if req.WantReply {
						req.Reply(false, nil)
					}
				}
			}
		}(requests)

		// Bidirectional copy between SSH and WebSocket
		go func() {
			defer wg.Done()
			defer closeOnce()
			log.Printf("Starting SSH -> WebSocket copy")
			buf := make([]byte, 32*1024)
			for {
				select {
				case <-done:
					return
				default:
					n, err := channel.Read(buf)
					if err != nil {
						if err != io.EOF {
							log.Printf("SSH read error: %v", err)
						}
						return
					}
					if _, err := ws.Write(buf[:n]); err != nil {
						log.Printf("WebSocket write error: %v", err)
						return
					}
				}
			}
		}()

		go func() {
			defer wg.Done()
			defer closeOnce()
			log.Printf("Starting WebSocket -> SSH copy")
			buf := make([]byte, 32*1024)
			for {
				select {
				case <-done:
					return
				default:
					n, err := ws.Read(buf)
					if err != nil {
						if err != io.EOF {
							log.Printf("WebSocket read error: %v", err)
						}
						// Send error exit status if read fails
						if _, err := channel.SendRequest("exit-status", false, []byte{0, 0, 0, 1}); err != nil {
							log.Printf("Failed to send error exit status: %v", err)
						}
						return
					}
					if _, err := channel.Write(buf[:n]); err != nil {
						log.Printf("SSH write error: %v", err)
						// Send error exit status if write fails
						if _, err := channel.SendRequest("exit-status", false, []byte{0, 0, 0, 1}); err != nil {
							log.Printf("Failed to send error exit status: %v", err)
						}
						return
					}
					// Send success exit status after writing response
					if _, err := channel.SendRequest("exit-status", false, []byte{0, 0, 0, 0}); err != nil {
						log.Printf("Failed to send success exit status: %v", err)
					}
					// Close the channel after sending exit status
					channel.Close()
					return
				}
			}
		}()

		// Wait for both copies to complete
		wg.Wait()
		log.Printf("Channel closed")
	}
}

func main() {
	sshPort := flag.Int("ssh-port", 2222, "SSH server port")
	wsEndpoint := flag.String("ws-endpoint", "ws://localhost:8080", "WebSocket server endpoint")
	keyPath := flag.String("key", "host.key", "Path to SSH host key")
	flag.Parse()

	hostKey, err := os.ReadFile(*keyPath)
	if err != nil {
		log.Fatalf("Failed to read host key: %v", err)
	}

	bridge, err := NewBridge(*sshPort, *wsEndpoint, hostKey)
	if err != nil {
		log.Fatalf("Failed to create bridge: %v", err)
	}

	if err := bridge.Start(); err != nil {
		log.Fatalf("Bridge failed: %v", err)
	}
}
