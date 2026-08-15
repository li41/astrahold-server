package sessioncredential

import (
	"errors"
	"testing"
	"time"

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

func TestLifecycleValidateAtUsesInclusiveStartAndExclusiveCutoffs(t *testing.T) {
	now := time.Date(2026, 8, 16, 0, 0, 0, 0, time.UTC)

	if err := (Lifecycle{
		NotBefore: now,
		ExpiresAt: now.Add(time.Hour),
	}).ValidateAt(now); err != nil {
		t.Fatalf("not-before boundary must be accepted: %v", err)
	}
	if err := (Lifecycle{NotBefore: now.Add(time.Second)}).ValidateAt(now); !errors.Is(err, ErrCredentialNotYetValid) {
		t.Fatalf("err=%v want not-yet-valid", err)
	}
	if err := (Lifecycle{ExpiresAt: now}).ValidateAt(now); !errors.Is(err, ErrCredentialExpired) {
		t.Fatalf("err=%v want expired", err)
	}
	if err := (Lifecycle{RevokedAt: now}).ValidateAt(now); !errors.Is(err, ErrCredentialRevoked) {
		t.Fatalf("err=%v want revoked", err)
	}
}

func TestLifecycleRejectsInvalidWindowAndZeroClock(t *testing.T) {
	start := time.Date(2026, 8, 16, 0, 0, 0, 0, time.UTC)
	for _, lifecycle := range []Lifecycle{
		{NotBefore: start, ExpiresAt: start},
		{NotBefore: start, ExpiresAt: start.Add(-time.Second)},
	} {
		if err := lifecycle.Validate(); !errors.Is(err, ErrInvalidCredentialLifecycle) {
			t.Fatalf("lifecycle=%+v err=%v want invalid lifecycle", lifecycle, err)
		}
	}
	if err := (Lifecycle{}).ValidateAt(time.Time{}); !errors.Is(err, ErrInvalidCredentialLifecycle) {
		t.Fatalf("zero clock err=%v want invalid lifecycle", err)
	}
}
