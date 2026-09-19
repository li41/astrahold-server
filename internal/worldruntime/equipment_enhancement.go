package worldruntime

import (
	"errors"
	"math/rand"
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
)

type equipmentEnhancementChances struct {
	Success  int
	NoChange int
	Break    int
}

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

// equipmentEnhancementChanceFor is the formal normal-scroll probability table.
// There is intentionally no gameplay maximum enhancement level. Tail levels keep using 1/39/60.
func equipmentEnhancementChanceFor(kind equipmentcatalog.Kind, level uint16) (equipmentEnhancementChances, bool) {
	switch kind {
	case equipmentcatalog.KindWeapon:
		if level < 6 {
			return equipmentEnhancementChances{Success: 100}, true
		}
		if level >= 12 {
			return equipmentEnhancementChances{Success: 1, NoChange: 39, Break: 60}, true
		}
		step := int(level - 6)
		success := 60 - step*10
		noChange := 20 + step*5
		return equipmentEnhancementChances{Success: success, NoChange: noChange, Break: 100 - success - noChange}, true
	case equipmentcatalog.KindArmor, equipmentcatalog.KindShield:
		if level < 4 {
			return equipmentEnhancementChances{Success: 100}, true
		}
		if level >= 10 {
			return equipmentEnhancementChances{Success: 1, NoChange: 39, Break: 60}, true
		}
		step := int(level - 4)
		success := 60 - step*10
		noChange := 15 + step*5
		return equipmentEnhancementChances{Success: success, NoChange: noChange, Break: 100 - success - noChange}, true
	default:
		return equipmentEnhancementChances{}, false
	}
}

// resolveEquipmentEnhancementRoll is pure so probability boundaries can be deterministically tested.
// roll is the Server-owned percentile roll in [1,100].
func resolveEquipmentEnhancementRoll(kind equipmentcatalog.Kind, level uint16, roll int) (protocol.EquipmentEnhancementOutcome, bool) {
	if roll < 1 || roll > 100 {
		return "", false
	}
	chance, ok := equipmentEnhancementChanceFor(kind, level)
	if !ok {
		return "", false
	}
	if roll <= chance.Success {
		return protocol.EquipmentEnhancementOutcomeEnhanced, true
	}
	if roll <= chance.Success+chance.NoChange {
		return protocol.EquipmentEnhancementOutcomeNoChange, true
	}
	return protocol.EquipmentEnhancementOutcomeBroken, true
}

// commitEquipmentEnhancementOutcome applies exactly one already-resolved legal enhancement attempt.
// It owns scroll consumption and exact target mutation atomically enough for the world-owner path:
// if target mutation fails, the removed scroll is restored before returning an error.
func commitEquipmentEnhancementOutcome(inv *inventory.Inventory, scrollID string, instance iteminstance.Instance, outcome protocol.EquipmentEnhancementOutcome) (uint16, error) {
	if inv == nil {
		return instance.EnhancementLevel, ErrEquipmentEnhancementRejected
	}
	switch outcome {
	case protocol.EquipmentEnhancementOutcomeEnhanced, protocol.EquipmentEnhancementOutcomeNoChange, protocol.EquipmentEnhancementOutcomeBroken:
	default:
		return instance.EnhancementLevel, ErrEquipmentEnhancementRejected
	}
	if err := inv.Remove(scrollID, 1); err != nil {
		return instance.EnhancementLevel, err
	}
	restoreScroll := func() {
		_ = inv.Add(scrollID, 1)
	}

	switch outcome {
	case protocol.EquipmentEnhancementOutcomeEnhanced:
		// uint16 is only the current persistence/wire representation, never a gameplay max.
		// Do not wrap it; representation migration is required before this boundary becomes reachable.
		if instance.EnhancementLevel == ^uint16(0) {
			restoreScroll()
			return instance.EnhancementLevel, ErrEquipmentEnhancementRejected
		}
		updated := instance
		updated.EnhancementLevel++
		if _, err := inv.ReplaceOwnedInstance(updated); err != nil {
			restoreScroll()
			return instance.EnhancementLevel, err
		}
		return updated.EnhancementLevel, nil
	case protocol.EquipmentEnhancementOutcomeNoChange:
		return instance.EnhancementLevel, nil
	case protocol.EquipmentEnhancementOutcomeBroken:
		if _, _, err := inv.RemoveOwnedInstance(instance.ID); err != nil {
			restoreScroll()
			return instance.EnhancementLevel, err
		}
		return instance.EnhancementLevel, nil
	default:
		panic("unreachable equipment enhancement outcome")
	}
}

func (r *Runtime) queueEquipmentEnhancementResult(sessionID session.ID, result protocol.EquipmentEnhancementResult) {
	r.pendingResourceMessages[sessionID] = append(r.pendingResourceMessages[sessionID], result)
}

func (r *Runtime) rejectEquipmentEnhancement(command useActionCommand, reason protocol.EquipmentEnhancementRejectionReason, previous uint16) {
	intent := *command.enhancement
	r.queueEquipmentEnhancementResult(command.sessionID, protocol.EquipmentEnhancementResult{
		ClientActionSequence:  command.sequence,
		ScrollItemArchetypeID: intent.ScrollItemArchetypeID,
		ItemInstanceID:        intent.ItemInstanceID,
		Outcome:               protocol.EquipmentEnhancementOutcomeRejected,
		Reason:                reason,
		PreviousLevel:         previous,
		CurrentLevel:          previous,
		ScrollConsumed:        false,
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

	previous := instance.EnhancementLevel
	outcome, ok := resolveEquipmentEnhancementRoll(definition.Kind, previous, rand.Intn(100)+1)
	if !ok {
		r.rejectEquipmentEnhancement(command, protocol.EquipmentEnhancementRejectionServerRejected, previous)
		return
	}
	current, err := commitEquipmentEnhancementOutcome(inv, intent.ScrollItemArchetypeID, instance, outcome)
	if err != nil {
		reason := protocol.EquipmentEnhancementRejectionServerRejected
		if inv.Quantity(intent.ScrollItemArchetypeID) == 0 {
			// This is only observable if ownership changed unexpectedly inside the world-owner path.
			// The legal precheck above normally guarantees the scroll exists.
			reason = protocol.EquipmentEnhancementRejectionMissingScroll
		}
		r.rejectEquipmentEnhancement(command, reason, previous)
		return
	}

	r.sessionInventoryPending[command.sessionID] = struct{}{}
	r.queueEquipmentEnhancementResult(command.sessionID, protocol.EquipmentEnhancementResult{
		ClientActionSequence:  command.sequence,
		ScrollItemArchetypeID: intent.ScrollItemArchetypeID,
		ItemInstanceID:        intent.ItemInstanceID,
		Outcome:               outcome,
		PreviousLevel:         previous,
		CurrentLevel:          current,
		ScrollConsumed:        true,
	})
}
