package sessioncredential

import (
	"context"
	"errors"
	"time"
)

var (
	ErrInvalidIssuedCredential = errors.New("sessioncredential: invalid issued credential")
)

// IssuedCredential is the one-time plaintext result returned by a trusted
// credential issuer. Credential is an opaque bearer value intended to be
// transported over the existing trusted TLS bootstrap and then submitted in the
// ASTRAH1 preface. Server-owned identity/takeover claims are deliberately absent.
type IssuedCredential struct {
	Credential string
	ExpiresAt  time.Time
}

func (c IssuedCredential) Valid() bool {
	return c.Credential != "" && !c.ExpiresAt.IsZero()
}

// Issuer mints one opaque short-lived session credential from already trusted,
// server-owned claims. Implementations must not encode those claims into the
// client-visible bearer value.
type Issuer interface {
	Issue(context.Context, Grant) (IssuedCredential, error)
}

// Revoker invalidates an opaque issued credential. The bool result reports
// whether a currently known credential was retired; callers should preserve
// fail-closed semantics regardless of that value.
type Revoker interface {
	Revoke(context.Context, []byte) (bool, error)
}
