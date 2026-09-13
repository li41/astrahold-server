package worldruntime

import (
	"errors"

	"github.com/li41/astrahold-server/internal/protocol"
)

// EnqueueFencedEquipmentInstanceCommand mirrors the trusted ownership fence used by all mutable
// character intents. It is staged for the next protocol version and must not be called by v27
// adapters before coordinated cutover.
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
