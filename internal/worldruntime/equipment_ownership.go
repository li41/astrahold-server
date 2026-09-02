package worldruntime

import (
	"errors"

	"github.com/li41/astrahold-server/internal/protocol"
)

func (r *Runtime) EnqueueFencedEquipmentCommand(fence SessionOwnershipFence, sequence uint32, equipment protocol.ClientEquipmentCommand) error {
	if !fence.Valid() {
		return ErrCharacterOwnershipFenceInvalid
	}
	if sequence == 0 {
		return errors.New("worldruntime: invalid equipment intent")
	}
	if err := validateEquipmentIntent(equipment); err != nil {
		return err
	}
	payload := equipment
	return r.queue.tryPush(equipmentCommand{sessionID: fence.SessionID, sequence: sequence, equipment: &payload, ownership: fence})
}
