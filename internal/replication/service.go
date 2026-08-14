// Package replication 將 AOI 世界狀態轉成 client 可消費的 spawn/despawn/snapshot/correction 訊息。
package replication

import (
	"sort"

	"github.com/li41/astrahold-server/internal/protocol"
	"github.com/li41/astrahold-server/internal/session"
	"github.com/li41/astrahold-server/internal/simulation"
	"github.com/li41/astrahold-server/internal/world"
)

type Outbound struct {
	Delivery protocol.Delivery
	Message  protocol.Message
}

type BuildStats struct {
	SnapshotCandidates      int
	SnapshotSelected        int
	SnapshotDeferred        int
	DirtyVisible            int
	ForcedRefreshCandidates int
	NearSelected            int
	MidSelected             int
	FarSelected             int
}

type Batch struct {
	Messages []Outbound
	Stats    BuildStats
}

type sentTransform struct {
	Position world.Position
	Yaw      float32
}

type snapshotCandidate struct {
	entity     world.EntityState
	generation uint64
	tier       Tier
	age        uint64
	cadence    uint64
	dirty      bool
}

type viewState struct {
	// known 只代表 Reliable EntitySpawn 已成功進入該 Session 的 outbound queue。
	// AOI 可見但 Spawn backpressure 的 Entity 不可提前標成 known，否則 Client 會永久漏 Spawn。
	known                   map[world.EntityID]struct{}
	desired                 map[world.EntityID]struct{}
	departed                []world.EntityID
	lastSnapshot            map[world.EntityID]sentTransform // legacy Build compatibility only
	lastDeliveredGeneration map[world.EntityID]uint64
	lastSentBuild           map[world.EntityID]uint64
	candidates              []snapshotCandidate
	buildNumber             uint64
}

type Service struct {
	views  map[session.ID]*viewState
	policy Policy
}

func NewService(policies ...Policy) *Service {
	if len(policies) > 1 {
		panic("replication: expected at most one policy")
	}
	policy := DefaultPolicy()
	if len(policies) == 1 {
		policy = resolvedPolicy(policies[0])
	}
	return &Service{views: make(map[session.ID]*viewState), policy: policy}
}

func newViewState() *viewState {
	return &viewState{
		known:                   make(map[world.EntityID]struct{}),
		desired:                 make(map[world.EntityID]struct{}),
		lastSnapshot:            make(map[world.EntityID]sentTransform),
		lastDeliveredGeneration: make(map[world.EntityID]uint64),
		lastSentBuild:           make(map[world.EntityID]uint64),
	}
}

func (s *Service) Register(id session.ID) {
	if _, ok := s.views[id]; ok {
		return
	}
	s.views[id] = newViewState()
}

func (s *Service) Remove(id session.ID) { delete(s.views, id) }

// Knows 回報該 Session 是否已成功排入 EntitySpawn。
// 這是 Reliable lifecycle knowledge，不等同於「目前 AOI 可見」。
func (s *Service) Knows(sessionID session.ID, entityID world.EntityID) bool {
	state := s.views[sessionID]
	if state == nil {
		return false
	}
	_, ok := state.known[entityID]
	return ok
}

// ConfirmSpawn 只在 EntitySpawn TrySend 成功後呼叫。
// Spawn backpressure 時不呼叫，下一次 Build 會自然重送最新 full spawn state。
func (s *Service) ConfirmSpawn(sessionID session.ID, entityID world.EntityID) {
	state := s.views[sessionID]
	if state == nil {
		return
	}
	state.known[entityID] = struct{}{}
}

// ConfirmDespawn 只在 EntityDespawn TrySend 成功後呼叫。
// 在成功前保留 known，讓下一次 Build 可重試 Despawn。
func (s *Service) ConfirmDespawn(sessionID session.ID, entityID world.EntityID) {
	state := s.views[sessionID]
	if state == nil {
		return
	}
	delete(state.known, entityID)
	delete(state.lastSnapshot, entityID)
	delete(state.lastDeliveredGeneration, entityID)
	delete(state.lastSentBuild, entityID)
}

