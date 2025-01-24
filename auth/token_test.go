package auth

import (
	"testing"
)

func TestTokenValidator(t *testing.T) {
	tests := []struct {
		name          string
		expectedToken string
		testToken     string
		wantErr       error
	}{
		{
			name:          "Valid token",
			expectedToken: "test-token",
			testToken:     "test-token",
			wantErr:       nil,
		},
		{
			name:          "Invalid token",
			expectedToken: "test-token",
			testToken:     "wrong-token",
			wantErr:       ErrInvalidToken,
		},
		{
			name:          "Empty token",
			expectedToken: "test-token",
			testToken:     "",
			wantErr:       ErrNoToken,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			validator, err := NewTokenValidator(tt.expectedToken)
			if err != nil {
				t.Fatalf("Failed to create validator: %v", err)
			}

			err = validator.ValidateToken(tt.testToken)
			if err != tt.wantErr {
				t.Errorf("ValidateToken() error = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestNewTokenValidator(t *testing.T) {
	tests := []struct {
		name          string
		expectedToken string
		wantErr       error
	}{
		{
			name:          "Valid token",
			expectedToken: "test-token",
			wantErr:       nil,
		},
		{
			name:          "Empty token",
			expectedToken: "",
			wantErr:       ErrTokenNotSet,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewTokenValidator(tt.expectedToken)
			if err != tt.wantErr {
				t.Errorf("NewTokenValidator() error = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestGenerateToken(t *testing.T) {
	// Test that tokens are unique
	token1 := GenerateToken()
	token2 := GenerateToken()
	if token1 == token2 {
		t.Error("Generated tokens are not unique")
	}

	// Test token length (32 characters for 16 bytes hex-encoded)
	if len(token1) != 32 {
		t.Errorf("Generated token length = %d, want 32", len(token1))
	}
}
