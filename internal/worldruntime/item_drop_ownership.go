package worldruntime

import (
	"github.com/li41/astrahold-server/internal/protocol"
)

func (r *Runtime) EnqueueFencedPickupItem(fence SessionOwnershipFence, sequence uint32, intent protocol.ClientPickupItem) error {
	if !fence.Valid() || sequence == 0 {
		return ErrCharacterOwnershipFenceInvalid
	}
	if err := validatePickupIntent(intent); err != nil {
		return err
	}
	payload := intent
	return r.queue.tryPush(useActionCommand{sessionID: fence.SessionID, sequence: sequence, pickup: &payload, ownership: fence})
}
