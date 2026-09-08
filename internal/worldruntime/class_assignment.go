package worldruntime

import (
	"errors"
	"fmt"

	"github.com/li41/astrahold-server/internal/character"
	"github.com/li41/astrahold-server/internal/characteridentity"
	"github.com/li41/astrahold-server/internal/characterstate"
	"github.com/li41/astrahold-server/internal/classid"
	"github.com/li41/astrahold-server/internal/inventory"
	"github.com/li41/astrahold-server/internal/session"
)

var (
	ErrInitialClassAssignmentRequiresPersistence    = errors.New("worldruntime: initial class assignment requires durable character persistence")
	ErrInitialClassAssignmentRequiresTrustedIdentity = errors.New("worldruntime: initial class assignment requires trusted character identity")
	ErrInitialClassAssignmentPending                = errors.New("worldruntime: initial class assignment already pending")
	ErrInitialClassAssignmentEquipmentIllegal       = errors.New("worldruntime: equipped item is illegal for target class")
	ErrInitialClassAssignmentCompletionInvalid      = errors.New("worldruntime: invalid durable initial class assignment completion")
)

type initialClassAssignmentCommand struct {
	sessionID session.ID
	ownership SessionOwnershipFence
	target    classid.ID
}

func (initialClassAssignmentCommand) name() string { return "initial_class_assignment" }

// EnqueueFencedInitialClassAssignment requests the only supported profession mutation:
// unassigned -> one canonical ClassID. The ownership fence prevents a stale transport owner from
// selecting a profession after session takeover. This enqueue performs no durability I/O and no
// live gameplay mutation.
func (r *Runtime) EnqueueFencedInitialClassAssignment(ownership SessionOwnershipFence, target classid.ID) error {
	if !ownership.Valid() {
		return ErrCharacterOwnershipFenceInvalid
	}
	if target == "" || !classid.IsCanonical(target) {
		return character.ErrInvalidClassAssignment
	}
	return r.queue.tryPush(initialClassAssignmentCommand{
		sessionID: ownership.SessionID,
		ownership: ownership,
		target:    target,
	})
}

func (r *Runtime) applyInitialClassAssignment(name string, command initialClassAssignmentCommand, report *StepReport) {
	fail := func(err error) {
		report.CommandErrors = append(report.CommandErrors, CommandError{Command: name, SessionID: command.sessionID, Err: err})
	}
	if err := r.characterIdentities.validateOwnership(command.sessionID, command.ownership); err != nil {
		fail(err)
		return
	}
	s, ok := r.sessions.Get(command.sessionID)
	if !ok {
		fail(session.ErrSessionNotFound)
		return
	}
	if !s.CharacterIdentity.Valid() || s.CharacterIdentity.Assurance != characteridentity.AssuranceTrusted {
		fail(ErrInitialClassAssignmentRequiresTrustedIdentity)
		return
	}
	if s.CharacterIdentity.ID != command.ownership.CharacterID || s.EntityID != command.ownership.EntityID {
		fail(ErrCharacterOwnershipFenceStale)
		return
	}
	if command.target == "" || !classid.IsCanonical(command.target) {
		fail(character.ErrInvalidClassAssignment)
		return
	}
	state, ok := r.characters.State(s.EntityID)
	if !ok {
		fail(character.ErrCharacterNotFound)
		return
	}
	if state.ClassID != "" {
		fail(character.ErrClassAlreadyAssigned)
		return
	}
	if r.characterStateOutbox == nil {
		fail(ErrInitialClassAssignmentRequiresPersistence)
		return
	}
	if _, pending := r.characterStateOutbox.CompletionForCharacter(s.CharacterIdentity.ID); pending {
		fail(ErrInitialClassAssignmentPending)
		return
	}
	inv := r.inventories[s.CharacterIdentity.ID]
	if !initialClassAssignmentEquipmentAllowed(inv, command.target) {
		fail(ErrInitialClassAssignmentEquipmentIllegal)
		return
	}

	binding, snapshot, ok := r.captureCharacterStateSnapshot(command.sessionID, s.EntityID, report)
	if !ok {
		return
	}
	if snapshot.ClassID != "" {
		fail(ErrInitialClassAssignmentPending)
		return
	}
	snapshot.ClassID = command.target
	if _, err := r.characterStateOutbox.EnqueueWithCompletion(binding, snapshot); err != nil {
		if errors.Is(err, characterstate.ErrSaveCompletionPending) {
			err = ErrInitialClassAssignmentPending
		}
		fail(err)
		if report != nil {
			report.Metrics.CharacterStateSaveIntentFailures++
		}
		return
	}
	if report != nil {
		report.Metrics.CharacterStateSaveIntentsEnqueued++
	}
}

