package worldruntime

import (
	"context"

	"github.com/li41/astrahold-server/internal/characteridentity"
)

// CharacterConnectionPlan is the world-owner decision for one authenticated trusted
// CharacterID connection attempt. Exactly one branch is valid: AdmissionLease for an
// inactive character, or Ownership for an already-active character that may be taken over.
type CharacterConnectionPlan struct {
	AdmissionLease CharacterAdmissionLease
	Ownership      SessionOwnershipFence
}

func (p CharacterConnectionPlan) Valid() bool {
	return p.AdmissionLease.Valid() != p.Ownership.Valid()
}

func (p CharacterConnectionPlan) Takeover() bool { return p.Ownership.Valid() }

// AwaitCharacterConnectionPlan reuses the S3-F.17 admission command as the transport-facing
// S3-F.20 trusted connection barrier. In one world-owner FIFO position it either reserves an
// inactive CharacterID or returns the exact active S3-F.19 ownership fence. Once queued, the
// caller observes completion because the inactive branch may have created a reservation.
func (r *Runtime) AwaitCharacterConnectionPlan(ctx context.Context, identity characteridentity.Binding) (CharacterConnectionPlan, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	select {
	case <-ctx.Done():
		return CharacterConnectionPlan{}, ctx.Err()
	default:
	}

	lease := CharacterAdmissionLease{}
	ownership := SessionOwnershipFence{}
	completion := make(chan error, 1)
	operation := characterAdmissionOperation{identity: identity, lease: &lease, ownership: &ownership}
	if err := r.queue.tryPush(characterAdmissionCommand{identity: operation, completion: completion}); err != nil {
		return CharacterConnectionPlan{}, err
	}
	if err := <-completion; err != nil {
		return CharacterConnectionPlan{}, err
	}
	result := CharacterConnectionPlan{AdmissionLease: lease, Ownership: ownership}
	if !result.Valid() {
		return CharacterConnectionPlan{}, ErrCharacterOwnershipFenceInvalid
	}
	return result, nil
}
