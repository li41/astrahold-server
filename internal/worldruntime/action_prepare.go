package worldruntime

import (
	"time"

	"github.com/li41/astrahold-server/internal/character"
	"github.com/li41/astrahold-server/internal/session"
)

func (r *Runtime) applyUseAction(name string, command useActionCommand, tick uint64, delta time.Duration, report *StepReport) {
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
	// Action sequence 代表 intent 已被 Server 處理；即使 actor 已 Defeated 而被 gameplay
	// rule 拒絕，也必須消耗 sequence，避免同一 intent 在 revive/respawn 後被重播。
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

	// Network/session authority ends here. Combat execution consumes an ActorEntityID intent so
	// future Server-owned AI can reuse the same legality/damage path without inventing fake Sessions.
	intent := combatIntentFromClientAction(s.EntityID, command.action)
	r.prepareAndDispatchAction(name, command.sessionID, command.sequence, intent, tick, delta, report)
}
