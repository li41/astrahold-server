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

const (
	WeaponEnhancementScrollItemArchetypeID = "item_astrahold_weapon_enhancement_scroll"
	ArmorEnhancementScrollItemArchetypeID  = "item_astrahold_armor_enhancement_scroll"
	MaxEquipmentEnhancementLevel    uint16  = 10
)

var ErrEquipmentEnhancementRejected = errors.New("worldruntime: equipment enhancement rejected")

func validateEnhanceEquipmentIntent(intent protocol.ClientEnhanceEquipment) error {
	if strings.TrimSpace(intent.ScrollItemArchetypeID) != intent.ScrollItemArchetypeID || strings.TrimSpace(intent.ItemInstanceID) != intent.ItemInstanceID {
		return ErrEquipmentEnhancementRejected
	}
	if intent.ItemInstanceID == "" {
		return ErrEquipmentEnhancementRejected
	}
	switch intent.ScrollItemArchetypeID {
	case WeaponEnhancementScrollItemArchetypeID, ArmorEnhancementScrollItemArchetypeID:
		return nil
	default:
		return ErrEquipmentEnhancementRejected
	}
}

func (r *Runtime) EnqueueEnhanceEquipment(id session.ID, sequence uint32, intent protocol.ClientEnhanceEquipment) error {
	if id == 0 || sequence == 0 {
		return ErrEquipmentEnhancementRejected
	}
	if err := validateEnhanceEquipmentIntent(intent); err != nil {
		return err
	}
	payload := intent
	return r.queue.tryPush(useActionCommand{sessionID: id, sequence: sequence, enhancement: &payload})
}

func (r *Runtime) EnqueueFencedEnhanceEquipment(fence SessionOwnershipFence, sequence uint32, intent protocol.ClientEnhanceEquipment) error {
	if !fence.Valid() {
		return ErrCharacterOwnershipFenceInvalid
	}
	if sequence == 0 {
		return ErrEquipmentEnhancementRejected
	}
	if err := validateEnhanceEquipmentIntent(intent); err != nil {
		return err
	}
	payload := intent
	return r.queue.tryPush(useActionCommand{sessionID: fence.SessionID, sequence: sequence, enhancement: &payload, ownership: fence})
}

func enhancementScrollAllows(scrollID string, kind equipmentcatalog.Kind) bool {
	switch scrollID {
	case WeaponEnhancementScrollItemArchetypeID:
		return kind == equipmentcatalog.KindWeapon
	case ArmorEnhancementScrollItemArchetypeID:
		return kind == equipmentcatalog.KindArmor || kind == equipmentcatalog.KindShield
	default:
		return false
	}
}

func (r *Runtime) queueEquipmentEnhancementResult(sessionID session.ID, result protocol.EquipmentEnhancementResult) {
	r.pendingResourceMessages[sessionID] = append(r.pendingResourceMessages[sessionID], result)
}

func (r *Runtime) rejectEquipmentEnhancement(command useActionCommand, reason protocol.EquipmentEnhancementRejectionReason, previous uint16) {
	intent := *command.enhancement
	r.queueEquipmentEnhancementResult(command.sessionID, protocol.EquipmentEnhancementResult{
		ClientActionSequence: command.sequence,
		ScrollItemArchetypeID: intent.ScrollItemArchetypeID,
		ItemInstanceID: intent.ItemInstanceID,
		Outcome: protocol.EquipmentEnhancementOutcomeRejected,
		Reason: reason,
		PreviousLevel: previous,
		CurrentLevel: previous,
		ScrollConsumed: false,
	})
}

func (r *Runtime) applyEnhanceEquipment(name string, command useActionCommand, report *StepReport) {
	if command.enhancement == nil {
		report.CommandErrors = append(report.CommandErrors, CommandError{Command: name, SessionID: command.sessionID, Err: ErrEquipmentEnhancementRejected})
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
	// A reliable enhancement intent is consumed exactly once even when gameplay rejects it.
	s.MarkProcessedAction(command.sequence)
	state, ok := r.characters.State(s.EntityID)
	if !ok {
		report.CommandErrors = append(report.CommandErrors, CommandError{Command: name, SessionID: command.sessionID, Err: character.ErrCharacterNotFound})
		return
	}
	if state.Defeated {
		r.rejectEquipmentEnhancement(command, protocol.EquipmentEnhancementRejectionDefeated, 0)
		return
	}
	inv := r.inventories[s.CharacterIdentity.ID]
	if inv == nil {
		r.rejectEquipmentEnhancement(command, protocol.EquipmentEnhancementRejectionServerRejected, 0)
		return
	}
	intent := *command.enhancement
	if inv.Quantity(intent.ScrollItemArchetypeID) == 0 {
		r.rejectEquipmentEnhancement(command, protocol.EquipmentEnhancementRejectionMissingScroll, 0)
		return
	}
	instance, _, ok := inv.OwnedInstance(iteminstance.ID(intent.ItemInstanceID))
	if !ok {
		r.rejectEquipmentEnhancement(command, protocol.EquipmentEnhancementRejectionMissingTarget, 0)
		return
	}
	definition, ok := defaultEquipmentCatalog.Resolve(instance.ItemArchetypeID)
	if !ok || !enhancementScrollAllows(intent.ScrollItemArchetypeID, definition.Kind) {
		r.rejectEquipmentEnhancement(command, protocol.EquipmentEnhancementRejectionWrongScroll, instance.EnhancementLevel)
		return
	}
	if err := iteminstance.Validate(instance, definition); err != nil {
		r.rejectEquipmentEnhancement(command, protocol.EquipmentEnhancementRejectionServerRejected, instance.EnhancementLevel)
		return
	}
	if instance.EnhancementLevel >= MaxEquipmentEnhancementLevel {
		r.rejectEquipmentEnhancement(command, protocol.EquipmentEnhancementRejectionAtLimit, instance.EnhancementLevel)
		return
	}

	previous := instance.EnhancementLevel
	updated := instance
	updated.EnhancementLevel++
	if err := inv.Remove(intent.ScrollItemArchetypeID, 1); err != nil {
		r.rejectEquipmentEnhancement(command, protocol.EquipmentEnhancementRejectionMissingScroll, previous)
		return
	}
	if _, err := inv.ReplaceOwnedInstance(updated); err != nil {
		// Single-owner execution makes this path exceptional. Roll back the consumed stack rather
		// than creating a loss window if an invariant is ever violated.
		_ = inv.Add(intent.ScrollItemArchetypeID, 1)
		r.rejectEquipmentEnhancement(command, protocol.EquipmentEnhancementRejectionServerRejected, previous)
		return
	}
	r.sessionInventoryPending[command.sessionID] = struct{}{}
	r.queueEquipmentEnhancementResult(command.sessionID, protocol.EquipmentEnhancementResult{
		ClientActionSequence: command.sequence,
		ScrollItemArchetypeID: intent.ScrollItemArchetypeID,
		ItemInstanceID: intent.ItemInstanceID,
		Outcome: protocol.EquipmentEnhancementOutcomeEnhanced,
		PreviousLevel: previous,
		CurrentLevel: updated.EnhancementLevel,
		ScrollConsumed: true,
	})
}

var _ = inventory.ErrInstanceNotFound
