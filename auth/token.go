package auth

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
)

var (
	ErrNoToken          = errors.New("no token provided")
	ErrInvalidToken     = errors.New("invalid token")
	ErrTokenNotSet      = errors.New("expected token not set")
	ErrInvalidTokenType = errors.New("invalid token type")
)

// TokenValidator validates a token against an expected value
type TokenValidator struct {
	expectedToken string
}

// NewTokenValidator creates a new token validator with the expected token
func NewTokenValidator(expectedToken string) (*TokenValidator, error) {
	if expectedToken == "" {
		return nil, ErrTokenNotSet
	}
	return &TokenValidator{expectedToken: expectedToken}, nil
}

// ValidateToken checks if the provided token matches the expected token
func (v *TokenValidator) ValidateToken(token string) error {
	if token == "" {
		return ErrNoToken
	}
	if token != v.expectedToken {
		return ErrInvalidToken
	}
	return nil
}

// GenerateToken generates a random token for testing
func GenerateToken() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}
