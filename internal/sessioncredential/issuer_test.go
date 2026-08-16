package sessioncredential

import (
	"testing"
	"time"
)

func TestIssuedCredentialValidRequiresOpaqueValueAndExpiry(t *testing.T) {
	now := time.Date(2026, 8, 16, 0, 0, 0, 0, time.UTC)
	if !(IssuedCredential{Credential: "opaque", ExpiresAt: now.Add(time.Minute)}).Valid() {
		t.Fatal("complete issued credential must be valid")
	}
	if (IssuedCredential{ExpiresAt: now.Add(time.Minute)}).Valid() {
		t.Fatal("missing bearer must be invalid")
	}
	if (IssuedCredential{Credential: "opaque"}).Valid() {
		t.Fatal("missing expiry must be invalid")
	}
}
