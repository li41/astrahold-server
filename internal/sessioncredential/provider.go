// Package sessioncredential defines the server-side contract between an opaque
// session credential and the trusted character claims selected by a credential
// provider.
package sessioncredential

import (
	"context"

	"github.com/li41/astrahold-server/internal/characteridentity"
)

// Grant is the server-owned result of validating one opaque session credential.
// Identity must be trusted. AllowActiveTakeover is a claim from the same
// credential validation result; callers may use it to construct a
// connection-scoped takeover authorizer without exposing transport types to the
// provider.
type Grant struct {
	Identity            characteridentity.Binding
	AllowActiveTakeover bool
}

func (g Grant) Valid() bool {
	return g.Identity.Valid() && g.Identity.Assurance == characteridentity.AssuranceTrusted
}

// Provider resolves an opaque credential into trusted, server-owned claims.
//
// Resolve runs outside the world-owner tick and may perform bounded blocking
// work, but it must honor ctx. The credential buffer belongs to the caller and
// must not be retained after Resolve returns.
type Provider interface {
	Resolve(context.Context, []byte) (Grant, error)
}
