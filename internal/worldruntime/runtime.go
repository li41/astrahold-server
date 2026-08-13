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

var (
	ErrSessionEntityNotFound = errors.New("worldruntime: session entity not found")
	ErrJoinEntityMismatch    = errors.New("worldruntime: join session/entity mismatch")
)

type Config struct {
	CommandQueueCapacity int
	MaxCommandsPerTick   int
	SnapshotEveryTicks   uint64
	AOIOptions           spatial.QueryOptions
}

func DefaultConfig() Config {
	return Config{
		CommandQueueCapacity: 4096,
		MaxCommandsPerTick:   2048,
		SnapshotEveryTicks:   2,
		AOIOptions: spatial.QueryOptions{
			SameLayer:      false,
			MaxHeightDelta: 64,
		},
	}
}

// JoinRequest 讓 Network/Auth 等外層只提交「加入世界」意圖。
// 真正的 Entity Spawn 與 Session Registry mutation 仍在 simulation owner goroutine 內完成。
type JoinRequest struct {
	Session       *session.Session
	Entity        world.EntityState
	Speed         float32
	Radius        float32
	MaxStepHeight float32
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

	dynamic                DynamicWorld
	dynamicRevision        uint64
	sessionDynamicRevision map[session.ID]uint64
}

func New(w *simulation.World, config Config, options ...Option) *Runtime {
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
	runtime := &Runtime{
		world:                  w,
		sessions:               session.NewRegistry(),
		replication:            replication.NewService(),
		queue:                  newCommandQueue(config.CommandQueueCapacity),
		config:                 config,
		sessionDynamicRevision: make(map[session.ID]uint64),
	}
	for _, option := range options {
		if option != nil {
			option(runtime)
		}
	}
	if runtime.dynamic != nil {
		// Revision 1 表示 bake 初始 dynamic state；新 Session 必須先收到完整 snapshot。
		runtime.dynamicRevision = 1
	}
	return runtime
}

func (r *Runtime) EnqueueRegister(s *session.Session) error {
	return r.queue.tryPush(registerSessionCommand{session: s})
}
func (r *Runtime) EnqueueUnregister(id session.ID) error {
	return r.queue.tryPush(unregisterSessionCommand{id: id})
}
func (r *Runtime) EnqueueJoin(request JoinRequest) error {
	return r.queue.tryPush(joinCommand{request: request})
}
func (r *Runtime) EnqueueLeave(id session.ID) error {
	return r.queue.tryPush(leaveCommand{id: id})
}
func (r *Runtime) EnqueueMove(id session.ID, sequence uint32, input protocol.ClientMoveInput) error {
	return r.queue.tryPush(moveInputCommand{sessionID: id, sequence: sequence, input: input})
}

func (r *Runtime) Step(tick uint64, delta time.Duration) StepReport {
	report := StepReport{Tick: tick}

	for _, cmd := range r.queue.drain(r.config.MaxCommandsPerTick) {
		switch c := cmd.(type) {
		case registerSessionCommand:
			r.applyRegister(cmd.name(), c, &report)
		case unregisterSessionCommand:
			r.applyUnregister(cmd.name(), c, &report)
		case joinCommand:
			r.applyJoin(cmd.name(), c.request, &report)
		case leaveCommand:
			r.applyLeave(cmd.name(), c.id, &report)
		case moveInputCommand:
			r.applyMove(cmd.name(), c, &report)
		case setBlockerCommand:
			r.applySetBlocker(cmd.name(), c, &report)
		}
	}

	report.TickErrors = r.world.Tick(float32(delta.Seconds()))

	// Dynamic World 是低頻 Reliable state，不能被 SnapshotEveryTicks 節流。
	r.replicateDynamicState(tick, &report)

	if tick%r.config.SnapshotEveryTicks != 0 {
		return report
	}

	for _, s := range r.sessions.List() {
		self, ok := r.world.Entity(s.EntityID)
		if !ok {
			report.CommandErrors = append(report.CommandErrors, CommandError{
				Command: "replicate", SessionID: s.ID, Err: ErrSessionEntityNotFound,
			})
			continue
		}

		visible := r.world.QueryAOI(self.Transform.Position, s.AOIRadius, r.config.AOIOptions)
		batch := r.replication.Build(s.ID, s.EntityID, s.LastProcessedInputSequence(), tick, visible)
		for _, out := range batch.Messages {
			envelope := protocol.Envelope{
				Delivery: out.Delivery,
				Sequence: s.NextOutboundSequence(out.Delivery),
				ServerTick: tick,
				Message: out.Message,
			}
			if err := s.Connection().TrySend(envelope); err != nil {
				report.DeliveryErrors = append(report.DeliveryErrors, DeliveryError{
					SessionID: s.ID, Delivery: out.Delivery, MessageType: out.Message.Type(), Err: err,
				})
			}
		}
	}
	return report
}

func (r *Runtime) applyRegister(name string, c registerSessionCommand, report *StepReport) {
	if c.session == nil {
		report.CommandErrors = append(report.CommandErrors, CommandError{Command: name, Err: session.ErrInvalidSession})
		return
	}
	if _, ok := r.world.Entity(c.session.EntityID); !ok {
		report.CommandErrors = append(report.CommandErrors, CommandError{Command: name, SessionID: c.session.ID, Err: ErrSessionEntityNotFound})
		return
	}
	if err := r.sessions.Add(c.session); err != nil {
		report.CommandErrors = append(report.CommandErrors, CommandError{Command: name, SessionID: c.session.ID, Err: err})
		return
	}
	r.replication.Register(c.session.ID)
}

func (r *Runtime) applyUnregister(name string, c unregisterSessionCommand, report *StepReport) {
	s, err := r.sessions.Remove(c.id)
	if err != nil {
		report.CommandErrors = append(report.CommandErrors, CommandError{Command: name, SessionID: c.id, Err: err})
		return
	}
	r.replication.Remove(c.id)
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
	if err := r.world.Spawn(request.Entity, request.Speed, request.Radius, request.MaxStepHeight); err != nil {
		report.CommandErrors = append(report.CommandErrors, CommandError{Command: name, SessionID: request.Session.ID, Err: err})
		return
	}
	if err := r.sessions.Add(request.Session); err != nil {
		r.world.Remove(request.Entity.ID)
		report.CommandErrors = append(report.CommandErrors, CommandError{Command: name, SessionID: request.Session.ID, Err: err})
		return
	}
	r.replication.Register(request.Session.ID)
}

func (r *Runtime) applyLeave(name string, id session.ID, report *StepReport) {
	s, err := r.sessions.Remove(id)
	if err != nil {
		report.CommandErrors = append(report.CommandErrors, CommandError{Command: name, SessionID: id, Err: err})
		return
	}
	r.replication.Remove(id)
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
	if err := r.world.SetMoveInput(s.EntityID, movement.Input{Direction: world.Vec3{X: c.input.DirectionX, Z: c.input.DirectionZ}}); err != nil {
		report.CommandErrors = append(report.CommandErrors, CommandError{Command: name, SessionID: c.sessionID, Err: err})
		return
	}
	s.MarkProcessedInput(c.sequence)
}
