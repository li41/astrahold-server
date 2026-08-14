package worldruntime

import (
	"github.com/li41/astrahold-server/internal/movement"
	"github.com/li41/astrahold-server/internal/session"
	"github.com/li41/astrahold-server/internal/world"
)

type JoinRequest struct {
	Session *session.Session
	Entity world.EntityState
	Speed float32
	Radius float32
	MaxStepHeight float32
}

func (r *Runtime) applyRegister(name string, c registerSessionCommand, report *StepReport) {
	if c.session == nil {
		report.CommandErrors = append(report.CommandErrors, CommandError{Command:name,Err:session.ErrInvalidSession})
		return
	}
	if _, ok := r.world.Entity(c.session.EntityID); !ok {
		report.CommandErrors = append(report.CommandErrors, CommandError{Command:name,SessionID:c.session.ID,Err:ErrSessionEntityNotFound})
		return
	}
	if _, ok := r.characters.State(c.session.EntityID); !ok {
		if err := r.characters.Register(c.session.EntityID); err != nil {
			report.CommandErrors = append(report.CommandErrors, CommandError{Command:name,SessionID:c.session.ID,Err:err})
			return
		}
	}
	r.ensureEntityVitalsRevision(c.session.EntityID)
	if err := r.sessions.Add(c.session); err != nil {
		report.CommandErrors = append(report.CommandErrors, CommandError{Command:name,SessionID:c.session.ID,Err:err})
		return
	}
	r.replication.Register(c.session.ID)
}

func (r *Runtime) applyUnregister(name string, c unregisterSessionCommand, report *StepReport) {
	s, err := r.sessions.Remove(c.id)
	if err != nil {
		report.CommandErrors = append(report.CommandErrors, CommandError{Command:name,SessionID:c.id,Err:err})
		return
	}
	r.replication.Remove(c.id)
	r.removeSessionVitals(c.id)
	_ = s.Connection().Close()
}

func (r *Runtime) applyJoin(name string, request JoinRequest, report *StepReport) {
	if request.Session == nil {
		report.CommandErrors = append(report.CommandErrors, CommandError{Command:name,Err:session.ErrInvalidSession})
		return
	}
	if request.Session.EntityID != request.Entity.ID {
		report.CommandErrors = append(report.CommandErrors, CommandError{Command:name,SessionID:request.Session.ID,Err:ErrJoinEntityMismatch})
		return
	}
	if err := r.world.Spawn(request.Entity, request.Speed, request.Radius, request.MaxStepHeight); err != nil {
		report.CommandErrors = append(report.CommandErrors, CommandError{Command:name,SessionID:request.Session.ID,Err:err})
		return
	}
	if err := r.characters.Register(request.Entity.ID); err != nil {
		r.world.Remove(request.Entity.ID)
		report.CommandErrors = append(report.CommandErrors, CommandError{Command:name,SessionID:request.Session.ID,Err:err})
		return
	}
	r.ensureEntityVitalsRevision(request.Entity.ID)
	if err := r.sessions.Add(request.Session); err != nil {
		r.removeEntityVitals(request.Entity.ID)
		r.characters.Remove(request.Entity.ID)
		r.world.Remove(request.Entity.ID)
		report.CommandErrors = append(report.CommandErrors, CommandError{Command:name,SessionID:request.Session.ID,Err:err})
		return
	}
	r.replication.Register(request.Session.ID)
}

func (r *Runtime) applyLeave(name string, id session.ID, report *StepReport) {
	s, err := r.sessions.Remove(id)
	if err != nil {
		report.CommandErrors = append(report.CommandErrors, CommandError{Command:name,SessionID:id,Err:err})
		return
	}
	r.replication.Remove(id)
	r.removeSessionVitals(id)
	r.removeEntityVitals(s.EntityID)
	r.characters.Remove(s.EntityID)
	r.world.Remove(s.EntityID)
	_ = s.Connection().Close()
}

func (r *Runtime) applyMove(name string, c moveInputCommand, report *StepReport) {
	s, ok := r.sessions.Get(c.sessionID)
	if !ok {
		report.CommandErrors = append(report.CommandErrors, CommandError{Command:name,SessionID:c.sessionID,Err:session.ErrSessionNotFound})
		return
	}
	if err := s.ValidateInputSequence(c.sequence); err != nil {
		report.CommandErrors = append(report.CommandErrors, CommandError{Command:name,SessionID:c.sessionID,Err:err})
		return
	}
	if err := r.world.SetMoveInput(s.EntityID, movement.Input{Direction:world.Vec3{X:c.input.DirectionX,Z:c.input.DirectionZ}}); err != nil {
		report.CommandErrors = append(report.CommandErrors, CommandError{Command:name,SessionID:c.sessionID,Err:err})
		return
	}
	s.MarkProcessedInput(c.sequence)
}
