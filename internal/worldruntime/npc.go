package worldruntime

import (
	"errors"

	"github.com/li41/astrahold-server/internal/character"
	"github.com/li41/astrahold-server/internal/protocol"
	"github.com/li41/astrahold-server/internal/session"
	"github.com/li41/astrahold-server/internal/world"
)

const (
	playtestNPCArchetypeID      = "npc_emberwatch_warden"
	playtestNPCDisplayName      = "Warden Sera"
	playtestNPCDialogue         = "The eastern road is secure. Keep your blade ready beyond the gate."
	npcInteractionRangeMeters   = float32(3.0)
)

var (
	ErrInvalidNPCIntent      = errors.New("worldruntime: invalid NPC interaction intent")
	ErrNPCNotFound           = errors.New("worldruntime: NPC not found")
	ErrNPCWrongLayer         = errors.New("worldruntime: NPC wrong layer")
	ErrNPCOutOfRange         = errors.New("worldruntime: NPC out of range")
	ErrNPCInteractionMissing = errors.New("worldruntime: NPC interaction definition missing")
)

func npcInteractionFor(entity world.EntityState) (protocol.NPCInteraction, error) {
	if entity.Kind != world.EntityNPC || entity.ID == 0 {
		return protocol.NPCInteraction{}, ErrNPCNotFound
	}
	switch entity.ArchetypeID {
	case playtestNPCArchetypeID:
		return protocol.NPCInteraction{
			NPCEntityID:    entity.ID,
			NPCArchetypeID: entity.ArchetypeID,
			DisplayName:    playtestNPCDisplayName,
			Text:           playtestNPCDialogue,
		}, nil
	default:
		return protocol.NPCInteraction{}, ErrNPCInteractionMissing
	}
}

func (r *Runtime) EnqueueInteractNPC(id session.ID, sequence uint32, intent protocol.ClientInteractNPC) error {
	if id == 0 || sequence == 0 || intent.NPCEntityID == 0 {
		return ErrInvalidNPCIntent
	}
	return r.queue.tryPush(npcCommand{sessionID: id, sequence: sequence, intent: intent})
}

func (r *Runtime) EnqueueFencedInteractNPC(ownership SessionOwnershipFence, sequence uint32, intent protocol.ClientInteractNPC) error {
	if !ownership.Valid() || sequence == 0 || intent.NPCEntityID == 0 {
		return ErrInvalidNPCIntent
	}
	return r.queue.tryPush(npcCommand{sessionID: ownership.SessionID, sequence: sequence, intent: intent, ownership: ownership})
}

func (r *Runtime) applyInteractNPC(name string, command npcCommand, report *StepReport) {
	if command.ownership.Valid() {
		if err := r.characterIdentities.validateOwnership(command.sessionID, command.ownership); err != nil {
			report.CommandErrors = append(report.CommandErrors, CommandError{Command: name, SessionID: command.sessionID, Err: err})
			return
		}
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
	// The reliable intent is consumed once the world owner processes it, including gameplay rejection.
	s.MarkProcessedAction(command.sequence)

	player, ok := r.world.Entity(s.EntityID)
	if !ok || player.Kind != world.EntityPlayer {
		report.CommandErrors = append(report.CommandErrors, CommandError{Command: name, SessionID: command.sessionID, Err: ErrSessionEntityNotFound})
		return
	}
	state, ok := r.characters.State(s.EntityID)
	if !ok {
		report.CommandErrors = append(report.CommandErrors, CommandError{Command: name, SessionID: command.sessionID, Err: character.ErrCharacterNotFound})
		return
	}
	if state.Defeated {
		report.CommandErrors = append(report.CommandErrors, CommandError{Command: name, SessionID: command.sessionID, Err: character.ErrCharacterDefeated})
		return
	}

	npc, ok := r.world.Entity(command.intent.NPCEntityID)
	if !ok || npc.Kind != world.EntityNPC {
		report.CommandErrors = append(report.CommandErrors, CommandError{Command: name, SessionID: command.sessionID, Err: ErrNPCNotFound})
		return
	}
	if player.Transform.Position.Layer != npc.Transform.Position.Layer {
		report.CommandErrors = append(report.CommandErrors, CommandError{Command: name, SessionID: command.sessionID, Err: ErrNPCWrongLayer})
		return
	}
	if player.Transform.Position.DistanceSquared(npc.Transform.Position) > npcInteractionRangeMeters*npcInteractionRangeMeters {
		report.CommandErrors = append(report.CommandErrors, CommandError{Command: name, SessionID: command.sessionID, Err: ErrNPCOutOfRange})
		return
	}
	interaction, err := npcInteractionFor(npc)
	if err != nil {
		report.CommandErrors = append(report.CommandErrors, CommandError{Command: name, SessionID: command.sessionID, Err: err})
		return
	}
	r.sessionNPCInteractionPending[s.ID] = interaction
}

func (r *Runtime) replicatePendingNPCInteractions(tick uint64, report *StepReport) {
	if len(r.sessionNPCInteractionPending) == 0 {
		return
	}
	for _, s := range r.sessions.List() {
		interaction, pending := r.sessionNPCInteractionPending[s.ID]
		if !pending {
			continue
		}
		envelope := protocol.Envelope{
			Delivery:   protocol.DeliveryReliableOrdered,
			Sequence:   s.NextOutboundSequence(protocol.DeliveryReliableOrdered),
			ServerTick: tick,
			Message:    interaction,
		}
		report.Metrics.OutboundMessages++
		if err := s.Connection().TrySend(envelope); err != nil {
			if !errors.Is(err, session.ErrBackpressure) {
				report.DeliveryErrors = append(report.DeliveryErrors, DeliveryError{SessionID: s.ID, Delivery: envelope.Delivery, MessageType: interaction.Type(), Err: err})
			}
			continue
		}
		delete(r.sessionNPCInteractionPending, s.ID)
	}
	for id := range r.sessionNPCInteractionPending {
		if _, ok := r.sessions.Get(id); !ok {
			delete(r.sessionNPCInteractionPending, id)
		}
	}
}
