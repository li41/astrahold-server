package worldruntime

import (
	"errors"
	"fmt"
	"math"

	"github.com/li41/astrahold-server/internal/characteridentity"
	"github.com/li41/astrahold-server/internal/protocol"
	"github.com/li41/astrahold-server/internal/session"
	"github.com/li41/astrahold-server/internal/world"
)

var (
	ErrCharacterOwnershipFenceInvalid = errors.New("worldruntime: character ownership fence invalid")
	ErrCharacterOwnershipFenceStale   = errors.New("worldruntime: character ownership fence stale")
	ErrCharacterOwnershipEpochExhausted = errors.New("worldruntime: character ownership epoch exhausted")
)

// SessionOwnershipFence identifies the currently authoritative trusted network session for
// one CharacterID. It is process-local, server-internal, and never serialized into Protocol v6.
// Epoch is independent from the shorter-lived S3-F.17 admission lease generation so future
// ownership transfer can advance active ownership without changing admission semantics.
type SessionOwnershipFence struct {
	SessionID   session.ID
	EntityID    world.EntityID
	CharacterID characteridentity.ID
	Epoch       uint64
}

func (f SessionOwnershipFence) Valid() bool {
	return f.SessionID != 0 && f.EntityID != 0 && f.CharacterID != "" && f.Epoch != 0
}

// prepareOwnership allocates a monotonic process-local epoch before join mutates world state.
// Failed joins may leave gaps; epochs are fencing identities, not a dense counter.
func (r *characterIdentityRegistry) prepareOwnership(s *session.Session) (SessionOwnershipFence, error) {
	if s == nil || !s.CharacterIdentity.Valid() {
		return SessionOwnershipFence{}, ErrCharacterIdentityMissing
	}
	if s.CharacterIdentity.Assurance != characteridentity.AssuranceTrusted {
		return SessionOwnershipFence{}, nil
	}
	if r.nextOwnershipEpoch == math.MaxUint64 {
		return SessionOwnershipFence{}, ErrCharacterOwnershipEpochExhausted
	}
	r.nextOwnershipEpoch++
	return SessionOwnershipFence{
		SessionID:   s.ID,
		EntityID:    s.EntityID,
		CharacterID: s.CharacterIdentity.ID,
		Epoch:       r.nextOwnershipEpoch,
	}, nil
}

func (r *characterIdentityRegistry) activateOwnership(fence SessionOwnershipFence) {
	if !fence.Valid() {
		return
	}
	r.ownershipByCharacter[fence.CharacterID] = fence
	r.ownershipBySession[fence.SessionID] = fence
}

// validateOwnership is world-owner-only. Network goroutines only carry immutable fences into
// the bounded command queue; they never read active ownership maps concurrently.
func (r *characterIdentityRegistry) validateOwnership(sessionID session.ID, fence SessionOwnershipFence) error {
	if !fence.Valid() || fence.SessionID != sessionID {
		return ErrCharacterOwnershipFenceInvalid
	}
	byCharacter, ok := r.ownershipByCharacter[fence.CharacterID]
	if !ok || byCharacter != fence {
		return fmt.Errorf("%w: character=%s session=%d epoch=%d", ErrCharacterOwnershipFenceStale, fence.CharacterID, fence.SessionID, fence.Epoch)
	}
	bySession, ok := r.ownershipBySession[fence.SessionID]
	if !ok || bySession != fence {
		return fmt.Errorf("%w: character=%s session=%d epoch=%d", ErrCharacterOwnershipFenceStale, fence.CharacterID, fence.SessionID, fence.Epoch)
	}
	return nil
}

// removeOwnershipBySession is generation-fenced defensively. A future transfer may install a
// newer owner before an older teardown reaches cleanup; only the exact current mapping is removed.
func (r *characterIdentityRegistry) removeOwnershipBySession(sessionID session.ID) {
	fence, ok := r.ownershipBySession[sessionID]
	if !ok {
		return
	}
	delete(r.ownershipBySession, sessionID)
	if current, ok := r.ownershipByCharacter[fence.CharacterID]; ok && current == fence {
		delete(r.ownershipByCharacter, fence.CharacterID)
	}
}

func (r *Runtime) EnqueueFencedLeave(fence SessionOwnershipFence) error {
	if !fence.Valid() {
		return ErrCharacterOwnershipFenceInvalid
	}
	return r.queue.tryPush(leaveCommand{id: fence.SessionID, ownership: fence})
}

func (r *Runtime) EnqueueFencedMove(fence SessionOwnershipFence, sequence uint32, input protocol.ClientMoveInput) error {
	if !fence.Valid() || sequence == 0 {
		return ErrCharacterOwnershipFenceInvalid
	}
	return r.queue.tryPush(moveInputCommand{sessionID: fence.SessionID, sequence: sequence, input: input, ownership: fence})
}

func (r *Runtime) EnqueueFencedUseAction(fence SessionOwnershipFence, sequence uint32, action protocol.ClientUseAction) error {
	if !fence.Valid() {
		return ErrCharacterOwnershipFenceInvalid
	}
	if sequence == 0 || action.ActionID == "" || action.TargetKind == "" || action.TargetID == "" {
		return errors.New("worldruntime: invalid action intent")
	}
	return r.queue.tryPush(useActionCommand{sessionID: fence.SessionID, sequence: sequence, action: action, ownership: fence})
}
