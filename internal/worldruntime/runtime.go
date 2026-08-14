// Package worldruntime 是 network/session 與單執行緒 world simulation 之間的應用層邊界。
package worldruntime

import (
	"errors"
	"time"

	"github.com/li41/astrahold-server/internal/character"
	"github.com/li41/astrahold-server/internal/combat"
	"github.com/li41/astrahold-server/internal/protocol"
	"github.com/li41/astrahold-server/internal/replication"
	"github.com/li41/astrahold-server/internal/session"
	"github.com/li41/astrahold-server/internal/siege"
	"github.com/li41/astrahold-server/internal/simulation"
	"github.com/li41/astrahold-server/internal/spatial"
	"github.com/li41/astrahold-server/internal/world"
)

var (
	ErrSessionEntityNotFound = errors.New("worldruntime: session entity not found")
	ErrJoinEntityMismatch    = errors.New("worldruntime: join session/entity mismatch")
)

type Config struct {
	CommandQueueCapacity       int
	MaxCommandsPerTick         int
	SnapshotEveryTicks         uint64
	CharacterMaxHP             uint32
	AOIOptions                 spatial.QueryOptions
	ReplicationPolicy          replication.Policy
	MaxSpawnsPerSessionBuild   int
	MaxDespawnsPerSessionBuild int
	CollectMetrics             bool
}

func DefaultConfig() Config {
	return Config{
		CommandQueueCapacity:       4096,
		MaxCommandsPerTick:         2048,
		SnapshotEveryTicks:         2,
		CharacterMaxHP:             1000,
		AOIOptions:                 spatial.QueryOptions{SameLayer: false, MaxHeightDelta: 64},
		ReplicationPolicy:          replication.DefaultPolicy(),
		MaxSpawnsPerSessionBuild:   32,
		MaxDespawnsPerSessionBuild: 64,
	}
}

type CommandError struct {
	Command   string
	SessionID session.ID
	Err       error
}
type ActionRejection struct {
	Action    string
	SessionID session.ID
	Err       error
}
type DeliveryError struct {
	SessionID   session.ID
	Delivery    protocol.Delivery
	MessageType protocol.MessageType
	Err         error
}

type StepMetrics struct {
	CommandQueueDepthBefore       int
	CommandQueueDepthAfter        int
	CommandsDrained               int
	SessionsReplicated            int
	AOIQueries                    int
	AOICandidates                 int
	AOIVisible                    int
	AOISharedCandidateBuilds      int
	AOISharedCandidateReuses      int
	AOIPhysicalCandidateScans     int
	OutboundMessages              int
	SnapshotCandidates            int
	SnapshotTransforms            int
	SnapshotDeferred              int
	SnapshotForcedRefreshes       int
	SnapshotNearTransforms        int
	SnapshotMidTransforms         int
	SnapshotFarTransforms         int
	SpawnCandidates               int
	SpawnSelected                 int
	SpawnDeferred                 int
	DespawnCandidates             int
	DespawnSelected               int
	DespawnDeferred               int
	LifecycleBackpressureStops    int
	CommandDuration               time.Duration
	SimulationDuration            time.Duration
	DynamicReplicationDuration    time.Duration
	ReplicationFrameBuildDuration time.Duration
	AOIDuration                   time.Duration
	ReplicationBuildDuration      time.Duration
	DeliveryDuration              time.Duration
	VitalsReplicationDuration     time.Duration
	TotalDuration                 time.Duration
}

type StepReport struct {
	Tick             uint64
	CommandErrors    []CommandError
	ActionRejections []ActionRejection
	TickErrors       []simulation.TickError
	DeliveryErrors   []DeliveryError
	Metrics          StepMetrics
}

type Runtime struct {
	world                     *simulation.World
	sessions                  *session.Registry
	replication               *replication.Service
	replicationFrameBuilder   *simulation.ReplicationFrameBuilder
	replicationVisibleScratch []int
	characters                *character.Service
	queue                     *commandQueue
	config                    Config
	dynamic                   DynamicWorld
	siege                     *siege.Service
	combat                    *combat.Service
	dynamicRevision           uint64
	sessionDynamicRevision    map[session.ID]uint64
	entityVitalsRevision      map[world.EntityID]uint64
	dirtyVitalsEntities       map[world.EntityID]struct{}
	sessionVitalsRevision     map[session.ID]map[world.EntityID]uint64
	sessionVitalsPending      map[session.ID]map[world.EntityID]struct{}
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
	if config.CharacterMaxHP == 0 {
		config.CharacterMaxHP = 1000
	}
	if config.MaxSpawnsPerSessionBuild <= 0 {
		config.MaxSpawnsPerSessionBuild = 32
	}
	if config.MaxDespawnsPerSessionBuild <= 0 {
		config.MaxDespawnsPerSessionBuild = 64
	}
	characters, err := character.NewService(config.CharacterMaxHP)
	if err != nil {
		panic(err)
	}
	r := &Runtime{
		world:                   w,
		sessions:                session.NewRegistry(),
		replication:             replication.NewService(config.ReplicationPolicy),
		replicationFrameBuilder: simulation.NewReplicationFrameBuilder(),
		characters:              characters,
		queue:                   newCommandQueue(config.CommandQueueCapacity),
		config:                  config,
		sessionDynamicRevision:  make(map[session.ID]uint64),
		entityVitalsRevision:    make(map[world.EntityID]uint64),
		dirtyVitalsEntities:     make(map[world.EntityID]struct{}),
		sessionVitalsRevision:   make(map[session.ID]map[world.EntityID]uint64),
		sessionVitalsPending:    make(map[session.ID]map[world.EntityID]struct{}),
	}
	for _, option := range options {
		if option != nil {
			option(r)
		}
	}
	if r.dynamic != nil {
		r.dynamicRevision = 1
	}
	return r
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
func (r *Runtime) EnqueueLeave(id session.ID) error { return r.queue.tryPush(leaveCommand{id: id}) }
func (r *Runtime) EnqueueMove(id session.ID, sequence uint32, input protocol.ClientMoveInput) error {
	return r.queue.tryPush(moveInputCommand{sessionID: id, sequence: sequence, input: input})
}

// EnqueueTeleport 將 server-authoritative teleport 排入 world owner command queue。
// 它不新增 wire message；位置改變仍透過既有 PositionCorrection / lifecycle replication 對 Client 收斂。
func (r *Runtime) EnqueueTeleport(entityID world.EntityID, position world.Position) error {
	return r.queue.tryPush(teleportCommand{entityID: entityID, position: position})
}