// Build 保留給既有單元測試與非 frame caller；production Runtime 使用 BuildFrame。
// compatibility path 仍可從 lastSnapshot 推導 generation，但不位於 S3-E.2 hot path。
func (s *Service) Build(sessionID session.ID, selfID world.EntityID, lastProcessedInput uint32, tick uint64, visible []world.EntityState) Batch {
	state := s.ensureView(sessionID)
	ordered := visible
	if !sort.SliceIsSorted(ordered, func(i, j int) bool { return ordered[i].ID < ordered[j].ID }) {
		ordered = append([]world.EntityState(nil), visible...)
		sort.Slice(ordered, func(i, j int) bool { return ordered[i].ID < ordered[j].ID })
	}
	frame := simulation.ReplicationFrame{
		Tick:                 tick,
		Entities:             ordered,
		TransformGenerations: make([]uint64, len(ordered)),
		IndexByID:            make(map[world.EntityID]int, len(ordered)),
	}
	visibleIndices := make([]int, len(ordered))
	for i := range ordered {
		e := ordered[i]
		visibleIndices[i] = i
		frame.IndexByID[e.ID] = i
		generation := state.lastDeliveredGeneration[e.ID]
		previous, hasPrevious := state.lastSnapshot[e.ID]
		if !hasPrevious || previous.Position != e.Transform.Position || previous.Yaw != e.Transform.Yaw {
			generation++
			if generation == 0 {
				generation = 1
			}
		}
		frame.TransformGenerations[i] = generation
	}
	return s.buildFrame(state, sessionID, selfID, lastProcessedInput, &frame, visibleIndices)
}

// BuildFrame 使用 shared immutable frame 與 AOI index view。
// production path 的 dirty 判斷只比較 generation，不重複比較 Position / Yaw。
func (s *Service) BuildFrame(sessionID session.ID, selfID world.EntityID, lastProcessedInput uint32, frame *simulation.ReplicationFrame, visibleIndices []int) Batch {
	state := s.ensureView(sessionID)
	return s.buildFrame(state, sessionID, selfID, lastProcessedInput, frame, visibleIndices)
}

func (s *Service) ensureView(sessionID session.ID) *viewState {
	state := s.views[sessionID]
	if state == nil {
		state = newViewState()
		s.views[sessionID] = state
	}
	return state
}

