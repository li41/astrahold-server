package worldruntime

import (
	"errors"
	"strings"

	"github.com/li41/astrahold-server/internal/ammunition"
	"github.com/li41/astrahold-server/internal/character"
	"github.com/li41/astrahold-server/internal/protocol"
	"github.com/li41/astrahold-server/internal/session"
)

var ErrInvalidAmmunitionIntent = errors.New("worldruntime: invalid ammunition intent")

func validAmmunitionIntent(intent protocol.ClientAmmunitionCommand) bool {
	itemID := strings.TrimSpace(intent.ItemArchetypeID)
	return intent.Operation == protocol.AmmunitionOperationSelect && itemID != "" && itemID == intent.ItemArchetypeID
}

func (r *Runtime) EnqueueAmmunitionCommand(id session.ID, sequence uint32, intent protocol.ClientAmmunitionCommand) error {
	if id == 0 || sequence == 0 || !validAmmunitionIntent(intent) {
		return ErrInvalidAmmunitionIntent
	}
	payload := intent
	return r.queue.tryPush(useActionCommand{sessionID: id, sequence: sequence, ammunition: &payload})
}

func (r *Runtime) EnqueueFencedAmmunitionCommand(ownership SessionOwnershipFence, sequence uint32, intent protocol.ClientAmmunitionCommand) error {
	if !ownership.Valid() || sequence == 0 || !validAmmunitionIntent(intent) {
		return ErrInvalidAmmunitionIntent
	}
	payload := intent
	return r.queue.tryPush(useActionCommand{
		sessionID: ownership.SessionID,
		sequence: sequence,
		ammunition: &payload,
		ownership: ownership,
	})
}

// ammunitionStateForSession normalizes a stale preference back to automatic mode as soon as an
// authoritative inventory snapshot is built. Empty means automatic wood -> silver ordering.
func (r *Runtime) ammunitionStateForSession(s *session.Session) protocol.AmmunitionState {
	if s == nil {
		return protocol.AmmunitionState{}
	}
	selected := r.sessionAmmunitionSelection[s.ID]
	if selected == "" {
		return protocol.AmmunitionState{}
	}
	definition, ok := ammunition.Resolve(selected)
	inv := r.inventories[s.CharacterIdentity.ID]
	if !ok || inv == nil || inv.Quantity(definition.ItemArchetypeID) == 0 {
		delete(r.sessionAmmunitionSelection, s.ID)
		return protocol.AmmunitionState{}
	}
	return protocol.AmmunitionState{SelectedItemArchetypeID: definition.ItemArchetypeID}
}

func (r *Runtime) queueAmmunitionFeedback(sessionID session.ID, result protocol.AmmunitionResult, state protocol.AmmunitionState) {
	r.pendingResourceMessages[sessionID] = append(r.pendingResourceMessages[sessionID], result, state)
}

func (r *Runtime) rejectAmmunitionCommand(s *session.Session, command useActionCommand, reason protocol.AmmunitionRejectionReason) {
	intent := *command.ammunition
	r.queueAmmunitionFeedback(command.sessionID, protocol.AmmunitionResult{
		ClientActionSequence: command.sequence,
		Operation: intent.Operation,
		Outcome: protocol.AmmunitionOutcomeRejected,
		Reason: reason,
		ItemArchetypeID: intent.ItemArchetypeID,
	}, r.ammunitionStateForSession(s))
}

func (r *Runtime) applyAmmunitionCommand(name string, command useActionCommand, report *StepReport) {
	if command.ammunition == nil {
		report.CommandErrors = append(report.CommandErrors, CommandError{Command: name, SessionID: command.sessionID, Err: ErrInvalidAmmunitionIntent})
		return
	}
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
	s.MarkProcessedAction(command.sequence)

	state, ok := r.characters.State(s.EntityID)
	if !ok {
		report.CommandErrors = append(report.CommandErrors, CommandError{Command: name, SessionID: command.sessionID, Err: character.ErrCharacterNotFound})
		return
	}
	if state.Defeated {
		r.rejectAmmunitionCommand(s, command, protocol.AmmunitionRejectionDefeated)
		return
	}
	intent := *command.ammunition
	definition, ok := ammunition.Resolve(intent.ItemArchetypeID)
	if !ok {
		r.rejectAmmunitionCommand(s, command, protocol.AmmunitionRejectionInvalidAmmunition)
		return
	}
	inv := r.inventories[s.CharacterIdentity.ID]
	if inv == nil {
		r.rejectAmmunitionCommand(s, command, protocol.AmmunitionRejectionServerRejected)
		return
	}
	if inv.Quantity(definition.ItemArchetypeID) == 0 {
		r.rejectAmmunitionCommand(s, command, protocol.AmmunitionRejectionInsufficientInventory)
		return
	}

	r.sessionAmmunitionSelection[s.ID] = definition.ItemArchetypeID
	stateMessage := r.ammunitionStateForSession(s)
	r.queueAmmunitionFeedback(command.sessionID, protocol.AmmunitionResult{
		ClientActionSequence: command.sequence,
		Operation: intent.Operation,
		Outcome: protocol.AmmunitionOutcomeSelected,
		ItemArchetypeID: definition.ItemArchetypeID,
	}, stateMessage)
}
