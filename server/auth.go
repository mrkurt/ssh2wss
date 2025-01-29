package server

import (
	"fmt"
	"log"

	"golang.org/x/crypto/ssh"
)

// AuthConfig holds SSH authentication configuration
type AuthConfig struct {
	HostKey           []byte
	NoAuth            bool
	PasswordCallback  func(conn ssh.ConnMetadata, password []byte) (*ssh.Permissions, error)
	PublicKeyCallback func(conn ssh.ConnMetadata, key ssh.PublicKey) (*ssh.Permissions, error)
}

// NewDefaultAuthConfig creates a basic auth config with just a host key
func NewDefaultAuthConfig(hostKey []byte) (*AuthConfig, error) {
	return &AuthConfig{
		HostKey: hostKey,
		NoAuth:  true,
	}, nil
}

// NewAuthConfig creates a new auth config with the specified settings
func NewAuthConfig(config AuthConfig) (*AuthConfig, error) {
	if len(config.HostKey) == 0 {
		return nil, fmt.Errorf("host key is required")
	}
	return &config, nil
}

// ToServerConfig converts the auth config to an SSH server config
func (a *AuthConfig) ToServerConfig() (*ssh.ServerConfig, error) {
	signer, err := ssh.ParsePrivateKey(a.HostKey)
	if err != nil {
		return nil, fmt.Errorf("failed to parse host key: %w", err)
	}

	config := &ssh.ServerConfig{}

	// Configure authentication
	if a.NoAuth {
		config.NoClientAuth = true
	} else {
		// Password auth
		if a.PasswordCallback != nil {
			config.PasswordCallback = func(conn ssh.ConnMetadata, password []byte) (*ssh.Permissions, error) {
				log.Printf("Password auth attempt from %s", conn.RemoteAddr())
				return a.PasswordCallback(conn, password)
			}
		}

		// Public key auth
		if a.PublicKeyCallback != nil {
			config.PublicKeyCallback = func(conn ssh.ConnMetadata, key ssh.PublicKey) (*ssh.Permissions, error) {
				log.Printf("Public key auth attempt from %s", conn.RemoteAddr())
				return a.PublicKeyCallback(conn, key)
			}
		}
	}

	config.AddHostKey(signer)
	return config, nil
}
