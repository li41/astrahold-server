package worldruntime

import (
	"context"
	"errors"
	"fmt"

	"github.com/li41/astrahold-server/internal/character"
	"github.com/li41/astrahold-server/internal/characteridentity"
	"github.com/li41/astrahold-server/internal/movement"
	"github.com/li41/astrahold-server/internal/session"
)

var (
	ErrCharacterOwnershipNotActive                = errors.New("worldruntime: trusted character ownership not active")
	ErrCharacterOwnershipTransferRequiresTrusted  = errors.New("worldruntime: ownership transfer requires trusted identity")
	ErrCharacterOwnershipTransferIdentityMismatch = errors.New("worldruntime: ownership transfer character identity mismatch")
	ErrCharacterOwnershipTransferEntityMismatch   = errors.New("worldruntime: ownership transfer entity mismatch")
	ErrCharacterOwnershipTransferSameSession      = errors.New("worldruntime: ownership transfer requires a new session")
)

// currentOwnership returns the exact active trusted ownership fence. It is called only by
// the world owner so transport integrations never race on ownership maps.
func (r *characterIdentityRegistry) currentOwnership(identity characteridentity.Binding) (SessionOwnershipFence, error) {
	if !identity.Valid() || identity.Assurance != characteridentity.AssuranceTrusted {
		return SessionOwnershipFence{}, ErrCharacterOwnershipTransferRequiresTrusted
	}
	fence, ok := r.ownershipByCharacter[identity.ID]
	if !ok || !fence.Valid() {
		return SessionOwnershipFence{}, fmt.Errorf("%w: character=%s", ErrCharacterOwnershipNotActive, identity.ID)
	}
	if bySession, ok := r.ownershipBySession[fence.SessionID]; !ok || bySession != fence {
		return SessionOwnershipFence{}, fmt.Errorf("%w: character=%s session=%d epoch=%d", ErrCharacterOwnershipFenceStale, fence.CharacterID, fence.SessionID, fence.Epoch)
	}
	return fence, nil
}

// AwaitCharacterOwnership places a read barrier in the world-owner FIFO and returns the
// exact active ownership fence for one trusted CharacterID. The returned fence is an
// optimistic CAS expectation; it does not reserve ownership and may be stale by transfer time.
func (r *Runtime) AwaitCharacterOwnership(ctx context.Context, identity characteridentity.Binding) (SessionOwnershipFence, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	select {
	case <-ctx.Done():
		return SessionOwnershipFence{}, ctx.Err()
	default:
	}
	result := SessionOwnershipFence{}
	completion := make(chan error, 1)
	if err := r.queue.tryPush(ownershipLookupCommand{identity: identity, result: &result, completion: completion}); err != nil {
		return SessionOwnershipFence{}, err
	}
	if err := <-completion; err != nil {
		return SessionOwnershipFence{}, err
	}
	return result, nil
}

// OwnershipTransferRequest atomically replaces the network Session that owns an already
// active trusted CharacterID while preserving the existing EntityID and all entity-scoped
// authoritative gameplay state. Expected must equal the current S3-F.18 ownership fence.
type OwnershipTransferRequest struct {
	Expected    SessionOwnershipFence
	Replacement *session.Session
	Result      *SessionOwnershipFence
}

// AwaitOwnershipTransfer queues one mutating compare-and-swap handoff. Once queued, the
// caller waits for world-owner completion so it cannot abandon a transfer that already
// replaced the authoritative Session.
func (r *Runtime) AwaitOwnershipTransfer(ctx context.Context, expected SessionOwnershipFence, replacement *session.Session) (SessionOwnershipFence, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	select {
	case <-ctx.Done():
		return SessionOwnershipFence{}, ctx.Err()
	default:
	}
	if !expected.Valid() {
		return SessionOwnershipFence{}, ErrCharacterOwnershipFenceInvalid
	}
	result := SessionOwnershipFence{}
	completion := make(chan error, 1)
	request := OwnershipTransferRequest{Expected: expected, Replacement: replacement, Result: &result}
	if err := r.queue.tryPush(ownershipTransferCommand{request: request, completion: completion}); err != nil {
		return SessionOwnershipFence{}, err
	}
	if err := <-completion; err != nil {
		return SessionOwnershipFence{}, err
	}
	return result, nil
}

