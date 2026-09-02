package worldruntime

import (
	"errors"
	"strings"

	"github.com/li41/astrahold-server/internal/protocol"
	"github.com/li41/astrahold-server/internal/session"
)

const trainingBladeArchetypeID = "item_training_blade"

var ErrEquipmentItemNotAllowed = errors.New("worldruntime: equipment item not allowed")

func validateEquipmentIntent(command protocol.ClientEquipmentCommand) error {
	if command.Slot != protocol.EquipmentSlotMainHand {
		return errors.New("worldruntime: invalid equipment slot")
	}
	switch command.Operation {
	case protocol.EquipmentOperationEquip:
		if strings.TrimSpace(command.ItemArchetypeID) == "" {
			return errors.New("worldruntime: invalid equipment item")
		}
	case protocol.EquipmentOperationUnequip:
		if command.ItemArchetypeID != "" {
			return errors.New("worldruntime: unequip must not specify an item")
		}
	default:
		return errors.New("worldruntime: invalid equipment operation")
	}
	return nil
}

func (r *Runtime) EnqueueEquipmentCommand(id session.ID, sequence uint32, equipment protocol.ClientEquipmentCommand) error {
	if id == 0 || sequence == 0 {
		return errors.New("worldruntime: invalid equipment intent")
	}
	if err := validateEquipmentIntent(equipment); err != nil {
		return err
	}
	return r.queue.tryPush(equipmentCommand{sessionID: id, sequence: sequence, equipment: equipment})
}

func (r *Runtime) applyEquipmentCommand(name string, command equipmentCommand, report *StepReport) {
	if command.ownership.Valid() {
		if err := r.characterIdentities.validateOwnership(command.sessionID, command.ownership); err != nil {
			report.CommandErrors = append(report.CommandErrors, CommandError{Command: name, SessionID: command.sessionID, Err: err})
			return
		}
	}
	s, ok := r.sessions.Get(command.sessionID)
	if !ok {
		report.CommandErrors = append(report.CommandErrors, CommandError{Command: name, SessionID: command.sessionID, Err: session.ErrSessionNotFound})
		return
	}
	if err := s.ValidateActionSequence(command.sequence); err != nil {
		report.CommandErrors = append(report.CommandErrors, CommandError{Command: name, SessionID: command.sessionID, Err: err})
		return
	}
	// Reliable equipment intent is consumed once the authoritative world owner processes it,
	// even when gameplay validation rejects the requested transaction.
	s.MarkProcessedAction(command.sequence)

	inv := r.inventories[s.CharacterIdentity.ID]
	if inv == nil {
		report.CommandErrors = append(report.CommandErrors, CommandError{Command: name, SessionID: command.sessionID, Err: errors.New("worldruntime: inventory unavailable")})
		return
	}

	var err error
	switch command.equipment.Operation {
	case protocol.EquipmentOperationEquip:
		if command.equipment.ItemArchetypeID != trainingBladeArchetypeID {
			err = ErrEquipmentItemNotAllowed
		} else {
			err = inv.EquipMainHand(command.equipment.ItemArchetypeID)
		}
	case protocol.EquipmentOperationUnequip:
		_, err = inv.UnequipMainHand()
	default:
		err = errors.New("worldruntime: invalid equipment operation")
	}
	if err != nil {
		report.CommandErrors = append(report.CommandErrors, CommandError{Command: name, SessionID: command.sessionID, Err: err})
		return
	}

	// One pending marker drives the paired authoritative Inventory + Equipment snapshots.
	r.sessionInventoryPending[command.sessionID] = struct{}{}
}
