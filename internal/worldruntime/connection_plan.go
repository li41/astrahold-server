package worldruntime

import (
	"context"
	"errors"

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

// prepareConnectionPlan keeps the inactive admission reservation and active ownership lookup
// in one world-owner FIFO position. Active is a normal connection plan, not an admission
// error. Inactive still uses the exact S3-F.17 lease reservation semantics.
func (r *characterIdentityRegistry) prepareConnectionPlan(identity characteridentity.Binding) (CharacterConnectionPlan, error) {
	ownership, err := r.currentOwnership(identity)
	if err == nil {
		return CharacterConnectionPlan{Ownership: ownership}, nil
	}
	if !errors.Is(err, ErrCharacterOwnershipNotActive) {
		return CharacterConnectionPlan{}, err
	}

	lease := CharacterAdmissionLease{}
	if err := r.validateAdmission(characterAdmissionOperation{identity: identity, lease: &lease}); err != nil {
		return CharacterConnectionPlan{}, err
	}
	return CharacterConnectionPlan{AdmissionLease: lease}, nil
}

// AwaitCharacterConnectionPlan is the transport-facing trusted connection barrier for
// S3-F.20. Once queued, the caller observes completion because the command may reserve an
// admission lease. The active branch only returns current process-local ownership truth.
func (r *Runtime) AwaitCharacterConnectionPlan(ctx context.Context, identity characteridentity.Binding) (CharacterConnectionPlan, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	select {
	case <-ctx.Done():
		return CharacterConnectionPlan{}, ctx.Err()
	default:
	}

	result := CharacterConnectionPlan{}
	completion := make(chan error, 1)
	if err := r.queue.tryPush(characterConnectionPlanCommand{identity: identity, result: &result, completion: completion}); err != nil {
		return CharacterConnectionPlan{}, err
	}
	if err := <-completion; err != nil {
		return CharacterConnectionPlan{}, err
	}
	if !result.Valid() {
		return CharacterConnectionPlan{}, ErrCharacterOwnershipFenceInvalid
	}
	return result, nil
}
