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

func (r *Runtime) sendItemUseResult(s *session.Session, result protocol.ItemUseResult, report *StepReport) {
	if s == nil || report == nil {
		return
	}
	envelope := protocol.Envelope{
		Delivery:   protocol.DeliveryReliableOrdered,
		Sequence:   s.NextOutboundSequence(protocol.DeliveryReliableOrdered),
		ServerTick: report.Tick,
		Message:    result,
	}
	report.Metrics.OutboundMessages++
	if err := s.Connection().TrySend(envelope); err != nil {
		report.DeliveryErrors = append(report.DeliveryErrors, DeliveryError{
			SessionID:   s.ID,
			Delivery:    envelope.Delivery,
			MessageType: protocol.MessageItemUseResult,
			Err:         err,
		})
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
