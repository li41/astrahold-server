package worldruntime

import (
	"errors"

	"github.com/li41/astrahold-server/internal/gameplayworld"
	"github.com/li41/astrahold-server/internal/protocol"
	"github.com/li41/astrahold-server/internal/session"
	"github.com/li41/astrahold-server/internal/world"
)

var ErrDynamicWorldUnavailable = errors.New("worldruntime: dynamic world unavailable")

// DynamicWorld 是 WorldRuntime 對動態 Gameplay Proxy 的最小需求。
// Siege/Gate domain 只透過此 contract 操作 blocker/LOS，不依賴 navigation implementation。
type DynamicWorld interface {
	SetBlockerEnabled(id string, enabled bool) error
	BlockerEnabled(id string) (bool, error)
	BlockerDefinition(id string) (gameplayworld.Blocker, error)
	BlockerStates() []gameplayworld.BlockerState
	HasLineOfSightIgnoringBlocker(from, to world.Position, ignoreBlockerID string) bool
}

type Option func(*Runtime)

func WithDynamicWorld(dynamic DynamicWorld) Option {
	return func(r *Runtime) { r.dynamic = dynamic }
}

func (r *Runtime) EnqueueSetBlocker(id string, enabled bool) error {
	if id == "" {
		return errors.New("worldruntime: blocker id is required")
	}
	return r.queue.tryPush(setBlockerCommand{id: id, enabled: enabled})
}

func (r *Runtime) applySetBlocker(name string, command setBlockerCommand, report *StepReport) {
	if r.dynamic == nil {
		report.CommandErrors = append(report.CommandErrors, CommandError{Command: name, Err: ErrDynamicWorldUnavailable})
		return
	}
	current, err := r.dynamic.BlockerEnabled(command.id)
	if err != nil {
		report.CommandErrors = append(report.CommandErrors, CommandError{Command: name, Err: err})
		return
	}
	if current == command.enabled {
		return
	}
	if err := r.dynamic.SetBlockerEnabled(command.id, command.enabled); err != nil {
		report.CommandErrors = append(report.CommandErrors, CommandError{Command: name, Err: err})
		return
	}
	r.bumpDynamicRevision()
}

func (r *Runtime) bumpDynamicRevision() {
	r.dynamicRevision++
	if r.dynamicRevision == 0 {
		r.dynamicRevision = 1
	}
}

func (r *Runtime) replicateDynamicState(tick uint64, report *StepReport) {
	if r.dynamic == nil || r.dynamicRevision == 0 {
		return
	}

	domainStates := r.dynamic.BlockerStates()
	blockers := make([]protocol.WorldBlockerState, len(domainStates))
	for i, state := range domainStates {
		blockers[i] = protocol.WorldBlockerState{ID: state.ID, Enabled: state.Enabled}
	}
	gates := make([]protocol.WorldGateState, 0)
	if r.siege != nil {
		states := r.siege.States()
		gates = make([]protocol.WorldGateState, len(states))
		for i, state := range states {
			gates[i] = protocol.WorldGateState{ID: state.ID, HP: state.HP, MaxHP: state.MaxHP, Destroyed: state.Destroyed}
		}
	}
	message := protocol.WorldDynamicState{Revision: r.dynamicRevision, Blockers: blockers, Gates: gates}

	sessions := r.sessions.List()
	active := make(map[session.ID]struct{}, len(sessions))
	for _, s := range sessions {
		active[s.ID] = struct{}{}
		if r.sessionDynamicRevision[s.ID] >= r.dynamicRevision {
			continue
		}
		envelope := protocol.Envelope{
			Delivery:   protocol.DeliveryReliableOrdered,
			Sequence:   s.NextOutboundSequence(protocol.DeliveryReliableOrdered),
			ServerTick: tick,
			Message:    message,
		}
		if err := s.Connection().TrySend(envelope); err != nil {
			report.DeliveryErrors = append(report.DeliveryErrors, DeliveryError{
				SessionID: s.ID, Delivery: envelope.Delivery, MessageType: message.Type(), Err: err,
			})
			continue
		}
		r.sessionDynamicRevision[s.ID] = r.dynamicRevision
	}

	for id := range r.sessionDynamicRevision {
		if _, ok := active[id]; !ok {
			delete(r.sessionDynamicRevision, id)
		}
	}
}
