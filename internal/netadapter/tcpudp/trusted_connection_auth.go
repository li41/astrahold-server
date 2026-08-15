package tcpudp

import (
	"context"
	"errors"
	"fmt"
	"net"
	"time"

	"github.com/li41/astrahold-server/internal/characteridentity"
	"github.com/li41/astrahold-server/internal/session"
	"github.com/li41/astrahold-server/internal/world"
)

const defaultTrustedCharacterAuthenticationTimeout = 5 * time.Second

var (
	ErrInvalidTrustedCharacterAuthenticationRequest = errors.New("tcpudp: invalid trusted character connection authentication request")
	ErrInvalidTrustedCharacterAuthenticationResult  = errors.New("tcpudp: invalid trusted character connection authentication result")
	ErrTrustedCharacterAuthenticationFailed         = errors.New("tcpudp: trusted character connection authentication failed")
)

// TrustedCharacterConnectionAuthenticationRequest is the transport-local context exposed to
// an opt-in authenticator before GameV1 bootstrap begins. Connection may be used to validate a
// transport-layer proof (for example a TLS-derived identity or a bounded custom preface). The
// authenticator must consume only its own authentication bytes and leave the connection ready
// for the normal GameV1 flow after it returns.
type TrustedCharacterConnectionAuthenticationRequest struct {
	CandidateSessionID session.ID
	AllocatedEntityID  world.EntityID
	RemoteAddress      string
	Connection         net.Conn
}

func (r TrustedCharacterConnectionAuthenticationRequest) Valid() bool {
	return r.CandidateSessionID != 0 && r.AllocatedEntityID != 0 && r.Connection != nil
}

// TrustedCharacterConnectionAuthentication is one authenticated connection result. Identity
// must be trusted. TakeoverAuthorizer is intentionally connection-scoped so any active takeover
// permission can remain bound to the same proof/claims that selected Identity. A nil authorizer
// is valid for inactive join but fails closed if this connection later attempts active takeover.
type TrustedCharacterConnectionAuthentication struct {
	Identity           characteridentity.Binding
	TakeoverAuthorizer CharacterTakeoverAuthorizer
}

func (a TrustedCharacterConnectionAuthentication) Valid() bool {
	return a.Identity.Valid() && a.Identity.Assurance == characteridentity.AssuranceTrusted
}

// TrustedCharacterConnectionAuthenticator may perform bounded blocking authentication I/O on
// request.Connection. It runs on the TCP connection goroutine, never on the world-owner tick.
type TrustedCharacterConnectionAuthenticator func(context.Context, TrustedCharacterConnectionAuthenticationRequest) (TrustedCharacterConnectionAuthentication, error)

func (s *Server) authenticateTrustedCharacterConnection(ctx context.Context, raw net.Conn, sid session.ID, allocatedEntityID world.EntityID) (TrustedCharacterConnectionAuthentication, error) {
	if s == nil || s.config.TrustedCharacterConnectionAuthenticator == nil || raw == nil {
		return TrustedCharacterConnectionAuthentication{}, ErrInvalidTrustedCharacterAuthenticationRequest
	}
	if ctx == nil {
		ctx = context.Background()
	}
	request := TrustedCharacterConnectionAuthenticationRequest{
		CandidateSessionID: sid,
		AllocatedEntityID:  allocatedEntityID,
		Connection:         raw,
	}
	if addr := raw.RemoteAddr(); addr != nil {
		request.RemoteAddress = addr.String()
	}
	if !request.Valid() {
		return TrustedCharacterConnectionAuthentication{}, ErrInvalidTrustedCharacterAuthenticationRequest
	}

	timeout := s.config.TrustedCharacterAuthenticationTimeout
	if timeout <= 0 {
		timeout = defaultTrustedCharacterAuthenticationTimeout
	}
	deadline := time.Now().Add(timeout)
	if parentDeadline, ok := ctx.Deadline(); ok && parentDeadline.Before(deadline) {
		deadline = parentDeadline
	}
	authCtx, cancel := context.WithDeadline(ctx, deadline)
	defer cancel()

	if err := raw.SetDeadline(deadline); err != nil {
		return TrustedCharacterConnectionAuthentication{}, fmt.Errorf("%w: set authentication deadline: %w", ErrTrustedCharacterAuthenticationFailed, err)
	}
	result, authErr := s.config.TrustedCharacterConnectionAuthenticator(authCtx, request)
	resetErr := raw.SetDeadline(time.Time{})
	if authErr != nil {
		if resetErr != nil {
			return TrustedCharacterConnectionAuthentication{}, errors.Join(
				fmt.Errorf("%w: %w", ErrTrustedCharacterAuthenticationFailed, authErr),
				fmt.Errorf("reset authentication deadline: %w", resetErr),
			)
		}
		return TrustedCharacterConnectionAuthentication{}, fmt.Errorf("%w: %w", ErrTrustedCharacterAuthenticationFailed, authErr)
	}
	if resetErr != nil {
		return TrustedCharacterConnectionAuthentication{}, fmt.Errorf("%w: reset authentication deadline: %w", ErrTrustedCharacterAuthenticationFailed, resetErr)
	}
	if !result.Valid() {
		return TrustedCharacterConnectionAuthentication{}, ErrInvalidTrustedCharacterAuthenticationResult
	}
	return result, nil
}
