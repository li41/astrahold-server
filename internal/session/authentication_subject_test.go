package session

import (
	"testing"

	"github.com/li41/astrahold-server/internal/characteridentity"
)

func TestTrustedSessionCarriesServerAuthenticationSubject(t *testing.T) {
	identity, err := characteridentity.NewTrusted("character:subject-test")
	if err != nil {
		t.Fatal(err)
	}
	s, err := NewWithCharacterIdentityAndAuthenticationSubject(1, 1, identity, "1", 64, NewQueueConnection(4, 4))
	if err != nil {
		t.Fatal(err)
	}
	if got := s.AuthenticationSubject(); got != "1" {
		t.Fatalf("authentication subject=%q", got)
	}
}

func TestEphemeralSessionRejectsAuthenticationSubject(t *testing.T) {
	identity, err := characteridentity.NewEphemeral()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewWithCharacterIdentityAndAuthenticationSubject(1, 1, identity, "1", 64, NewQueueConnection(4, 4)); err != ErrInvalidSession {
		t.Fatalf("err=%v want=%v", err, ErrInvalidSession)
	}
}
