package worldruntime

import (
	"github.com/li41/astrahold-server/internal/character"
	"github.com/li41/astrahold-server/internal/movement"
	"github.com/li41/astrahold-server/internal/session"
	"github.com/li41/astrahold-server/internal/world"
)

type JoinRequest struct {
	Session       *session.Session
	Entity        world.EntityState
	Speed         float32
	Radius        float32
	MaxStepHeight float32
}

func (r *Runtime) applyRegister(name string, c registerSessionCommand, report *StepReport) {
	if c.session == nil {
		report.CommandErrors = append(report.CommandErrors, CommandError{Command: name, Err: session.ErrInvalidSession})
		return
	}
	if err := r.characterIdentities.validateSession(c.session); err != nil {
		report.CommandErrors = append(report.CommandErrors, CommandError{Command: name, SessionID: c.session.ID, Err: err})
		return
	}
	if _, ok := r.world.Entity(c.session.EntityID); !ok {
		report.CommandErrors = append(report.CommandErrors, CommandError{Command: name, SessionID: c.session.ID, Err: ErrSessionEntityNotFound})
		return
	}
	if _, ok := r.characters.State(c.session.EntityID); !ok {
		if err := r.characters.Register(c.session.EntityID); err != nil {
			report.CommandErrors = append(report.CommandErrors, CommandError{Command: name, SessionID: c.session.ID, Err: err})
			return
		}
	}
	r.ensureEntityVitalsRevision(c.session.EntityID)
	if err := r.sessions.Add(c.session); err != nil {
		report.CommandErrors = append(report.CommandErrors, CommandError{Command: name, SessionID: c.session.ID, Err: err})
		return
	}
	r.characterIdentities.bindSession(c.session)
	r.replication.Register(c.session.ID)
}

func (r *Runtime) applyUnregister(name string, c unregisterSessionCommand, report *StepReport) {
	s, err := r.sessions.Remove(c.id)
	if err != nil {
		report.CommandErrors = append(report.CommandErrors, CommandError{Command: name, SessionID: c.id, Err: err})
		return
	}
	r.replication.Remove(c.id)
	r.removeSessionVitals(c.id)
	_ = s.Connection().Close()
}

func (r *Runtime) applyJoin(name string, request JoinRequest, report *StepReport) {
	if request.Session == nil {
		report.CommandErrors = append(report.CommandErrors, CommandError{Command: name, Err: session.ErrInvalidSession})
		return
	}
	if request.Session.EntityID != request.Entity.ID {
		report.CommandErrors = append(report.CommandErrors, CommandError{Command: name, SessionID: request.Session.ID, Err: ErrJoinEntityMismatch})
		return
	}
	if err := r.characterIdentities.validateSession(request.Session); err != nil {
		report.CommandErrors = append(report.CommandErrors, CommandError{Command: name, SessionID: request.Session.ID, Err: err})
		return
	}
	if err := r.world.Spawn(request.Entity, request.Speed, request.Radius, request.MaxStepHeight); err != nil {
		report.CommandErrors = append(report.CommandErrors, CommandError{Command: name, SessionID: request.Session.ID, Err: err})
		return
	}
	if err := r.characters.Register(request.Entity.ID); err != nil {
		r.world.Remove(request.Entity.ID)
		report.CommandErrors = append(report.CommandErrors, CommandError{Command: name, SessionID: request.Session.ID, Err: err})
		return
	}
	r.ensureEntityVitalsRevision(request.Entity.ID)
	if err := r.sessions.Add(request.Session); err != nil {
		r.removeEntityVitals(request.Entity.ID)
		r.characters.Remove(request.Entity.ID)
		r.world.Remove(request.Entity.ID)
		report.CommandErrors = append(report.CommandErrors, CommandError{Command: name, SessionID: request.Session.ID, Err: err})
		return
	}
	r.characterIdentities.bindSession(request.Session)
	r.replication.Register(request.Session.ID)
}

func (r *Runtime) applyLeave(name string, id session.ID, report *StepReport) {
	s, err := r.sessions.Remove(id)
	if err != nil {
		report.CommandErrors = append(report.CommandErrors, CommandError{Command: name, SessionID: id, Err: err})
		return
	}
	r.replication.Remove(id)
	r.removeSessionVitals(id)
	r.removeEntityVitals(s.EntityID)
	r.clearReviveProtection(s.EntityID)
	r.clearDeathOutcomeState(s.EntityID)
	if r.respawnPolicy != nil {
		r.respawnPolicy.Remove(s.EntityID)
	}
	r.characters.Remove(s.EntityID)
	r.characterIdentities.removeEntity(s.EntityID)
	r.world.Remove(s.EntityID)
	_ = s.Connection().Close()
}

func (r *Runtime) applyMove(name string, c moveInputCommand, report *StepReport) {
	s, ok := r.sessions.Get(c.sessionID)
	if !ok {
		report.CommandErrors = append(report.CommandErrors, CommandError{Command: name, SessionID: c.sessionID, Err: session.ErrSessionNotFound})
		return
	}
	if err := s.ValidateInputSequence(c.sequence); err != nil {
		report.CommandErrors = append(report.CommandErrors, CommandError{Command: name, SessionID: c.sessionID, Err: err})
		return
	}
	state, ok := r.characters.State(s.EntityID)
	if !ok {
		report.CommandErrors = append(report.CommandErrors, CommandError{Command: name, SessionID: c.sessionID, Err: character.ErrCharacterNotFound})
		return
	}
	if state.Defeated {
		// Defeated 是正常 gameplay state，不是 command fault。仍消耗 input sequence，
		// 但 authoritative movement input 固定歸零，避免倒地期間累積方向並在未來 revive 後重播。
		if err := r.world.SetMoveInput(s.EntityID, movement.Input{}); err != nil {
			report.CommandErrors = append(report.CommandErrors, CommandError{Command: name, SessionID: c.sessionID, Err: err})
			return
		}
		s.MarkProcessedInput(c.sequence)
		return
	}
	if err := r.world.SetMoveInput(s.EntityID, movement.Input{Direction: world.Vec3{X: c.input.DirectionX, Z: c.input.DirectionZ}}); err != nil {
		report.CommandErrors = append(report.CommandErrors, CommandError{Command: name, SessionID: c.sessionID, Err: err})
		return
	}
	s.MarkProcessedInput(c.sequence)
}

func (r *Runtime) applyTeleport(name string, c teleportCommand, report *StepReport) {
	if err := r.world.Teleport(c.entityID, c.position); err != nil {
		report.CommandErrors = append(report.CommandErrors, CommandError{Command: name, Err: err})
	}
}

func (r *Runtime) applyTeleportBatch(name string, c teleportBatchCommand, report *StepReport) {
	for _, request := range c.requests {
		if err := r.world.Teleport(request.EntityID, request.Position); err != nil {
			report.CommandErrors = append(report.CommandErrors, CommandError{Command: name, Err: err})
		}
	}
}
