package worldruntime

import (
	"errors"

	"github.com/li41/astrahold-server/internal/character"
	"github.com/li41/astrahold-server/internal/protocol"
	"github.com/li41/astrahold-server/internal/session"
)

var (
	ErrInvalidRespawnIntent = errors.New("worldruntime: invalid respawn intent")
	ErrRespawnNotScheduled  = errors.New("worldruntime: respawn not scheduled")
)

// EnqueueRespawnRequest accepts only the source session and reliable sequence. The client cannot
// choose entity, destination or timing; those remain bound to the authoritative death outcome.
func (r *Runtime) EnqueueRespawnRequest(id session.ID, sequence uint32, _ protocol.ClientRespawnRequest) error {
	if id == 0 || sequence == 0 {
		return ErrInvalidRespawnIntent
	}
	intent := protocol.ClientRespawnRequest{}
	return r.queue.tryPush(useActionCommand{sessionID: id, sequence: sequence, respawn: &intent})
}

func (r *Runtime) EnqueueFencedRespawnRequest(ownership SessionOwnershipFence, sequence uint32, _ protocol.ClientRespawnRequest) error {
	if !ownership.Valid() || sequence == 0 {
		return ErrInvalidRespawnIntent
	}
	intent := protocol.ClientRespawnRequest{}
	return r.queue.tryPush(useActionCommand{
		sessionID: ownership.SessionID,
		sequence:  sequence,
		respawn:   &intent,
		ownership: ownership,
	})
}

// applyRespawnRequest records player consent to restart. It does not revive directly: the normal
// respawn-policy due phase remains the only timed policy transition, so an early click cannot bypass
// PvE/PvP/Siege delay or choose a different destination.
func (r *Runtime) applyRespawnRequest(name string, command useActionCommand, _ uint64, report *StepReport) {
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
	// Reliable sequence is consumed once the intent reaches authoritative gameplay handling. This
	// prevents a pre-death/replayed click from becoming valid after a later death or respawn.
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
	if !state.Defeated {
		report.CommandErrors = append(report.CommandErrors, CommandError{Command: name, SessionID: command.sessionID, Err: character.ErrCharacterNotDefeated})
		return
	}
	if r.respawnPolicy == nil {
		report.CommandErrors = append(report.CommandErrors, CommandError{Command: name, SessionID: command.sessionID, Err: ErrRespawnPolicyUnavailable})
		return
	}
	if _, ok := r.respawnPolicy.Pending(s.EntityID); !ok {
		report.CommandErrors = append(report.CommandErrors, CommandError{Command: name, SessionID: command.sessionID, Err: ErrRespawnNotScheduled})
		return
	}

	r.respawnRequested[s.EntityID] = struct{}{}
}
