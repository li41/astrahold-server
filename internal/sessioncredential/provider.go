// Package sessioncredential defines the server-side contract between an opaque
// session credential and the trusted character claims selected by a credential
// provider.
package sessioncredential

import (
	"context"
	"errors"
	"time"

	"github.com/li41/astrahold-server/internal/characteridentity"
)

var (
	ErrInvalidCredentialLifecycle = errors.New("sessioncredential: invalid credential lifecycle")
	ErrCredentialNotYetValid       = errors.New("sessioncredential: credential not yet valid")
	ErrCredentialExpired           = errors.New("sessioncredential: credential expired")
	ErrCredentialRevoked           = errors.New("sessioncredential: credential revoked")
)

// Grant is the server-owned result of validating one opaque session credential.
// Identity must be trusted. AllowActiveTakeover is a claim from the same
// credential validation result; callers may use it to construct a
// connection-scoped takeover authorizer without exposing transport types to the
// provider.
//
// RevocationScope is an optional opaque, server-owned identifier for the exact
// credential proof/policy generation that produced this grant. It is never sent
// to the Client. Providers that support live session invalidation should keep the
// value stable while the credential record is unchanged and change it whenever a
// security-relevant credential record changes.
//
// AuthenticationSubject and AuthenticationGeneration are optional Server-only
// provenance for credentials issued from an account-authentication layer. They
// are never sent to the Client and do not grant gameplay authority. An issuance
// runtime may retain them so a password rotation, account disable, or other
// account-proof generation change can revoke already-issued game credentials.
type Grant struct {
	Identity                 characteridentity.Binding
	AllowActiveTakeover      bool
	RevocationScope          string
	AuthenticationSubject    string
	AuthenticationGeneration string
}

func (g Grant) Valid() bool {
	return g.Identity.Valid() && g.Identity.Assurance == characteridentity.AssuranceTrusted
}

// Lifecycle defines admission-time validity for one credential. Zero-valued
// boundaries are unbounded. NotBefore is inclusive; ExpiresAt and RevokedAt are
// exclusive upper bounds in the sense that a credential is rejected at exactly
// either cutoff.
//
// Lifecycle is intentionally independent from a live admitted session. A
// provider validates these boundaries when Resolve runs; a runtime credential
// controller may additionally use the same boundaries to retire already-admitted
// sessions through the transport's normal teardown path.
type Lifecycle struct {
	NotBefore time.Time
	ExpiresAt time.Time
	RevokedAt time.Time
}

func (l Lifecycle) Validate() error {
	if !l.NotBefore.IsZero() && !l.ExpiresAt.IsZero() && !l.ExpiresAt.After(l.NotBefore) {
		return ErrInvalidCredentialLifecycle
	}
	return nil
}

func (l Lifecycle) ValidateAt(now time.Time) error {
	if now.IsZero() {
		return ErrInvalidCredentialLifecycle
	}
	if err := l.Validate(); err != nil {
		return err
	}
	if !l.NotBefore.IsZero() && now.Before(l.NotBefore) {
		return ErrCredentialNotYetValid
	}
	if !l.RevokedAt.IsZero() && !now.Before(l.RevokedAt) {
		return ErrCredentialRevoked
	}
	if !l.ExpiresAt.IsZero() && !now.Before(l.ExpiresAt) {
		return ErrCredentialExpired
	}
	return nil
}

// Provider resolves an opaque credential into trusted, server-owned claims.
//
// Resolve runs outside the world-owner tick and may perform bounded blocking
// work, but it must honor ctx. The credential buffer belongs to the caller and
// must not be retained after Resolve returns.
type Provider interface {
	Resolve(context.Context, []byte) (Grant, error)
}
