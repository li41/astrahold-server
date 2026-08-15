package worldruntime

import (
	"context"
	"errors"

	"github.com/li41/astrahold-server/internal/characteridentity"
)

var ErrCharacterAdmissionRequiresTrustedIdentity = errors.New("worldruntime: character admission requires trusted identity")

// AwaitCharacterAdmission inserts a read-only barrier into the world-owner command FIFO.
// If an older leave command was already enqueued, that leave is processed first and its
// character-state save intent exists before this method succeeds. The barrier never
// performs persistence I/O and never evicts an active character.
func (r *Runtime) AwaitCharacterAdmission(ctx context.Context, identity characteridentity.Binding) error {
	if ctx == nil {
		ctx = context.Background()
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	completion := make(chan error, 1)
	if err := r.queue.tryPush(characterAdmissionCommand{identity: identity, completion: completion}); err != nil {
		return err
	}
	select {
	case err := <-completion:
		return err
	case <-ctx.Done():
		// Admission is read-only, so abandoning the wait cannot leave partial world state.
		return ctx.Err()
	}
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

func completeWorldOwnerCommand(completion chan error, err error) {
	if completion == nil {
		return
	}
	// Completion channels are one-shot and buffered so the world owner never blocks on
	// a network goroutine that stopped waiting (admission may be cancelled).
	select {
	case completion <- err:
	default:
	}
}