func (s *Service) buildFrame(state *viewState, sessionID session.ID, selfID world.EntityID, lastProcessedInput uint32, frame *simulation.ReplicationFrame, visibleIndices []int) Batch {
	state.buildNumber++
	tick := frame.Tick

	self, _, hasSelf := frame.Entity(selfID)
	var selfTransform protocol.EntityTransform
	var selfPosition world.Position
	if hasSelf {
		selfPosition = self.Transform.Position
		selfTransform = protocol.EntityTransform{
			EntityID: self.ID,
			Tick:     tick,
			Position: self.Transform.Position,
			Yaw:      self.Transform.Yaw,
		}
	}

	clear(state.desired)
	state.candidates = state.candidates[:0]
	messageCapacity := 4 + (s.policy.MaxTransformsPerBuild+protocol.MaxSnapshotEntitiesPerChunk-1)/protocol.MaxSnapshotEntitiesPerChunk
	batch := Batch{Messages: make([]Outbound, 0, messageCapacity)}

	for _, index := range visibleIndices {
		if index < 0 || index >= len(frame.Entities) {
			continue
		}
		e := frame.Entities[index]
		generation := frame.TransformGenerations[index]
		state.desired[e.ID] = struct{}{}
		tr := protocol.EntityTransform{EntityID: e.ID, Tick: tick, Position: e.Transform.Position, Yaw: e.Transform.Yaw}
		if _, ok := state.known[e.ID]; !ok {
			batch.Messages = append(batch.Messages, Outbound{
				Delivery: protocol.DeliveryReliableOrdered,
				Message:  protocol.EntitySpawn{EntityID: e.ID, Kind: e.Kind, Transform: tr},
			})
			// Spawn 自己已包含 authoritative transform。直到 Reliable Spawn 成功前，
			// 不把這個 Entity 放進 realtime snapshot，也不讓 Vitals 認為 Client 已知。
			continue
		}
		if e.ID == selfID {
			continue
		}

		tier := TierFar
		if hasSelf {
			tier = s.policy.tier(selfPosition, e.Transform.Position)
		}
		lastGeneration, hasDelivered := state.lastDeliveredGeneration[e.ID]
		dirty := !hasDelivered || lastGeneration != generation
		if dirty {
			batch.Stats.DirtyVisible++
		}
		lastBuild := state.lastSentBuild[e.ID]
		age := state.buildNumber - lastBuild
		cadence := s.policy.cadence(tier)
		forced := hasDelivered && age >= s.policy.refresh(tier)
		dueDirty := dirty && (!hasDelivered || age >= cadence)
		if !dueDirty && !forced {
			continue
		}
		if forced {
			batch.Stats.ForcedRefreshCandidates++
		}
		state.candidates = append(state.candidates, snapshotCandidate{
			entity:     e,
			generation: generation,
			tier:       tier,
			age:        age,
			cadence:    cadence,
			dirty:      dirty,
		})
	}

	state.departed = state.departed[:0]
	for id := range state.known {
		if _, ok := state.desired[id]; !ok {
			state.departed = append(state.departed, id)
		}
	}
	sort.Slice(state.departed, func(i, j int) bool { return state.departed[i] < state.departed[j] })
	for _, id := range state.departed {
		batch.Messages = append(batch.Messages, Outbound{
			Delivery: protocol.DeliveryReliableOrdered,
			Message:  protocol.EntityDespawn{EntityID: id},
		})
	}

	batch.Stats.SnapshotCandidates = len(state.candidates)
	budgetExceeded := len(state.candidates) > s.policy.MaxTransformsPerBuild
	if budgetExceeded {
		sort.Slice(state.candidates, func(i, j int) bool {
			a, b := state.candidates[i], state.candidates[j]
			// age/cadence 越大代表相對於自己的 LOD cadence 越 overdue。
			// 只有真的超過 budget 時才付 ranking 成本；normal path 保留 frame EntityID 穩定順序。
			left := a.age * b.cadence
			right := b.age * a.cadence
			if left != right {
				return left > right
			}
			if a.dirty != b.dirty {
				return a.dirty
			}
			if a.tier != b.tier {
				return a.tier < b.tier
			}
			if a.age != b.age {
				return a.age > b.age
			}
			return a.entity.ID < b.entity.ID
		})
	}

	selectedCount := len(state.candidates)
	if selectedCount > s.policy.MaxTransformsPerBuild {
		selectedCount = s.policy.MaxTransformsPerBuild
	}
	batch.Stats.SnapshotSelected = selectedCount
	batch.Stats.SnapshotDeferred = len(state.candidates) - selectedCount
	transforms := make([]protocol.EntityTransform, selectedCount)
	for i := 0; i < selectedCount; i++ {
		candidate := state.candidates[i]
		e := candidate.entity
		transforms[i] = protocol.EntityTransform{EntityID: e.ID, Tick: tick, Position: e.Transform.Position, Yaw: e.Transform.Yaw}
		state.lastSnapshot[e.ID] = sentTransform{Position: e.Transform.Position, Yaw: e.Transform.Yaw}
		state.lastDeliveredGeneration[e.ID] = candidate.generation
		state.lastSentBuild[e.ID] = state.buildNumber
		switch candidate.tier {
		case TierNear:
			batch.Stats.NearSelected++
		case TierMid:
			batch.Stats.MidSelected++
		case TierFar:
			batch.Stats.FarSelected++
		}
	}
	if budgetExceeded {
		sort.Slice(transforms, func(i, j int) bool { return transforms[i].EntityID < transforms[j].EntityID })
	}

	chunkCount := (len(transforms) + protocol.MaxSnapshotEntitiesPerChunk - 1) / protocol.MaxSnapshotEntitiesPerChunk
	if chunkCount == 0 {
		chunkCount = 1
	}
	for chunk := 0; chunk < chunkCount; chunk++ {
		start := chunk * protocol.MaxSnapshotEntitiesPerChunk
		end := start + protocol.MaxSnapshotEntitiesPerChunk
		if end > len(transforms) {
			end = len(transforms)
		}
		batch.Messages = append(batch.Messages, Outbound{
			Delivery: protocol.DeliveryRealtimeSequenced,
			Message: protocol.WorldSnapshot{
				Tick:       tick,
				ChunkIndex: uint16(chunk),
				ChunkCount: uint16(chunkCount),
				Entities:   transforms[start:end],
			},
		})
	}

	if hasSelf {
		batch.Messages = append(batch.Messages, Outbound{
			Delivery: protocol.DeliveryRealtimeSequenced,
			Message: protocol.PositionCorrection{
				Tick:                       tick,
				EntityID:                   selfTransform.EntityID,
				Position:                   selfTransform.Position,
				Yaw:                        selfTransform.Yaw,
				LastProcessedInputSequence: lastProcessedInput,
			},
		})
	}

	return batch
}
