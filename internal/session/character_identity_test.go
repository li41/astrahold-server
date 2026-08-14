package session

import (
	"testing"

	"github.com/li41/astrahold-server/internal/characteridentity"
)

func TestNewIssuesEphemeralCharacterIdentity(t *testing.T) {
	conn := NewQueueConnection(1, 1)
	s, err := New(1, 1, 10, conn)
	if err != nil {
		t.Fatal(err)
	}
	if !s.CharacterIdentity.Valid() || s.CharacterIdentity.Assurance != characteridentity.AssuranceEphemeral {
		t.Fatalf("identity=%#v", s.CharacterIdentity)
	}
}

func TestNewWithCharacterIdentityPreservesTrustedBinding(t *testing.T) {
	identity, err := characteridentity.NewTrusted("character:alpha")
	if err != nil {
		t.Fatal(err)
	}
	conn := NewQueueConnection(1, 1)
	s, err := NewWithCharacterIdentity(1, 1, identity, 10, conn)
	if err != nil {
		t.Fatal(err)
	}
	if s.CharacterIdentity != identity {
		t.Fatalf("identity=%#v want=%#v", s.CharacterIdentity, identity)
	}
	if _, err := NewWithCharacterIdentity(2, 2, characteridentity.Binding{}, 10, NewQueueConnection(1, 1)); err != ErrInvalidSession {
		t.Fatalf("invalid identity err=%v", err)
	}
}
