// Package replication 將 AOI 世界狀態轉成 client 可消費的 spawn/despawn/snapshot/correction 訊息。
package replication

import (
	"sort"

	"github.com/li41/astrahold-server/internal/protocol"
	"github.com/li41/astrahold-server/internal/session"
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
	entity   world.EntityState
	tier     Tier
	age      uint64
	cadence  uint64
	dirty    bool
	forced   bool
}

type viewState struct {
	known         map[world.EntityID]struct{}
	scratch       map[world.EntityID]struct{}
	departed      []world.EntityID
	lastSnapshot  map[world.EntityID]sentTransform
	lastSentBuild map[world.EntityID]uint64
	candidates    []snapshotCandidate
	buildNumber   uint64
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
		known:         make(map[world.EntityID]struct{}),
		scratch:       make(map[world.EntityID]struct{}),
		lastSnapshot:  make(map[world.EntityID]sentTransform),
		lastSentBuild: make(map[world.EntityID]uint64),
	}
}

func (s *Service) Register(id session.ID) {
	if _, ok := s.views[id]; ok {
		return
	}
	s.views[id] = newViewState()
}

func (s *Service) Remove(id session.ID) { delete(s.views, id) }

// Knows 回報該 Session 是否已經收過 EntitySpawn，供低頻 Reliable entity state 做 AOI fan-out。
func (s *Service) Knows(sessionID session.ID, entityID world.EntityID) bool {
	state := s.views[sessionID]
	if state == nil {
		return false
	}
	_, ok := state.known[entityID]
	return ok
}

func (s *Service) Build(sessionID session.ID, selfID world.EntityID, lastProcessedInput uint32, tick uint64, visible []world.EntityState) Batch {
	state := s.views[sessionID]
	if state == nil {
		state = newViewState()
		s.views[sessionID] = state
	}
	state.buildNumber++

	ordered := visible
	if !sort.SliceIsSorted(ordered, func(i, j int) bool { return ordered[i].ID < ordered[j].ID }) {
		ordered = append([]world.EntityState(nil), visible...)
		sort.Slice(ordered, func(i, j int) bool { return ordered[i].ID < ordered[j].ID })
	}

	var selfTransform protocol.EntityTransform
	var selfPosition world.Position
	hasSelf := false
	for i := range ordered {
		if ordered[i].ID != selfID {
			continue
		}
		selfPosition = ordered[i].Transform.Position
		selfTransform = protocol.EntityTransform{
			EntityID: ordered[i].ID,
			Tick:     tick,
			Position: ordered[i].Transform.Position,
			Yaw:      ordered[i].Transform.Yaw,
		}
		hasSelf = true
		break
	}

	clear(state.scratch)
	state.candidates = state.candidates[:0]
	messageCapacity := 4 + (s.policy.MaxTransformsPerBuild+protocol.MaxSnapshotEntitiesPerChunk-1)/protocol.MaxSnapshotEntitiesPerChunk
	batch := Batch{Messages: make([]Outbound, 0, messageCapacity)}

	for i := range ordered {
		e := ordered[i]
		state.scratch[e.ID] = struct{}{}
		tr := protocol.EntityTransform{EntityID: e.ID, Tick: tick, Position: e.Transform.Position, Yaw: e.Transform.Yaw}
		if _, ok := state.known[e.ID]; !ok {
			batch.Messages = append(batch.Messages, Outbound{Delivery: protocol.DeliveryReliableOrdered, Message: protocol.EntitySpawn{EntityID: e.ID, Kind: e.Kind, Transform: tr}})
		}
		if e.ID == selfID {
			continue
		}

		tier := TierFar
		if hasSelf {
			tier = s.policy.tier(selfPosition, e.Transform.Position)
		}
		previous, hasPrevious := state.lastSnapshot[e.ID]
		dirty := !hasPrevious || previous.Position != e.Transform.Position || previous.Yaw != e.Transform.Yaw
		if dirty {
			batch.Stats.DirtyVisible++
		}
		lastBuild := state.lastSentBuild[e.ID]
		age := state.buildNumber - lastBuild
		cadence := s.policy.cadence(tier)
		forced := hasPrevious && age >= s.policy.refresh(tier)
		dueDirty := dirty && (!hasPrevious || age >= cadence)
		if !dueDirty && !forced {
			continue
		}
		if forced {
			batch.Stats.ForcedRefreshCandidates++
		}
		state.candidates = append(state.candidates, snapshotCandidate{
			entity:  e,
			tier:    tier,
			age:     age,
			cadence: cadence,
			dirty:   dirty,
			forced:  forced,
		})
	}

	state.departed = state.departed[:0]
	for id := range state.known {
		if _, ok := state.scratch[id]; !ok {
			state.departed = append(state.departed, id)
		}
	}
	sort.Slice(state.departed, func(i, j int) bool { return state.departed[i] < state.departed[j] })
	for _, id := range state.departed {
		batch.Messages = append(batch.Messages, Outbound{Delivery: protocol.DeliveryReliableOrdered, Message: protocol.EntityDespawn{EntityID: id}})
		delete(state.lastSnapshot, id)
		delete(state.lastSentBuild, id)
	}

	batch.Stats.SnapshotCandidates = len(state.candidates)
	sort.Slice(state.candidates, func(i, j int) bool {
		a, b := state.candidates[i], state.candidates[j]
		// age/cadence 越大代表相對於自己的 LOD cadence 越 overdue。
		// 交叉相乘避免 hot path 浮點除法，也讓 budget 壓力下的排序 deterministic。
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
	sort.Slice(transforms, func(i, j int) bool { return transforms[i].EntityID < transforms[j].EntityID })

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
		batch.Messages = append(batch.Messages, Outbound{Delivery: protocol.DeliveryRealtimeSequenced, Message: protocol.WorldSnapshot{Tick: tick, ChunkIndex: uint16(chunk), ChunkCount: uint16(chunkCount), Entities: transforms[start:end]}})
	}

	if hasSelf {
		batch.Messages = append(batch.Messages, Outbound{Delivery: protocol.DeliveryRealtimeSequenced, Message: protocol.PositionCorrection{Tick: tick, EntityID: selfTransform.EntityID, Position: selfTransform.Position, Yaw: selfTransform.Yaw, LastProcessedInputSequence: lastProcessedInput}})
	}

	state.known, state.scratch = state.scratch, state.known
	return batch
}
