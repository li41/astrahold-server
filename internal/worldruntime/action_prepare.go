package worldruntime

import (
	"time"

	"github.com/li41/astrahold-server/internal/character"
	"github.com/li41/astrahold-server/internal/session"
)

func (r *Runtime) applyUseAction(name string, command useActionCommand, tick uint64, delta time.Duration, report *StepReport) {
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
		report.ActionRejections = append(report.ActionRejections, ActionRejection{Action: name, SessionID: command.sessionID, Err: character.ErrCharacterDefeated})
		return
	}
	r.prepareAndDispatchAction(name, command, s.EntityID, tick, delta, report)
}
