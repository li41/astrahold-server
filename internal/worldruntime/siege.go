package worldruntime

import (
	"errors"
	"time"

	"github.com/li41/astrahold-server/internal/gameplayworld"
	"github.com/li41/astrahold-server/internal/session"
	"github.com/li41/astrahold-server/internal/siege"
)

var ErrSiegeUnavailable = errors.New("worldruntime: siege unavailable")

func WithSiegeGates(gates []gameplayworld.Gate) Option {
	return func(r *Runtime) {
		if len(gates) > 0 {
			r.siege = siege.NewService(gates)
		}
	}
}

func (r *Runtime) EnqueueAttackGate(id session.ID, sequence uint32, gateID string) error {
	if id == 0 || sequence == 0 || gateID == "" {
		return errors.New("worldruntime: invalid gate attack")
	}
	return r.queue.tryPush(attackGateCommand{sessionID: id, sequence: sequence, gateID: gateID})
}

func (r *Runtime) applyAttackGate(name string, command attackGateCommand, tick uint64, delta time.Duration, report *StepReport) {
	if r.siege == nil || r.dynamic == nil {
		report.CommandErrors = append(report.CommandErrors, CommandError{Command: name, SessionID: command.sessionID, Err: ErrSiegeUnavailable})
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
	// Sequence 表示已處理的 action intent，而不是「成功造成 damage」。
	// 即使 gameplay validation 拒絕，也不能允許同一 reliable action 被重播。
	s.MarkProcessedAction(command.sequence)

	entity, ok := r.world.Entity(s.EntityID)
	if !ok {
		report.CommandErrors = append(report.CommandErrors, CommandError{Command: name, SessionID: command.sessionID, Err: ErrSessionEntityNotFound})
		return
	}
	if _, err := r.siege.Attack(s.EntityID, entity.Transform.Position, command.gateID, tick, delta, r.dynamic); err != nil {
		if isExpectedGateRejection(err) {
			report.ActionRejections = append(report.ActionRejections, ActionRejection{Action: name, SessionID: command.sessionID, Err: err})
			return
		}
		report.CommandErrors = append(report.CommandErrors, CommandError{Command: name, SessionID: command.sessionID, Err: err})
		return
	}
	r.bumpDynamicRevision()
}

func isExpectedGateRejection(err error) bool {
	return errors.Is(err, siege.ErrUnknownGate) ||
		errors.Is(err, siege.ErrGateDestroyed) ||
		errors.Is(err, siege.ErrGateWrongLayer) ||
		errors.Is(err, siege.ErrGateOutOfRange) ||
		errors.Is(err, siege.ErrGateNoLineOfSight) ||
		errors.Is(err, siege.ErrGateAttackCooldown)
}
