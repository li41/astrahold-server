// Package worldruntime 是 network/session 與單執行緒 world simulation 之間的應用層邊界。
package worldruntime

import (
	"errors"
	"time"

	"github.com/li41/astrahold-server/internal/movement"
	"github.com/li41/astrahold-server/internal/protocol"
	"github.com/li41/astrahold-server/internal/replication"
	"github.com/li41/astrahold-server/internal/session"
	"github.com/li41/astrahold-server/internal/simulation"
	"github.com/li41/astrahold-server/internal/spatial"
	"github.com/li41/astrahold-server/internal/world"
)

var ErrSessionEntityNotFound = errors.New("worldruntime: session entity not found")

type Config struct {
	CommandQueueCapacity int
	MaxCommandsPerTick   int
	SnapshotEveryTicks   uint64
	AOIOptions           spatial.QueryOptions
}

func DefaultConfig() Config {
	return Config{CommandQueueCapacity: 4096, MaxCommandsPerTick: 2048, SnapshotEveryTicks: 2, AOIOptions: spatial.QueryOptions{SameLayer: false, MaxHeightDelta: 64}}
}

type CommandError struct {
	Command   string
	SessionID session.ID
	Err       error
}
type DeliveryError struct {
	SessionID   session.ID
	Delivery    protocol.Delivery
	MessageType protocol.MessageType
	Err         error
}
type StepReport struct {
	Tick           uint64
	CommandErrors  []CommandError
	TickErrors     []simulation.TickError
	DeliveryErrors []DeliveryError
}

type Runtime struct {
	world       *simulation.World
	sessions    *session.Registry
	replication *replication.Service
	queue       *commandQueue
	config      Config
}

func New(w *simulation.World, config Config) *Runtime {
	if w == nil {
		panic("worldruntime: world is required")
	}
	if config.CommandQueueCapacity <= 0 {
		config.CommandQueueCapacity = 4096
	}
	if config.MaxCommandsPerTick <= 0 {
		config.MaxCommandsPerTick = 2048
	}
	if config.SnapshotEveryTicks == 0 {
		config.SnapshotEveryTicks = 1
	}
	return &Runtime{world: w, sessions: session.NewRegistry(), replication: replication.NewService(), queue: newCommandQueue(config.CommandQueueCapacity), config: config}
}
func (r *Runtime) EnqueueRegister(s *session.Session) error {
	return r.queue.tryPush(registerSessionCommand{session: s})
}
func (r *Runtime) EnqueueUnregister(id session.ID) error {
	return r.queue.tryPush(unregisterSessionCommand{id: id})
}
func (r *Runtime) EnqueueMove(id session.ID, input protocol.ClientMoveInput) error {
	return r.queue.tryPush(moveInputCommand{sessionID: id, input: input})
}

func (r *Runtime) Step(tick uint64, delta time.Duration) StepReport {
	report := StepReport{Tick: tick}
	for _, cmd := range r.queue.drain(r.config.MaxCommandsPerTick) {
		switch c := cmd.(type) {
		case registerSessionCommand:
			if c.session == nil {
				report.CommandErrors = append(report.CommandErrors, CommandError{Command: cmd.name(), Err: session.ErrInvalidSession})
				continue
			}
			if _, ok := r.world.Entity(c.session.EntityID); !ok {
				report.CommandErrors = append(report.CommandErrors, CommandError{Command: cmd.name(), SessionID: c.session.ID, Err: ErrSessionEntityNotFound})
				continue
			}
			if err := r.sessions.Add(c.session); err != nil {
				report.CommandErrors = append(report.CommandErrors, CommandError{Command: cmd.name(), SessionID: c.session.ID, Err: err})
				continue
			}
			r.replication.Register(c.session.ID)
		case unregisterSessionCommand:
			s, err := r.sessions.Remove(c.id)
			if err != nil {
				report.CommandErrors = append(report.CommandErrors, CommandError{Command: cmd.name(), SessionID: c.id, Err: err})
				continue
			}
			r.replication.Remove(c.id)
			_ = s.Connection().Close()
		case moveInputCommand:
			s, ok := r.sessions.Get(c.sessionID)
			if !ok {
				report.CommandErrors = append(report.CommandErrors, CommandError{Command: cmd.name(), SessionID: c.sessionID, Err: session.ErrSessionNotFound})
				continue
			}
			if err := s.ValidateInputSequence(c.input.Sequence); err != nil {
				report.CommandErrors = append(report.CommandErrors, CommandError{Command: cmd.name(), SessionID: c.sessionID, Err: err})
				continue
			}
			err := r.world.SetMoveInput(s.EntityID, movement.Input{Direction: world.Vec3{X: c.input.DirectionX, Z: c.input.DirectionZ}})
			if err != nil {
				report.CommandErrors = append(report.CommandErrors, CommandError{Command: cmd.name(), SessionID: c.sessionID, Err: err})
				continue
			}
			s.MarkProcessedInput(c.input.Sequence)
		}
	}
	report.TickErrors = r.world.Tick(float32(delta.Seconds()))
	if tick%r.config.SnapshotEveryTicks != 0 {
		return report
	}
	for _, s := range r.sessions.List() {
		self, ok := r.world.Entity(s.EntityID)
		if !ok {
			report.CommandErrors = append(report.CommandErrors, CommandError{Command: "replicate", SessionID: s.ID, Err: ErrSessionEntityNotFound})
			continue
		}
		visible := r.world.QueryAOI(self.Transform.Position, s.AOIRadius, r.config.AOIOptions)
		batch := r.replication.Build(s.ID, s.EntityID, s.LastProcessedInputSequence(), tick, visible)
		for _, out := range batch.Messages {
			envelope := protocol.Envelope{Delivery: out.Delivery, Sequence: s.NextOutboundSequence(out.Delivery), ServerTick: tick, Message: out.Message}
			if err := s.Connection().TrySend(envelope); err != nil {
				report.DeliveryErrors = append(report.DeliveryErrors, DeliveryError{SessionID: s.ID, Delivery: out.Delivery, MessageType: out.Message.Type(), Err: err})
			}
		}
	}
	return report
}
