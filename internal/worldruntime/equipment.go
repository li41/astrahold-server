package worldruntime

import (
	"errors"
	"strings"

	"github.com/li41/astrahold-server/internal/character"
	"github.com/li41/astrahold-server/internal/classid"
	"github.com/li41/astrahold-server/internal/equipmentcatalog"
	"github.com/li41/astrahold-server/internal/protocol"
	"github.com/li41/astrahold-server/internal/session"
)

const trainingBladeArchetypeID = "item_training_blade"

var (
	ErrEquipmentItemNotAllowed = errors.New("worldruntime: equipment item not allowed")
	defaultEquipmentCatalog    = mustDefaultEquipmentCatalog()
)

func mustDefaultEquipmentCatalog() *equipmentcatalog.Catalog {
	catalog, err := equipmentcatalog.Default()
	if err != nil {
		panic(err)
	}
	return catalog
}

// equipmentDefinitionAllowed owns active equipment legality. Fixed-class metadata is deliberately
// excluded: classless characters are constrained by the actual item kind and equipment slot only.
func equipmentDefinitionAllowed(definition equipmentcatalog.Definition, kind equipmentcatalog.Kind, slot equipmentcatalog.Slot) bool {
	return definition.Kind == kind && definition.Slot == slot
}

func mainHandItemAllowed(itemArchetypeID string) bool {
	itemArchetypeID = strings.TrimSpace(itemArchetypeID)
	if itemArchetypeID == trainingBladeArchetypeID {
		return true
	}
	definition, ok := defaultEquipmentCatalog.Resolve(itemArchetypeID)
	if !ok {
		return false
	}
	return equipmentDefinitionAllowed(definition, equipmentcatalog.KindWeapon, equipmentcatalog.SlotMainHand)
}

// mainHandItemAllowedForClass is a temporary v27/internal-call compatibility seam. ClassID no
// longer participates in gameplay legality and this wrapper can disappear with the legacy callers.
func mainHandItemAllowedForClass(itemArchetypeID string, _ classid.ID) bool {
	return mainHandItemAllowed(itemArchetypeID)
}

func offHandItemAllowed(itemArchetypeID string) bool {
	definition, ok := defaultEquipmentCatalog.Resolve(strings.TrimSpace(itemArchetypeID))
	if !ok {
		return false
	}
	return equipmentDefinitionAllowed(definition, equipmentcatalog.KindShield, equipmentcatalog.SlotOffHand)
}

// offHandItemAllowedForClass is a temporary v27/internal-call compatibility seam. ClassID no
// longer participates in gameplay legality and this wrapper can disappear with the legacy callers.
func offHandItemAllowedForClass(itemArchetypeID string, _ classid.ID) bool {
	return offHandItemAllowed(itemArchetypeID)
}

func (r *Runtime) EnqueueEquipmentCommand(id session.ID, sequence uint32, equipment protocol.ClientEquipmentCommand) error {
	if id == 0 || sequence == 0 {
		return errors.New("worldruntime: invalid equipment intent")
	}
	if err := validateEquipmentIntent(equipment); err != nil {
		return err
	}
	payload := equipment
	return r.queue.tryPush(equipmentCommand{sessionID: id, sequence: sequence, equipment: &payload})
}

func validateEquipmentIntent(command protocol.ClientEquipmentCommand) error {
	switch command.Slot {
	case protocol.EquipmentSlotMainHand, protocol.EquipmentSlotOffHand:
	default:
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

func (r *Runtime) applyEquipmentCommand(name string, command equipmentCommand, report *StepReport) {
	if command.equipment == nil {
		report.CommandErrors = append(report.CommandErrors, CommandError{Command: name, SessionID: command.sessionID, Err: errors.New("worldruntime: equipment payload missing")})
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

	request := *command.equipment
	var err error
	switch request.Slot {
	case protocol.EquipmentSlotMainHand:
		switch request.Operation {
		case protocol.EquipmentOperationEquip:
			if !mainHandItemAllowed(request.ItemArchetypeID) {
				err = ErrEquipmentItemNotAllowed
			} else {
				err = inv.EquipMainHand(request.ItemArchetypeID)
			}
		case protocol.EquipmentOperationUnequip:
			_, err = inv.UnequipMainHand()
		}
	case protocol.EquipmentSlotOffHand:
		switch request.Operation {
		case protocol.EquipmentOperationEquip:
			if !offHandItemAllowed(request.ItemArchetypeID) {
				err = ErrEquipmentItemNotAllowed
			} else {
				err = inv.EquipOffHand(request.ItemArchetypeID)
			}
		case protocol.EquipmentOperationUnequip:
			_, err = inv.UnequipOffHand()
		}
	default:
		err = errors.New("worldruntime: invalid equipment slot")
	}
	if err != nil {
		report.CommandErrors = append(report.CommandErrors, CommandError{Command: name, SessionID: command.sessionID, Err: err})
		return
	}
	r.sessionInventoryPending[command.sessionID] = struct{}{}
}
