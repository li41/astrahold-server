package worldruntime

import (
	"errors"

	"github.com/li41/astrahold-server/internal/classid"
	"github.com/li41/astrahold-server/internal/session"
)

// ErrFixedClassSelectionRetired is returned by every former initial-class mutation seam.
// Protocol v27 may still decode historical class-selection messages, but current gameplay is
// classless and no Server caller may turn them into mutable character profession truth.
var ErrFixedClassSelectionRetired = errors.New("worldruntime: fixed class selection is retired")

// initialClassAssignmentCommand remains only as a fail-closed command tombstone until the next
// breaking Protocol cleanup removes the historical v27 message family entirely. No public enqueue
// path creates it.
type initialClassAssignmentCommand struct {
	sessionID session.ID
}

func (initialClassAssignmentCommand) name() string { return "retired_initial_class_assignment" }

// EnqueueFencedInitialClassAssignment is retained temporarily as a source-compatibility tombstone.
// It never queues a command and never mutates gameplay or persistence.
func (r *Runtime) EnqueueFencedInitialClassAssignment(_ SessionOwnershipFence, _ classid.ID) error {
	return ErrFixedClassSelectionRetired
}

func (r *Runtime) applyInitialClassAssignment(name string, command initialClassAssignmentCommand, report *StepReport) {
	if report == nil {
		return
	}
	report.CommandErrors = append(report.CommandErrors, CommandError{
		Command: name, SessionID: command.sessionID, Err: ErrFixedClassSelectionRetired,
	})
}

// applyCharacterStateSaveCompletions is kept only while characterstate's historical completion lane
// still exists. Current worldruntime has no producer for completion-requested character saves.
// Unexpected completions fail closed instead of resurrecting the retired profession transaction.
func (r *Runtime) applyCharacterStateSaveCompletions(report *StepReport) {
	if r.characterStateOutbox == nil || r.characterStateOutbox.CompletionDepth() == 0 || report == nil {
		return
	}
	report.CommandErrors = append(report.CommandErrors, CommandError{
		Command: "retired_character_state_completion", Err: ErrFixedClassSelectionRetired,
	})
}
