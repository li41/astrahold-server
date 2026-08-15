package main

import (
	"testing"

	"github.com/li41/astrahold-server/internal/characteridentity"
	"github.com/li41/astrahold-server/internal/session"
)

func TestE2ECharacterIdentityFactory(t *testing.T) {
	tests := []struct {
		sessionID session.ID
		wantID    string
	}{
		{sessionID: 1, wantID: e2eAttackerCharacterID},
		{sessionID: 2, wantID: e2eDefenderCharacterID},
	}
	for _, test := range tests {
		binding, err := e2eCharacterIdentityFactory(test.sessionID, 0)
		if err != nil {
			t.Fatalf("session=%d: %v", test.sessionID, err)
		}
		if binding.Assurance != characteridentity.AssuranceTrusted || string(binding.ID) != test.wantID {
			t.Fatalf("session=%d binding=%+v", test.sessionID, binding)
		}
	}
	if _, err := e2eCharacterIdentityFactory(3, 0); err == nil {
		t.Fatal("third session must be rejected")
	}
}

func TestValidateHarnessAddressLoopbackOnly(t *testing.T) {
	for _, address := range []string{"127.0.0.1:27777", "[::1]:27777"} {
		if err := validateHarnessAddress(address); err != nil {
			t.Fatalf("expected %q to be valid: %v", address, err)
		}
	}
	for _, address := range []string{"0.0.0.0:27777", "192.0.2.1:27777", "localhost:27777", "bad"} {
		if err := validateHarnessAddress(address); err == nil {
			t.Fatalf("expected %q to be rejected", address)
		}
	}
}
