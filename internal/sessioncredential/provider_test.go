package sessioncredential

import (
	"testing"

	"github.com/li41/astrahold-server/internal/characteridentity"
)

func TestGrantValidRequiresTrustedIdentity(t *testing.T) {
	trusted, err := characteridentity.NewTrusted("credential-character")
	if err != nil {
		t.Fatal(err)
	}
	if !((Grant{Identity: trusted}).Valid()) {
		t.Fatal("trusted identity must produce a valid grant")
	}

	ephemeral, err := characteridentity.NewEphemeral()
	if err != nil {
		t.Fatal(err)
	}
	if (Grant{Identity: ephemeral}).Valid() {
		t.Fatal("ephemeral identity must not be accepted as a credential grant")
	}
	if (Grant{}).Valid() {
		t.Fatal("zero grant must be invalid")
	}
}
