package accountrecovery

import (
	"context"
	"errors"
	"strings"
	"time"
)

const (
	MaxLoginIDBytes   = 128
	MaxAccountIDBytes = 128
	MaxRequestIDBytes = 128
	MaxProofBytes     = 256
)

var (
	ErrRejected    = errors.New("accountrecovery: rejected")
	ErrUnavailable = errors.New("accountrecovery: unavailable")
)

// Subject is Server-owned metadata bound to one public recovery challenge.
// Eligible is deliberately not exposed to the caller of the public HTTP API.
type Subject struct {
	LoginID           string
	AccountID         string
	CredentialVersion uint64
	Eligible          bool
}

func (s Subject) Valid() bool {
	return validTrimmed(s.LoginID, MaxLoginIDBytes) &&
		validTrimmed(s.AccountID, MaxAccountIDBytes) &&
		s.CredentialVersion > 0
}

type Challenge struct {
	RequestID string
	ExpiresAt time.Time
}

func (c Challenge) Valid() bool {
	return validTrimmed(c.RequestID, MaxRequestIDBytes) && !c.ExpiresAt.IsZero()
}

// Grant is returned only after a provider verifies the recovery proof. The
// account generation remains Server-owned and must be checked again by the
// durable account writer before a password mutation can commit.
type Grant struct {
	AccountID         string
	CredentialVersion uint64
}

func (g Grant) Valid() bool {
	return validTrimmed(g.AccountID, MaxAccountIDBytes) && g.CredentialVersion > 0
}

// Provider owns recovery challenge/proof verification. Implementations may
// deliver a proof through email/SMS/WebAuthn/etc., or verify a pre-provisioned
// recovery factor. Begin must not reveal Subject.Eligible through its public
// Challenge shape. Verify does not commit account state; Consume is called only
// after the durable password reset has committed successfully.
type Provider interface {
	Begin(context.Context, Subject) (Challenge, error)
	Verify(context.Context, string, []byte) (Grant, error)
	Consume(context.Context, string)
	Method() string
	Revision() string
}

func validTrimmed(value string, maxBytes int) bool {
	return value != "" && value == strings.TrimSpace(value) && len(value) <= maxBytes
}