func initialClassAssignmentEquipmentAllowed(inv *inventory.Inventory, target classid.ID) bool {
	if inv == nil || target == "" || !classid.IsCanonical(target) {
		return false
	}
	if mainHand := inv.MainHand(); mainHand != "" && !mainHandItemAllowedForClass(mainHand, target) {
		return false
	}
	if offHand := inv.OffHand(); offHand != "" && !offHandItemAllowedForClass(offHand, target) {
		return false
	}
	return true
}

// applyCharacterStateSaveCompletions is called only by Runtime.Step, before normal queued
// gameplay commands. The persistence worker publishes an acknowledgement only after journal fsync,
// Store application and checkpoint advancement. The world owner applies first, then releases the
// completion transaction, so an invariant failure cannot erase the durable target via later saves.
func (r *Runtime) applyCharacterStateSaveCompletions(report *StepReport) {
	if r.characterStateOutbox == nil {
		return
	}
	limit := r.config.MaxCommandsPerTick
	if limit <= 0 {
		limit = 1
	}
	for _, intent := range r.characterStateOutbox.Completed(limit) {
		if err := r.applyDurableInitialClassAssignmentCompletion(intent); err != nil {
			report.CommandErrors = append(report.CommandErrors, CommandError{Command: "complete_initial_class_assignment", Err: err})
			return
		}
		if err := r.characterStateOutbox.ConfirmCompletion(intent.IntentID); err != nil {
			report.CommandErrors = append(report.CommandErrors, CommandError{Command: "complete_initial_class_assignment", Err: err})
			return
		}
	}
}

func (r *Runtime) applyDurableInitialClassAssignmentCompletion(intent characterstate.SaveIntent) error {
	if !intent.CompletionRequested || !intent.Identity.Valid() || intent.Identity.Assurance != characteridentity.AssuranceTrusted {
		return ErrInitialClassAssignmentCompletionInvalid
	}
	target := intent.Snapshot.ClassID
	if target == "" || !classid.IsCanonical(target) {
		return ErrInitialClassAssignmentCompletionInvalid
	}

	ownership, err := r.characterIdentities.currentOwnership(intent.Identity)
	if err != nil {
		if errors.Is(err, ErrCharacterOwnershipNotActive) {
			// The durable assignment already committed, but the character left before the world
			// owner consumed the acknowledgement. Reconnect restores the Store's ClassID.
			return nil
		}
		return err
	}
	state, ok := r.characters.State(ownership.EntityID)
	if !ok {
		return fmt.Errorf("%w: character=%s entity=%d: %v", ErrInitialClassAssignmentCompletionInvalid, intent.Identity.ID, ownership.EntityID, character.ErrCharacterNotFound)
	}
	if state.ClassID == target {
		return nil
	}
	if state.ClassID != "" {
		return fmt.Errorf("%w: character=%s current=%s durable=%s", ErrInitialClassAssignmentCompletionInvalid, intent.Identity.ID, state.ClassID, target)
	}
	if !initialClassAssignmentEquipmentAllowed(r.inventories[intent.Identity.ID], target) {
		return ErrInitialClassAssignmentEquipmentIllegal
	}
	_, err = r.characters.AssignInitialClass(ownership.EntityID, target)
	return err
}
