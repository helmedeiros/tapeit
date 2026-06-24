package spotify

import (
	"crypto/sha256"
	"encoding/base64"
	"testing"
)

func TestPKCE(t *testing.T) {
	verifier, challenge, err := pkce()
	if err != nil {
		t.Fatalf("pkce: %v", err)
	}
	// RFC 7636 requires a 43–128 char verifier.
	if len(verifier) < 43 || len(verifier) > 128 {
		t.Errorf("verifier length %d out of range", len(verifier))
	}
	sum := sha256.Sum256([]byte(verifier))
	want := base64.RawURLEncoding.EncodeToString(sum[:])
	if challenge != want {
		t.Errorf("challenge = %q, want %q", challenge, want)
	}
}

func TestTokenExpired(t *testing.T) {
	tests := []struct {
		name    string
		expiry  func() Token
		expired bool
	}{
		{"future", func() Token { return Token{Expiry: timeAhead(10)} }, false},
		{"within skew", func() Token { return Token{Expiry: timeAhead(0)} }, true},
		{"past", func() Token { return Token{Expiry: timeAgo(10)} }, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.expiry().expired(); got != tt.expired {
				t.Errorf("expired() = %v, want %v", got, tt.expired)
			}
		})
	}
}
