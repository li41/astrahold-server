package worldruntime

import (
	"errors"
	"math"
	"sort"
	"time"

	"github.com/li41/astrahold-server/internal/character"
	"github.com/li41/astrahold-server/internal/classaction"
	"github.com/li41/astrahold-server/internal/protocol"
	"github.com/li41/astrahold-server/internal/session"
	"github.com/li41/astrahold-server/internal/targetresource"
	"github.com/li41/astrahold-server/internal/world"
)

const shadowbladeFrontDotThreshold float64 = 0.5

type targetResourceSpendPlan struct {
	ResourceID targetresource.ID
	Amount     uint32
	Damage     uint32
}

func isSideOrBackAttackPosition(actor, target world.EntityState) bool {
	dx := float64(actor.Transform.Position.X - target.Transform.Position.X)
	dz := float64(actor.Transform.Position.Z - target.Transform.Position.Z)
	length := math.Hypot(dx, dz)
	if length <= 1e-6 {
		return false
	}
	dx /= length
	dz /= length
	yaw := float64(target.Transform.Yaw) * math.Pi / 180
	forwardX := math.Sin(yaw)
	forwardZ := math.Cos(yaw)
	return forwardX*dx+forwardZ*dz < shadowbladeFrontDotThreshold
}

func (r *Runtime) prepareTargetResourceSpend(sourceID, targetID world.EntityID, actionID string) (targetResourceSpendPlan, bool, error) {
	policy, ok := classaction.ForAction(actionID)
	if !ok || policy.TargetSpend.ResourceID == "" {
		return targetResourceSpendPlan{}, false, nil
	}
	state, exists := r.characters.TargetResourceState(sourceID, targetID, policy.TargetSpend.ResourceID)
	if !exists || state.Current == 0 {
		return targetResourceSpendPlan{}, true, character.ErrInsufficientResource
	}
	amount, damage, ok := policy.TargetSpend.Resolve(state.Current)
	if !ok {
		return targetResourceSpendPlan{}, true, targetresource.ErrInvalidState
	}
	return targetResourceSpendPlan{ResourceID: policy.TargetSpend.ResourceID, Amount: amount, Damage: damage}, true, nil
}

func (r *Runtime) commitTargetResourceSpend(sourceSessionID session.ID, sourceID, targetID world.EntityID, plan targetResourceSpendPlan, report *StepReport) error {
	state, err := r.characters.SpendTargetResource(sourceID, targetID, plan.ResourceID, plan.Amount)
	if err != nil {
		return err
	}
	r.sendTargetResourceState(sourceSessionID, protocol.CharacterTargetResourceState{SourceEntityID: state.SourceEntityID, TargetEntityID: state.TargetEntityID, ResourceID: string(state.ResourceID), Current: state.Current, Max: state.Max}, report)
	return nil
}

func (r *Runtime) applyHitTargetResource(name string, sourceSessionID session.ID, actor, target world.EntityState, actionID string, tick uint64, delta time.Duration, report *StepReport) {
	policy, ok := classaction.ForAction(actionID)
	if !ok || policy.HitTargetResource == "" || policy.HitTargetGain == 0 || policy.HitTargetMax == 0 {
		return
	}
	if policy.RequireSideOrBack && !isSideOrBackAttackPosition(actor, target) {
		return
	}
	var (
		state   targetresource.State
		changed bool
		err     error
	)
	if policy.HitTargetPreserveReadyTick {
		state, changed, err = r.characters.GainTargetResourcePreservingReadyTick(actor.ID, target.ID, policy.HitTargetResource, policy.HitTargetGain, policy.HitTargetMax)
	} else {
		if delta <= 0 || policy.HitTargetICDSeconds <= 0 {
			report.CommandErrors = append(report.CommandErrors, CommandError{Command: name, SessionID: sourceSessionID, Err: targetresource.ErrInvalidState})
			return
		}
		cooldownTicks := uint64(math.Ceil(policy.HitTargetICDSeconds / delta.Seconds()))
		state, changed, err = r.characters.GainTargetResource(actor.ID, target.ID, policy.HitTargetResource, policy.HitTargetGain, policy.HitTargetMax, tick, tick+cooldownTicks)
	}
	if err != nil {
		report.CommandErrors = append(report.CommandErrors, CommandError{Command: name, SessionID: sourceSessionID, Err: err})
		return
	}
	if changed {
		r.sendTargetResourceState(sourceSessionID, protocol.CharacterTargetResourceState{SourceEntityID: state.SourceEntityID, TargetEntityID: state.TargetEntityID, ResourceID: string(state.ResourceID), Current: state.Current, Max: state.Max}, report)
	}
}

func sameTargetResourceState(a, b protocol.CharacterTargetResourceState) bool {
	return a.SourceEntityID == b.SourceEntityID && a.TargetEntityID == b.TargetEntityID && a.ResourceID == b.ResourceID
}

func (r *Runtime) sendTargetResourceState(sessionID session.ID, message protocol.CharacterTargetResourceState, report *StepReport) {
	s, ok := r.sessions.Get(sessionID)
	if !ok || s.EntityID != message.SourceEntityID || report == nil {
		return
	}
	pending := r.pendingClassMessages[s.ID]
	for i, existing := range pending {
		if current, ok := existing.(protocol.CharacterTargetResourceState); ok && sameTargetResourceState(current, message) {
			pending[i] = message
			r.pendingClassMessages[s.ID] = pending
			return
		}
	}
	if len(pending) > 0 {
		if len(pending) >= maxPendingResourceMessagesPerSession {
			_ = s.Connection().Close()
			return
		}
		r.pendingClassMessages[s.ID] = append(pending, message)
		return
	}
	if err := r.trySendResourceMessage(s, message, report.Tick, report); err != nil {
		if errors.Is(err, session.ErrBackpressure) {
			r.pendingClassMessages[s.ID] = []protocol.Message{message}
			return
		}
		report.DeliveryErrors = append(report.DeliveryErrors, DeliveryError{SessionID: s.ID, Delivery: protocol.DeliveryReliableOrdered, MessageType: message.Type(), Err: err})
	}
}

func (r *Runtime) clearTargetResourcesForEntity(entityID world.EntityID, report *StepReport) {
	removed := r.characters.ClearTargetResourcesForEntity(entityID)
	if len(removed) == 0 || report == nil {
		return
	}
	sessions := r.sessions.List()
	sort.Slice(removed, func(i, j int) bool { return removed[i].SourceEntityID < removed[j].SourceEntityID })
	for _, state := range removed {
		if state.SourceEntityID == entityID || state.Current == 0 {
			continue
		}
		for _, s := range sessions {
			if s.EntityID == state.SourceEntityID {
				r.sendTargetResourceState(s.ID, protocol.CharacterTargetResourceState{SourceEntityID: state.SourceEntityID, TargetEntityID: state.TargetEntityID, ResourceID: string(state.ResourceID), Current: 0, Max: state.Max}, report)
				break
			}
		}
	}
}
