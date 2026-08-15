package tcpudp

import (
	"context"
	"errors"
	"fmt"

	"github.com/li41/astrahold-server/internal/characteridentity"
	"github.com/li41/astrahold-server/internal/session"
	"github.com/li41/astrahold-server/internal/worldruntime"
)

var (
	ErrInvalidCharacterTakeoverRequest = errors.New("tcpudp: invalid trusted character takeover authorization request")
	ErrCharacterTakeoverUnauthorized   = errors.New("tcpudp: trusted character active takeover is not authorized")
)

// CharacterTakeoverRequest is server-internal authorization context for replacing an
// already-active trusted Session. ExpectedOwnership is the exact F.18/F.19 fence that will
// be used as the subsequent transfer CAS expectation; authorization never grants authority
// over a newer owner that may appear before transfer execution.
type CharacterTakeoverRequest struct {
	CandidateSessionID session.ID
	Identity           characteridentity.Binding
	ExpectedOwnership  worldruntime.SessionOwnershipFence
	RemoteAddress      string
}

func (r CharacterTakeoverRequest) Valid() bool {
	return r.CandidateSessionID != 0 &&
		r.Identity.Valid() &&
		r.Identity.Assurance == characteridentity.AssuranceTrusted &&
		r.ExpectedOwnership.Valid() &&
		r.ExpectedOwnership.CharacterID == r.Identity.ID
}

// CharacterTakeoverAuthorizer is an upstream authenticated policy seam. Returning nil
// explicitly authorizes only the exact ExpectedOwnership supplied in the request. Any error
// rejects the candidate before replacement Session creation or world-owner ownership transfer.
type CharacterTakeoverAuthorizer func(context.Context, CharacterTakeoverRequest) error

func (s *Server) authorizeCharacterTakeover(ctx context.Context, candidateSessionID session.ID, identity characteridentity.Binding, expected worldruntime.SessionOwnershipFence, remoteAddress string) error {
	if ctx == nil {
		ctx = context.Background()
	}
	request := CharacterTakeoverRequest{
		CandidateSessionID: candidateSessionID,
		Identity:           identity,
		ExpectedOwnership:  expected,
		RemoteAddress:      remoteAddress,
	}
	if !request.Valid() {
		return ErrInvalidCharacterTakeoverRequest
	}
	if s.config.CharacterTakeoverAuthorizer == nil {
		return ErrCharacterTakeoverUnauthorized
	}
	if err := s.config.CharacterTakeoverAuthorizer(ctx, request); err != nil {
		return fmt.Errorf("%w: %w", ErrCharacterTakeoverUnauthorized, err)
	}
	return nil
}