// applyOwnershipTransfer is deliberately session-only. It does not despawn/re-spawn the
// entity, capture Character State, restore durable data, or close the old transport. FIFO
// ordering defines the handoff boundary: commands before this operation belong to the old
// owner; fenced old-owner commands after it fail against the newly activated epoch.
func (r *Runtime) applyOwnershipTransfer(request OwnershipTransferRequest) error {
	expected := request.Expected
	replacement := request.Replacement
	if !expected.Valid() {
		return ErrCharacterOwnershipFenceInvalid
	}
	if replacement == nil || !replacement.CharacterIdentity.Valid() {
		return session.ErrInvalidSession
	}
	if replacement.CharacterIdentity.Assurance != characteridentity.AssuranceTrusted {
		return ErrCharacterOwnershipTransferRequiresTrusted
	}
	if replacement.CharacterIdentity.ID != expected.CharacterID {
		return fmt.Errorf("%w: expected=%s replacement=%s", ErrCharacterOwnershipTransferIdentityMismatch, expected.CharacterID, replacement.CharacterIdentity.ID)
	}
	if replacement.EntityID != expected.EntityID {
		return fmt.Errorf("%w: expected=%d replacement=%d", ErrCharacterOwnershipTransferEntityMismatch, expected.EntityID, replacement.EntityID)
	}
	if replacement.ID == expected.SessionID {
		return ErrCharacterOwnershipTransferSameSession
	}
	if err := r.characterIdentities.validateOwnership(expected.SessionID, expected); err != nil {
		return err
	}
	if err := r.characterIdentities.validateSession(replacement); err != nil {
		return err
	}
	currentSession, ok := r.sessions.Get(expected.SessionID)
	if !ok {
		return session.ErrSessionNotFound
	}
	if currentSession.EntityID != expected.EntityID || currentSession.CharacterIdentity.ID != expected.CharacterID {
		return ErrCharacterOwnershipFenceStale
	}
	if _, exists := r.sessions.Get(replacement.ID); exists {
		return session.ErrSessionExists
	}
	if _, ok := r.world.Entity(expected.EntityID); !ok {
		return ErrSessionEntityNotFound
	}
	if _, ok := r.characters.State(expected.EntityID); !ok {
		return character.ErrCharacterNotFound
	}

	newOwnership, err := r.characterIdentities.prepareOwnership(replacement)
	if err != nil {
		return err
	}

	// Do not carry a previous transport owner's held movement intent across the handoff.
	// This is the only entity-scoped gameplay mutation performed by the transfer seam.
	if err := r.world.SetMoveInput(expected.EntityID, movement.Input{}); err != nil {
		return err
	}

	// Add-first keeps the current owner intact if the replacement Session is invalid or
	// already present. The following Remove cannot race inside the single world owner.
	if err := r.sessions.Add(replacement); err != nil {
		return err
	}
	removed, err := r.sessions.Remove(expected.SessionID)
	if err != nil {
		_, _ = r.sessions.Remove(replacement.ID)
		return err
	}
	if removed != currentSession {
		_, _ = r.sessions.Remove(replacement.ID)
		_ = r.sessions.Add(currentSession)
		return ErrCharacterOwnershipFenceStale
	}

	// Session-scoped delivery truth must restart for the replacement connection while all
	// entity/character-scoped gameplay and persistence truth remains in place.
	r.replication.Remove(expected.SessionID)
	r.removeSessionVitals(expected.SessionID)
	delete(r.sessionDynamicRevision, expected.SessionID)
	r.replication.Register(replacement.ID)

	// Activate the new epoch before removing the old by-session entry. The generation-fenced
	// removal cannot clear the newly installed by-character ownership.
	r.characterIdentities.activateOwnership(newOwnership)
	r.characterIdentities.removeOwnershipBySession(expected.SessionID)
	if request.Result != nil {
		*request.Result = newOwnership
	}
	return nil
}
