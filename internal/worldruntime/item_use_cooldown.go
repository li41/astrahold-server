package worldruntime

import (
	"errors"

	"github.com/li41/astrahold-server/internal/character"
	"github.com/li41/astrahold-server/internal/characteridentity"
	"github.com/li41/astrahold-server/internal/inventory"
	"github.com/li41/astrahold-server/internal/protocol"
	"github.com/li41/astrahold-server/internal/session"
)

var ErrItemUseCooldown = errors.New("worldruntime: item-use cooldown active")

type itemUseCooldownKey struct {
	CharacterID characteridentity.ID
	Group       string
}

func (r *Runtime) itemUseCooldown(characterID characteridentity.ID, group string, tick uint64) (uint64, bool) {
	readyTick, ok := r.itemUseCooldownReadyTick[itemUseCooldownKey{CharacterID: characterID, Group: group}]
	return readyTick, ok && tick < readyTick
}

func (r *Runtime) commitItemUseCooldown(characterID characteridentity.ID, group string, tick, cooldownTicks uint64) uint64 {
	readyTick := tick + cooldownTicks
	if readyTick < tick {
		readyTick = ^uint64(0)
	}
	r.itemUseCooldownReadyTick[itemUseCooldownKey{CharacterID: characterID, Group: group}] = readyTick
	return readyTick
}

func (r *Runtime) pruneItemUseCooldowns(tick uint64) {
	for key, readyTick := range r.itemUseCooldownReadyTick {
		if readyTick <= tick {
			delete(r.itemUseCooldownReadyTick, key)
		}
	}
}

func (r *Runtime) rejectItemUse(
	name string,
	s *session.Session,
	clientActionSequence uint32,
	itemArchetypeID string,
	err error,
	cooldownReadyTick uint64,
	report *StepReport,
) {
	report.CommandErrors = append(report.CommandErrors, CommandError{Command: name, SessionID: s.ID, Err: err})
	r.sendItemUseResult(s, protocol.ItemUseResult{
		ClientActionSequence: clientActionSequence,
		ItemArchetypeID:      itemArchetypeID,
		Outcome:              protocol.ItemUseOutcomeRejected,
		Reason:               itemUseRejectionReason(err),
		CooldownReadyTick:    cooldownReadyTick,
	}, report)
}

// sendItemUseResult preserves per-session result order across temporary Reliable backpressure.
// Authoritative gameplay has already been decided before this presentation/correlation result is
// emitted, so retry only re-sends the result message and never re-applies item consumption/vitals.
func (r *Runtime) sendItemUseResult(s *session.Session, result protocol.ItemUseResult, report *StepReport) {
	if s == nil || report == nil {
		return
	}
	if len(r.pendingItemUseResults[s.ID]) > 0 {
		r.pendingItemUseResults[s.ID] = append(r.pendingItemUseResults[s.ID], result)
		return
	}
	if err := r.trySendItemUseResult(s, result, report.Tick, report); err != nil {
		if errors.Is(err, session.ErrBackpressure) {
			r.pendingItemUseResults[s.ID] = append(r.pendingItemUseResults[s.ID], result)
			return
		}
		report.DeliveryErrors = append(report.DeliveryErrors, DeliveryError{
			SessionID:   s.ID,
			Delivery:    protocol.DeliveryReliableOrdered,
			MessageType: protocol.MessageItemUseResult,
			Err:         err,
		})
	}
}

func (r *Runtime) trySendItemUseResult(s *session.Session, result protocol.ItemUseResult, tick uint64, report *StepReport) error {
	envelope := protocol.Envelope{
		Delivery:   protocol.DeliveryReliableOrdered,
		Sequence:   s.NextOutboundSequence(protocol.DeliveryReliableOrdered),
		ServerTick: tick,
		Message:    result,
	}
	report.Metrics.OutboundMessages++
	return s.Connection().TrySend(envelope)
}

// retryPendingItemUseResults drains retained results in FIFO order. Backpressure keeps the
// remaining suffix for a later tick. A non-backpressure transport error is terminal for this
// source session's presentation feedback; gameplay truth remains recoverable from snapshots.
func (r *Runtime) retryPendingItemUseResults(tick uint64, report *StepReport) {
	if report == nil || len(r.pendingItemUseResults) == 0 {
		return
	}
	for _, s := range r.sessions.List() {
		pending := r.pendingItemUseResults[s.ID]
		for len(pending) > 0 {
			result := pending[0]
			if err := r.trySendItemUseResult(s, result, tick, report); err != nil {
				if errors.Is(err, session.ErrBackpressure) {
					break
				}
				report.DeliveryErrors = append(report.DeliveryErrors, DeliveryError{
					SessionID:   s.ID,
					Delivery:    protocol.DeliveryReliableOrdered,
					MessageType: protocol.MessageItemUseResult,
					Err:         err,
				})
				pending = nil
				break
			}
			pending = pending[1:]
		}
		if len(pending) == 0 {
			delete(r.pendingItemUseResults, s.ID)
		} else {
			r.pendingItemUseResults[s.ID] = pending
		}
	}
	for id := range r.pendingItemUseResults {
		if _, ok := r.sessions.Get(id); !ok {
			delete(r.pendingItemUseResults, id)
		}
	}
}

func itemUseRejectionReason(err error) protocol.ItemUseRejectionReason {
	switch {
	case errors.Is(err, ErrItemUseCooldown):
		return protocol.ItemUseRejectionCooldown
	case errors.Is(err, character.ErrResourceFull):
		return protocol.ItemUseRejectionResourceFull
	case errors.Is(err, character.ErrCharacterDefeated):
		return protocol.ItemUseRejectionDefeated
	case errors.Is(err, inventory.ErrInsufficient):
		return protocol.ItemUseRejectionMissingItem
	case errors.Is(err, ErrItemNotUsable):
		return protocol.ItemUseRejectionNotUsable
	default:
		return protocol.ItemUseRejectionServerRejected
	}
}
