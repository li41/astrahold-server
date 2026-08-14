package worldruntime

import (
	"time"

	"github.com/li41/astrahold-server/internal/session"
)

func (r *Runtime) applyUseAction(name string, command useActionCommand, tick uint64, delta time.Duration, report *StepReport) {
	if r.combat == nil {
		report.CommandErrors = append(report.CommandErrors, CommandError{Command:name,SessionID:command.sessionID,Err:ErrCombatUnavailable})
		return
	}
	s, ok := r.sessions.Get(command.sessionID)
	if !ok {
		report.CommandErrors = append(report.CommandErrors, CommandError{Command:name,SessionID:command.sessionID,Err:session.ErrSessionNotFound})
		return
	}
	if err := s.ValidateActionSequence(command.sequence); err != nil {
		report.CommandErrors = append(report.CommandErrors, CommandError{Command:name,SessionID:command.sessionID,Err:err})
		return
	}
	s.MarkProcessedAction(command.sequence)
	if _, ok := r.world.Entity(s.EntityID); !ok {
		report.CommandErrors = append(report.CommandErrors, CommandError{Command:name,SessionID:command.sessionID,Err:ErrSessionEntityNotFound})
		return
	}
	r.prepareAndDispatchAction(name, command, s.EntityID, tick, delta, report)
}
