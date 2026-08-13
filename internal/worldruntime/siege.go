package worldruntime

import (
	"errors"
	"time"

	"github.com/li41/astrahold-server/internal/combat"
	"github.com/li41/astrahold-server/internal/gameplayworld"
	"github.com/li41/astrahold-server/internal/protocol"
	"github.com/li41/astrahold-server/internal/session"
	"github.com/li41/astrahold-server/internal/siege"
)

var (
	ErrSiegeUnavailable  = errors.New("worldruntime: siege unavailable")
	ErrCombatUnavailable = errors.New("worldruntime: combat unavailable")
)

func WithSiegeGates(gates []gameplayworld.Gate) Option {
	return func(r *Runtime) {
		if len(gates) > 0 {
			r.siege = siege.NewService(gates)
		}
	}
}

func WithCombatService(service *combat.Service) Option {
	return func(r *Runtime) { r.combat = service }
}

func (r *Runtime) EnqueueUseAction(id session.ID, sequence uint32, action protocol.ClientUseAction) error {
	if id == 0 || sequence == 0 || action.ActionID == "" || action.TargetKind == "" || action.TargetID == "" {
		return errors.New("worldruntime: invalid action intent")
	}
	return r.queue.tryPush(useActionCommand{sessionID: id, sequence: sequence, action: action})
}

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
	// Sequence 表示這個 reliable action intent 已處理，不代表 gameplay 一定成功。
	// 因此 validation rejection 也不可用同一 sequence 重播。
	s.MarkProcessedAction(command.sequence)

	entity, ok := r.world.Entity(s.EntityID)
	if !ok {
		report.CommandErrors = append(report.CommandErrors, CommandError{Command: name, SessionID: command.sessionID, Err: ErrSessionEntityNotFound})
		return
	}

	prepared, err := r.combat.Prepare(
		s.EntityID,
		command.action.ActionID,
		combat.Target{Kind: combat.TargetKind(command.action.TargetKind), ID: command.action.TargetID},
		tick,
	)
	if err != nil {
		if isExpectedCombatRejection(err) {
			report.ActionRejections = append(report.ActionRejections, ActionRejection{Action: name, SessionID: command.sessionID, Err: err})
			return
		}
		report.CommandErrors = append(report.CommandErrors, CommandError{Command: name, SessionID: command.sessionID, Err: err})
		return
	}

	switch prepared.Target.Kind {
	case combat.TargetGate:
		if r.siege == nil || r.dynamic == nil {
			report.CommandErrors = append(report.CommandErrors, CommandError{Command: name, SessionID: command.sessionID, Err: ErrSiegeUnavailable})
			return
		}
		if _, err := r.siege.ApplyActionDamage(entity.Transform.Position, prepared.Target.ID, prepared.Definition.Range, prepared.Damage, r.dynamic); err != nil {
			if isExpectedGateRejection(err) {
				report.ActionRejections = append(report.ActionRejections, ActionRejection{Action: name, SessionID: command.sessionID, Err: err})
				return
			}
			report.CommandErrors = append(report.CommandErrors, CommandError{Command: name, SessionID: command.sessionID, Err: err})
			return
		}
		r.combat.Commit(prepared, tick, delta)
		r.bumpDynamicRevision()
	default:
		report.ActionRejections = append(report.ActionRejections, ActionRejection{Action: name, SessionID: command.sessionID, Err: combat.ErrTargetNotAllowed})
	}
}

func isExpectedCombatRejection(err error) bool {
	return errors.Is(err, combat.ErrUnknownAction) ||
		errors.Is(err, combat.ErrTargetNotAllowed) ||
		errors.Is(err, combat.ErrActionCooldown)
}

func isExpectedGateRejection(err error) bool {
	return errors.Is(err, siege.ErrUnknownGate) ||
		errors.Is(err, siege.ErrGateDestroyed) ||
		errors.Is(err, siege.ErrGateWrongLayer) ||
		errors.Is(err, siege.ErrGateOutOfRange) ||
		errors.Is(err, siege.ErrGateNoLineOfSight)
}
