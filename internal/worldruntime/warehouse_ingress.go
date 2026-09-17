package worldruntime

import (
	"errors"

	"github.com/li41/astrahold-server/internal/protocol"
	"github.com/li41/astrahold-server/internal/session"
)

var ErrInvalidWarehouseIntent = errors.New("worldruntime: invalid warehouse intent")

// EnqueueWarehouseCommand carries an unfenced reliable warehouse intent into the bounded world
// owner queue. Authentication subject, map, range, inventory and warehouse truth are resolved only
// when the world owner applies the command.
func (r *Runtime) EnqueueWarehouseCommand(id session.ID, sequence uint32, intent protocol.ClientWarehouseCommand) error {
	if id == 0 || sequence == 0 {
		return ErrInvalidWarehouseIntent
	}
	payload := intent
	return r.queue.tryPush(useActionCommand{sessionID: id, sequence: sequence, warehouse: &payload})
}

// EnqueueFencedWarehouseCommand is the trusted-session ingress path. The immutable ownership fence
// is revalidated by the world owner before any warehouse legality check or mutation occurs.
func (r *Runtime) EnqueueFencedWarehouseCommand(ownership SessionOwnershipFence, sequence uint32, intent protocol.ClientWarehouseCommand) error {
	if !ownership.Valid() || sequence == 0 {
		return ErrInvalidWarehouseIntent
	}
	payload := intent
	return r.queue.tryPush(useActionCommand{
		sessionID: ownership.SessionID,
		sequence:  sequence,
		warehouse: &payload,
		ownership: ownership,
	})
}
