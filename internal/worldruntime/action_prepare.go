package worldruntime

import (
	"time"

	"github.com/li41/astrahold-server/internal/character"
	"github.com/li41/astrahold-server/internal/session"
)

func (r *Runtime) applyUseAction(name string, command useActionCommand, tick uint64, delta time.Duration, report *StepReport) {
	// Equipment, enhancement, warehouse, pickup, item-use and respawn share the existing bounded
	// Reliable client-intent carrier, but remain distinct typed payloads and never enter combat
	// preparation or the skill/action path.
	if command.equipmentInstance != nil {
		r.applyEquipmentInstanceCommand(name, command, report)
		return
	}
	if command.enhancement != nil {
		r.applyEnhanceEquipment(name, command, report)
		return
	}
	if command.warehouse != nil {
		r.applyWarehouseCommand(name, command, report)
		return
	}
	if command.ammunition != nil {
		r.applyAmmunitionCommand(name, command, report)
		return
	}
	if command.equipment != nil {
		r.applyEquipmentCommand(name, command, report)
		return
	}
	if command.pickup != nil {
		r.applyPickupItem(name, command, report)
		return
	}
	if command.useItem != nil {
		r.applyUseItem(name, command, report)
		return
	}
	if command.respawn != nil {
		r.applyRespawnRequest(name, command, tick, report)
		return
	}
	if command.ownership.Valid() {
		if err := r.characterIdentities.validateOwnership(command.sessionID, command.ownership); err != nil {
			report.CommandErrors = append(report.CommandErrors, CommandError{Command: name, SessionID: command.sessionID, Err: err})
			return
		}
	}
	if r.combat == nil {
		report.CommandErrors = append(report.CommandErrors, CommandError{Command: name, SessionID: command.sessionID, Err: ErrCombatUnavailable})
		return
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
	if _, ok := r.world.Entity(s.EntityID); !ok {
		report.CommandErrors = append(report.CommandErrors, CommandError{Command: name, SessionID: command.sessionID, Err: ErrSessionEntityNotFound})
		return
	}
	state, ok := r.characters.State(s.EntityID)
	if !ok {
		report.CommandErrors = append(report.CommandErrors, CommandError{Command: name, SessionID: command.sessionID, Err: character.ErrCharacterNotFound})
		return
	}
	if state.Defeated {
		r.rejectClientAction(name, command.sessionID, command.sequence, s.EntityID, command.action.ActionID, command.action.TargetKind, character.ErrCharacterDefeated, tick, report)
		return
	}
	intent := combatIntentFromClientAction(s.EntityID, command.action)
	r.prepareAndDispatchAction(name, command.sessionID, command.sequence, intent, tick, delta, report)
}
