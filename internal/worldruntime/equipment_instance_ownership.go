package worldruntime

import (
	"errors"

	"github.com/li41/astrahold-server/internal/protocol"
)

// EnqueueFencedEquipmentInstanceCommand is the trusted-session Protocol v28 exact-instance path.
// It mirrors every other mutable character intent: immutable ownership is captured by the adapter,
// then validated again by the world owner before any inventory/equipment truth can change.
func (r *Runtime) EnqueueFencedEquipmentInstanceCommand(fence SessionOwnershipFence, sequence uint32, equipment protocol.ClientEquipmentInstanceCommand) error {
	if !fence.Valid() {
		return ErrCharacterOwnershipFenceInvalid
	}
	if sequence == 0 {
		return errors.New("worldruntime: invalid equipment instance intent")
	}
	if err := validateEquipmentInstanceIntent(equipment); err != nil {
		return err
	}
	payload := equipment
	return r.queue.tryPush(equipmentCommand{sessionID: fence.SessionID, sequence: sequence, equipmentInstance: &payload, ownership: fence})
}
