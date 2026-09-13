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
	switch command.Slot {
	case protocol.EquipmentSlotMainHand, protocol.EquipmentSlotOffHand:
	default:
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

// EnqueueEquipmentInstanceCommand is a Server-internal staged entrypoint for the next protocol
// contract. Protocol v27 adapters must not call it; formal network acceptance starts only after the
// coordinated version cutover. The mutation itself already uses the normal bounded world-owner path.
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
	var err error
	switch request.Operation {
	case protocol.EquipmentOperationEquip:
		err = r.applyEquipInstance(inv, request.Slot, iteminstance.ID(request.ItemInstanceID))
	case protocol.EquipmentOperationUnequip:
		err = applyUnequipInstanceSlot(inv, request.Slot)
	default:
		err = errors.New("worldruntime: invalid equipment operation")
	}
	if err != nil {
		report.CommandErrors = append(report.CommandErrors, CommandError{Command: name, SessionID: command.sessionID, Err: err})
		return
	}
	r.sessionInventoryPending[command.sessionID] = struct{}{}
}

func (r *Runtime) applyEquipInstance(inv *inventory.Inventory, slot protocol.EquipmentSlot, instanceID iteminstance.ID) error {
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

	switch slot {
	case protocol.EquipmentSlotMainHand:
		if !equipmentDefinitionAllowed(definition, equipmentcatalog.KindWeapon, equipmentcatalog.SlotMainHand) {
			return ErrEquipmentItemNotAllowed
		}
		return inv.EquipMainHandInstance(instanceID)
	case protocol.EquipmentSlotOffHand:
		if !equipmentDefinitionAllowed(definition, equipmentcatalog.KindShield, equipmentcatalog.SlotOffHand) {
			return ErrEquipmentItemNotAllowed
		}
		return inv.EquipOffHandInstance(instanceID)
	default:
		return errors.New("worldruntime: invalid equipment slot")
	}
}

func applyUnequipInstanceSlot(inv *inventory.Inventory, slot protocol.EquipmentSlot) error {
	switch slot {
	case protocol.EquipmentSlotMainHand:
		if _, ok := inv.MainHandInstance(); !ok {
			return inventory.ErrEquipmentSlotEmpty
		}
		_, err := inv.UnequipMainHandInstance()
		return err
	case protocol.EquipmentSlotOffHand:
		if _, ok := inv.OffHandInstance(); !ok {
			return inventory.ErrEquipmentSlotEmpty
		}
		_, err := inv.UnequipOffHandInstance()
		return err
	default:
		return errors.New("worldruntime: invalid equipment slot")
	}
}
