package worldruntime

import (
	"errors"
	"strings"

	"github.com/li41/astrahold-server/internal/character"
	"github.com/li41/astrahold-server/internal/equipmentcatalog"
	"github.com/li41/astrahold-server/internal/inventory"
	"github.com/li41/astrahold-server/internal/iteminstance"
	"github.com/li41/astrahold-server/internal/protocol"
	"github.com/li41/astrahold-server/internal/session"
)

func validateEquipmentInstanceIntent(command protocol.ClientEquipmentInstanceCommand) error {
	if _, ok := inventoryEquipmentSlot(command.Slot); !ok {
		return errors.New("worldruntime: invalid equipment slot")
	}
	instanceID := strings.TrimSpace(command.ItemInstanceID)
	switch command.Operation {
	case protocol.EquipmentOperationEquip:
		if instanceID == "" || instanceID != command.ItemInstanceID {
			return errors.New("worldruntime: invalid equipment instance")
		}
	case protocol.EquipmentOperationUnequip:
		if command.ItemInstanceID != "" {
			return errors.New("worldruntime: instance unequip must not specify an item")
		}
	default:
		return errors.New("worldruntime: invalid equipment operation")
	}
	return nil
}

func (r *Runtime) EnqueueEquipmentInstanceCommand(id session.ID, sequence uint32, equipment protocol.ClientEquipmentInstanceCommand) error {
	if id == 0 || sequence == 0 {
		return errors.New("worldruntime: invalid equipment instance intent")
	}
	if err := validateEquipmentInstanceIntent(equipment); err != nil {
		return err
	}
	payload := equipment
	return r.queue.tryPush(equipmentCommand{sessionID: id, sequence: sequence, equipmentInstance: &payload})
}

func (r *Runtime) applyEquipmentInstanceCommand(name string, command equipmentCommand, report *StepReport) {
	if command.equipmentInstance == nil {
		report.CommandErrors = append(report.CommandErrors, CommandError{Command: name, SessionID: command.sessionID, Err: errors.New("worldruntime: equipment instance payload missing")})
		return
	}
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
	s.MarkProcessedAction(command.sequence)
	state, ok := r.characters.State(s.EntityID)
	if !ok {
		report.CommandErrors = append(report.CommandErrors, CommandError{Command: name, SessionID: command.sessionID, Err: character.ErrCharacterNotFound})
		return
	}
	if state.Defeated {
		report.CommandErrors = append(report.CommandErrors, CommandError{Command: name, SessionID: command.sessionID, Err: character.ErrCharacterDefeated})
		return
	}
	inv := r.inventories[s.CharacterIdentity.ID]
	if inv == nil {
		report.CommandErrors = append(report.CommandErrors, CommandError{Command: name, SessionID: command.sessionID, Err: errors.New("worldruntime: inventory unavailable")})
		return
	}
	request := *command.equipmentInstance
	slot, ok := inventoryEquipmentSlot(request.Slot)
	if !ok {
		report.CommandErrors = append(report.CommandErrors, CommandError{Command: name, SessionID: command.sessionID, Err: errors.New("worldruntime: invalid equipment slot")})
		return
	}
	var err error
	switch request.Operation {
	case protocol.EquipmentOperationEquip:
		err = r.applyEquipInstance(inv, request.Slot, iteminstance.ID(request.ItemInstanceID))
	case protocol.EquipmentOperationUnequip:
		_, err = inv.UnequipInstance(slot)
	default:
		err = errors.New("worldruntime: invalid equipment operation")
	}
	if err != nil {
		report.CommandErrors = append(report.CommandErrors, CommandError{Command: name, SessionID: command.sessionID, Err: err})
		return
	}
	r.sessionInventoryPending[command.sessionID] = struct{}{}
}

// applyEquipInstance keeps Protocol slot semantics at the worldruntime boundary, while inventory
// owns the authoritative generic slot representation internally.
func (r *Runtime) applyEquipInstance(inv *inventory.Inventory, protocolSlot protocol.EquipmentSlot, instanceID iteminstance.ID) error {
	slot, ok := inventoryEquipmentSlot(protocolSlot)
	if !ok {
		return ErrEquipmentItemNotAllowed
	}
	instance, ok := inv.Instance(instanceID)
	if !ok {
		return inventory.ErrInstanceNotFound
	}
	definition, ok := defaultEquipmentCatalog.Resolve(instance.ItemArchetypeID)
	if !ok || definition.Tier == equipmentcatalog.TierLow {
		return ErrEquipmentItemNotAllowed
	}
	if err := iteminstance.Validate(instance, definition); err != nil {
		return ErrEquipmentItemNotAllowed
	}
	kind, ok := expectedEquipmentKind(slot)
	if !ok {
		return ErrEquipmentItemNotAllowed
	}
	catalogSlot, ok := catalogEquipmentSlot(slot)
	if !ok || !equipmentDefinitionAllowed(definition, kind, catalogSlot) {
		return ErrEquipmentItemNotAllowed
	}
	if slot == inventory.SlotMainHand {
		if err := validateMainHandEquipmentCompatibility(inv, instance.ItemArchetypeID); err != nil {
			return err
		}
	}
	if slot == inventory.SlotOffHand {
		if err := validateOffHandEquipmentCompatibility(inv); err != nil {
			return err
		}
	}
	return inv.EquipInstance(slot, instanceID)
}
