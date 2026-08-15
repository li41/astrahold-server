package worldruntime

import (
	"context"
	"errors"

	"github.com/li41/astrahold-server/internal/characteridentity"
)

var ErrCharacterAdmissionRequiresTrustedIdentity = errors.New("worldruntime: character admission requires trusted identity")

// AwaitCharacterAdmission inserts a mutating admission-reservation barrier into the
// world-owner command FIFO. If an older leave command was already enqueued, that leave is
// processed first and its character-state save intent exists before the lease is issued.
//
// Once queued, the caller must observe completion even if ctx later cancels because the
// command may have created a process-local reservation that must be explicitly released or
// consumed by a matching join. No persistence or network I/O runs in the world owner.
func (r *Runtime) AwaitCharacterAdmission(ctx context.Context, identity characteridentity.Binding) (CharacterAdmissionLease, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	select {
	case <-ctx.Done():
		return CharacterAdmissionLease{}, ctx.Err()
	default:
	}
	lease := CharacterAdmissionLease{}
	completion := make(chan error, 1)
	if err := r.queue.tryPush(characterAdmissionCommand{
		identity: characterAdmissionOperation{identity: identity, lease: &lease},
		completion: completion,
	}); err != nil {
		return CharacterAdmissionLease{}, err
	}
	if err := <-completion; err != nil {
		return CharacterAdmissionLease{}, err
	}
	return lease, nil
}

// ReleaseCharacterAdmission releases only the exact lease generation supplied by the caller.
// A stale release is an idempotent no-op and cannot clear a newer admission reservation.
func (r *Runtime) ReleaseCharacterAdmission(ctx context.Context, lease CharacterAdmissionLease) error {
	if ctx == nil {
		ctx = context.Background()
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	completion := make(chan error, 1)
	if err := r.queue.tryPush(characterAdmissionCommand{
		identity: characterAdmissionOperation{lease: &lease, release: true},
		completion: completion,
	}); err != nil {
		return err
	}
	return <-completion
}

// AwaitJoin waits for the world owner to commit or reject the existing join transaction.
// Once the mutating command is queued, the caller must observe its completion rather than
// returning early on context cancellation; otherwise a late successful join could leave
// an entity without a transport owner.
func (r *Runtime) AwaitJoin(ctx context.Context, request JoinRequest) error {
	if ctx == nil {
		ctx = context.Background()
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	completion := make(chan error, 1)
	if err := r.queue.tryPush(joinCommand{request: request, completion: completion}); err != nil {
		return err
	}
	return <-completion
}

// AwaitJoinOwned is the trusted transport seam added by S3-F.18. It uses the same join
// transaction and completion ordering as AwaitJoin but also returns the active ownership
// fence minted by the world owner. Ephemeral joins return a zero fence.
func (r *Runtime) AwaitJoinOwned(ctx context.Context, request JoinRequest) (SessionOwnershipFence, error) {
	ownership := SessionOwnershipFence{}
	request.OwnershipFence = &ownership
	if err := r.AwaitJoin(ctx, request); err != nil {
		return SessionOwnershipFence{}, err
	}
	return ownership, nil
}

func completeWorldOwnerCommand(completion chan error, err error) {
	if completion == nil {
		return
	}
	// Completion channels are one-shot and buffered so the world owner never blocks on
	// a network goroutine that stopped waiting before a mutating command was queued.
	select {
	case completion <- err:
	default:
	}
}
