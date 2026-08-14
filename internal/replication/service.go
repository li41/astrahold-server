// Package replication 將 AOI 世界狀態轉成 client 可消費的 spawn/despawn/snapshot/correction 訊息。
package replication

import (
	"container/heap"
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
	SpawnCandidates         int
	SpawnSelected           int
	SpawnDeferred           int
	DespawnCandidates       int
	DespawnSelected         int
	DespawnDeferred         int
	SnapshotCandidates      int
	SnapshotSelected        int
	SnapshotDeferred        int
	DirtyVisible            int
	ForcedRefreshCandidates int
	NearSelected            int
	MidSelected             int
	FarSelected             int
}

// LifecycleLimits 將可重建的 Reliable lifecycle materialization 限制在固定 work quantum。
// 個別 limit 的 0 代表 unlimited，保留既有 Build / BuildFrame compatibility semantics；
// 負值代表本 build 禁止該類 / 全部 lifecycle materialization。
// MaxMessages 是 Spawn + Despawn 共用的 combined quantum，讓 Runtime 可再套 global per-snapshot budget。
type LifecycleLimits struct {
	MaxSpawns   int
	MaxDespawns int
	MaxMessages int
}

func lifecycleLimitAllows(limit, selected int) bool {
	if limit < 0 {
		return false
	}
	return limit == 0 || selected < limit
}

func (l LifecycleLimits) allowSpawn(selected int) bool {
	return lifecycleLimitAllows(l.MaxSpawns, selected)
}

func (l LifecycleLimits) allowDespawn(selected int) bool {
	return lifecycleLimitAllows(l.MaxDespawns, selected)
}

func (l LifecycleLimits) allowMessage(selected int) bool {
	return lifecycleLimitAllows(l.MaxMessages, selected)
}

type Batch struct {
	Messages []Outbound
	Stats    BuildStats
}

type sentTransform struct {
	Position world.Position
	Yaw      float32
}

type entityTrack struct {
	id                      world.EntityID
	known                   bool
	lastDeliveredGeneration uint64
	lastSentBuild           uint64
}

type snapshotCandidate struct {
	entity     world.EntityState
	generation uint64
	trackIndex int
	tier       Tier
	age        uint64
	cadence    uint64
	dirty      bool
}

// snapshotCandidateHeap 把「目前 top-K 中最差的 candidate」放在 root。
// 新 candidate 只要優先序高於 root 就取代它，因此不必對整份 AOI candidates 做 full sort。
type snapshotCandidateHeap []snapshotCandidate

func (h snapshotCandidateHeap) Len() int { return len(h) }
func (h snapshotCandidateHeap) Less(i, j int) bool {
	return candidateHigherPriority(h[j], h[i])
}
func (h snapshotCandidateHeap) Swap(i, j int) { h[i], h[j] = h[j], h[i] }
func (h *snapshotCandidateHeap) Push(value any) {
	*h = append(*h, value.(snapshotCandidate))
}
func (h *snapshotCandidateHeap) Pop() any {
	old := *h
	last := len(old) - 1
	value := old[last]
	*h = old[:last]
	return value
}

// snapshotSelection 在 visible scan 當下就維護 bounded top-K。
// 前 budget 個 candidate 保持原 visible order；只有真的看到第 budget+1 個時才 heap.Init，
// 因此 candidate 數量未超 budget 時與舊 path 的輸出順序完全相同。超額時使用與
// selectSnapshotCandidates 相同的 heap/comparator，只省掉「先 materialize 全部 candidates，再掃第二遍」。
type snapshotSelection struct {
	selected  []snapshotCandidate
	heap      snapshotCandidateHeap
	budget    int
	count     int
	heapReady bool
}

func newSnapshotSelection(buffer []snapshotCandidate, budget int) snapshotSelection {
	if budget > 0 && cap(buffer) < budget {
		buffer = make([]snapshotCandidate, 0, budget)
	} else {
		buffer = buffer[:0]
	}
	return snapshotSelection{selected: buffer, budget: budget}
}

func (s *snapshotSelection) Consider(candidate snapshotCandidate) {
	s.count++
	if s.budget <= 0 || len(s.selected) < s.budget {
		s.selected = append(s.selected, candidate)
		return
	}
	if !s.heapReady {
		s.heap = snapshotCandidateHeap(s.selected)
		heap.Init(&s.heap)
		s.heapReady = true
	}
	if candidateHigherPriority(candidate, s.heap[0]) {
		s.heap[0] = candidate
		heap.Fix(&s.heap, 0)
	}
}

func (s *snapshotSelection) Selected() []snapshotCandidate {
	if s.heapReady {
		return []snapshotCandidate(s.heap)
	}
	return s.selected
}

func (s *snapshotSelection) Count() int { return s.count }

type viewState struct {
	// known 只代表 Reliable EntitySpawn 已成功進入該 Session 的 outbound queue。
	// 它保留為 lifecycle truth / Knows API；steady-state transform scheduler 不再逐 Entity 查 map。
	known map[world.EntityID]struct{}

	// desiredIDs / tracks 與 shared frame 的 stable EntityID order 對齊。
	// AOI membership 不變時，dirty / cadence / known 都走 dense slice，避免每 Session × visible map lookup。
	desiredIDs []world.EntityID
	tracks     []entityTrack

	departed                   []world.EntityID
	lastSnapshot               map[world.EntityID]sentTransform // legacy Build compatibility only
	lastDeliveredGeneration    map[world.EntityID]uint64         // legacy Build compatibility + rare membership rebuild
	lastSentBuild              map[world.EntityID]uint64         // legacy Build compatibility + rare membership rebuild
	candidates                 []snapshotCandidate               // legacy selector tests / compatibility scratch
	selectedCandidates         []snapshotCandidate
	messages                   []Outbound
	borrowedSnapshotTransforms []protocol.EntityTransform
	buildNumber                uint64
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
	if index := desiredIndex(state.desiredIDs, entityID); index >= 0 && index < len(state.tracks) {
		state.tracks[index].known = true
	}
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
	if index := desiredIndex(state.desiredIDs, entityID); index >= 0 && index < len(state.tracks) {
		state.tracks[index].known = false
		state.tracks[index].lastDeliveredGeneration = 0
		state.tracks[index].lastSentBuild = 0
	}
}

// Build 保留給既有單元測試與非 frame caller；production Runtime 使用 BuildFrame。
// compatibility path 仍可從 lastSnapshot 推導 generation，但不位於 S3-E.2+ hot path。
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
	return s.buildFrame(state, selfID, lastProcessedInput, &frame, visibleIndices, false, LifecycleLimits{})
}

// BuildFrame 使用 shared immutable frame 與 AOI index view。
// 回傳的 WorldSnapshot Entities 擁有獨立 backing storage，可交給會非同步保存 Envelope 的 Connection。
func (s *Service) BuildFrame(sessionID session.ID, selfID world.EntityID, lastProcessedInput uint32, frame *simulation.ReplicationFrame, visibleIndices []int) Batch {
	state := s.ensureView(sessionID)
	return s.buildFrame(state, selfID, lastProcessedInput, frame, visibleIndices, false, LifecycleLimits{})
}

// BuildFrameBorrowed 只可搭配 session.ImmediateRealtimeConnection。
// WorldSnapshot Entities 使用 per-session reusable scratch；caller 必須在下一次同 Session build 前，
// 讓每個 Realtime TrySend 成功返回並完成同步 materialization。
func (s *Service) BuildFrameBorrowed(sessionID session.ID, selfID world.EntityID, lastProcessedInput uint32, frame *simulation.ReplicationFrame, visibleIndices []int) Batch {
	state := s.ensureView(sessionID)
	return s.buildFrame(state, selfID, lastProcessedInput, frame, visibleIndices, true, LifecycleLimits{})
}

// BuildFrameWithLifecycleLimits 與 BuildFrame 相同，但限制本次 build 會 materialize 的
// Reliable Spawn / Despawn 數量。未選中的 lifecycle state 仍留在 desired/known truth，下一次 build 重試。
func (s *Service) BuildFrameWithLifecycleLimits(sessionID session.ID, selfID world.EntityID, lastProcessedInput uint32, frame *simulation.ReplicationFrame, visibleIndices []int, limits LifecycleLimits) Batch {
	state := s.ensureView(sessionID)
	return s.buildFrame(state, selfID, lastProcessedInput, frame, visibleIndices, false, limits)
}

// BuildFrameBorrowedWithLifecycleLimits 是 production ImmediateRealtimeConnection 的 bounded lifecycle path。
func (s *Service) BuildFrameBorrowedWithLifecycleLimits(sessionID session.ID, selfID world.EntityID, lastProcessedInput uint32, frame *simulation.ReplicationFrame, visibleIndices []int, limits LifecycleLimits) Batch {
	state := s.ensureView(sessionID)
	return s.buildFrame(state, selfID, lastProcessedInput, frame, visibleIndices, true, limits)
}

func (s *Service) ensureView(sessionID session.ID) *viewState {
	state := s.views[sessionID]
	if state == nil {
		state = newViewState()
		s.views[sessionID] = state
	}
	return state
}

func (s *Service) buildFrame(state *viewState, selfID world.EntityID, lastProcessedInput uint32, frame *simulation.ReplicationFrame, visibleIndices []int, borrowSnapshotStorage bool, lifecycleLimits LifecycleLimits) Batch {
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

	desiredChanged := !sameDesiredIDs(state.desiredIDs, frame, visibleIndices)
	if desiredChanged {
		rebuildDesiredTracks(state, frame, visibleIndices)
	}

	state.messages = state.messages[:0]
	batch := Batch{Messages: state.messages}
	hasUnknown := false
	selection := newSnapshotSelection(state.selectedCandidates, s.policy.MaxTransformsPerBuild)

	for i, index := range visibleIndices {
		if index < 0 || index >= len(frame.Entities) || i >= len(state.tracks) {
			continue
		}
		e := frame.Entities[index]
		generation := frame.TransformGenerations[index]
		track := &state.tracks[i]

		if !track.known {
			hasUnknown = true
			batch.Stats.SpawnCandidates++
			totalLifecycle := batch.Stats.SpawnSelected + batch.Stats.DespawnSelected
			if lifecycleLimits.allowMessage(totalLifecycle) && lifecycleLimits.allowSpawn(batch.Stats.SpawnSelected) {
				tr := protocol.EntityTransform{EntityID: e.ID, Tick: tick, Position: e.Transform.Position, Yaw: e.Transform.Yaw}
				batch.Messages = append(batch.Messages, Outbound{
					Delivery: protocol.DeliveryReliableOrdered,
					Message:  protocol.EntitySpawn{EntityID: e.ID, Kind: e.Kind, Transform: tr},
				})
				batch.Stats.SpawnSelected++
			} else {
				batch.Stats.SpawnDeferred++
			}
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
		hasDelivered := track.lastDeliveredGeneration != 0
		dirty := !hasDelivered || track.lastDeliveredGeneration != generation
		if dirty {
			batch.Stats.DirtyVisible++
		}
		age := state.buildNumber - track.lastSentBuild
		cadence := s.policy.cadence(tier)
		forced := hasDelivered && age >= s.policy.refresh(tier)
		dueDirty := dirty && (!hasDelivered || age >= cadence)
		if !dueDirty && !forced {
			continue
		}
		if forced {
			batch.Stats.ForcedRefreshCandidates++
		}
		selection.Consider(snapshotCandidate{
			entity:     e,
			generation: generation,
			trackIndex: i,
			tier:       tier,
			age:        age,
			cadence:    cadence,
			dirty:      dirty,
		})
	}

	// Steady-state AOI membership 不變且所有 visible 都 known 時，不需要再掃整份 known map。
	// 若 desired 改變、仍有未知 Spawn、或 known 數量大於 desired，才做 Reliable despawn diff。
	state.departed = state.departed[:0]
	if desiredChanged || hasUnknown || len(state.known) > len(state.desiredIDs) {
		for id := range state.known {
			if !containsDesiredID(state.desiredIDs, id) {
				state.departed = append(state.departed, id)
			}
		}
		sort.Slice(state.departed, func(i, j int) bool { return state.departed[i] < state.departed[j] })
	}
	batch.Stats.DespawnCandidates = len(state.departed)
	for _, id := range state.departed {
		totalLifecycle := batch.Stats.SpawnSelected + batch.Stats.DespawnSelected
		if !lifecycleLimits.allowMessage(totalLifecycle) || !lifecycleLimits.allowDespawn(batch.Stats.DespawnSelected) {
			batch.Stats.DespawnDeferred++
			continue
		}
		batch.Messages = append(batch.Messages, Outbound{
			Delivery: protocol.DeliveryReliableOrdered,
			Message:  protocol.EntityDespawn{EntityID: id},
		})
		batch.Stats.DespawnSelected++
	}

	state.selectedCandidates = selection.Selected()
	selectedCandidates := state.selectedCandidates
	candidateCount := selection.Count()
	batch.Stats.SnapshotCandidates = candidateCount
	budgetExceeded := candidateCount > s.policy.MaxTransformsPerBuild
	selectedCount := len(selectedCandidates)
	batch.Stats.SnapshotSelected = selectedCount
	batch.Stats.SnapshotDeferred = candidateCount - selectedCount

	var transforms []protocol.EntityTransform
	if borrowSnapshotStorage {
		state.borrowedSnapshotTransforms = resizeTransforms(state.borrowedSnapshotTransforms, selectedCount)
		transforms = state.borrowedSnapshotTransforms
	} else {
		// Generic Connection 可能在 TrySend 返回後仍非同步持有 Envelope，因此必須給它 owned backing storage。
		transforms = make([]protocol.EntityTransform, selectedCount)
	}
	for i := 0; i < selectedCount; i++ {
		candidate := selectedCandidates[i]
		e := candidate.entity
		transforms[i] = protocol.EntityTransform{EntityID: e.ID, Tick: tick, Position: e.Transform.Position, Yaw: e.Transform.Yaw}
		if candidate.trackIndex >= 0 && candidate.trackIndex < len(state.tracks) {
			track := &state.tracks[candidate.trackIndex]
			track.lastDeliveredGeneration = candidate.generation
			track.lastSentBuild = state.buildNumber
		}
		// compatibility mirrors：legacy Build 會讀；rare membership rebuild 也從這裡重建 dense track。
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

	state.messages = batch.Messages
	return batch
}

// selectSnapshotCandidates 保留給單元測試與 compatibility 對照；production buildFrame
// 已改為 snapshotSelection，在第一趟 visible scan 即維護相同 top-K set。
func selectSnapshotCandidates(buffer []snapshotCandidate, candidates []snapshotCandidate, budget int) []snapshotCandidate {
	if budget <= 0 || len(candidates) <= budget {
		return candidates
	}
	if cap(buffer) < budget {
		buffer = make([]snapshotCandidate, budget)
	} else {
		buffer = buffer[:budget]
	}
	copy(buffer, candidates[:budget])
	selected := snapshotCandidateHeap(buffer)
	heap.Init(&selected)
	for _, candidate := range candidates[budget:] {
		if candidateHigherPriority(candidate, selected[0]) {
			selected[0] = candidate
			heap.Fix(&selected, 0)
		}
	}
	return []snapshotCandidate(selected)
}

func candidateHigherPriority(a, b snapshotCandidate) bool {
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
}

// rebuildDesiredTracks 只在 AOI membership 真正改變時執行。
// S3-E.4 直接重用既有 dense buffers，並從 lifecycle/map mirrors 重建 rare-path state，
// 避免每次 membership change 都配置新的 desiredIDs / tracks。
func rebuildDesiredTracks(state *viewState, frame *simulation.ReplicationFrame, visibleIndices []int) {
	count := len(visibleIndices)
	state.desiredIDs = resizeEntityIDs(state.desiredIDs, count)
	state.tracks = resizeEntityTracks(state.tracks, count)
	for i, index := range visibleIndices {
		if index < 0 || index >= len(frame.Entities) {
			continue
		}
		id := frame.Entities[index].ID
		_, known := state.known[id]
		state.desiredIDs[i] = id
		state.tracks[i] = entityTrack{
			id:                      id,
			known:                   known,
			lastDeliveredGeneration: state.lastDeliveredGeneration[id],
			lastSentBuild:           state.lastSentBuild[id],
		}
	}
}

func resizeEntityIDs(buffer []world.EntityID, count int) []world.EntityID {
	if cap(buffer) < count {
		capacity := count
		if doubled := cap(buffer) * 2; doubled > capacity {
			capacity = doubled
		}
		buffer = make([]world.EntityID, count, capacity)
	} else {
		buffer = buffer[:count]
		clear(buffer)
	}
	return buffer
}

func resizeEntityTracks(buffer []entityTrack, count int) []entityTrack {
	if cap(buffer) < count {
		capacity := count
		if doubled := cap(buffer) * 2; doubled > capacity {
			capacity = doubled
		}
		buffer = make([]entityTrack, count, capacity)
	} else {
		buffer = buffer[:count]
		clear(buffer)
	}
	return buffer
}

func resizeTransforms(buffer []protocol.EntityTransform, count int) []protocol.EntityTransform {
	if cap(buffer) < count {
		capacity := count
		if doubled := cap(buffer) * 2; doubled > capacity {
			capacity = doubled
		}
		return make([]protocol.EntityTransform, count, capacity)
	}
	buffer = buffer[:count]
	clear(buffer)
	return buffer
}

func sameDesiredIDs(previous []world.EntityID, frame *simulation.ReplicationFrame, visibleIndices []int) bool {
	if len(previous) != len(visibleIndices) {
		return false
	}
	for i, index := range visibleIndices {
		if index < 0 || index >= len(frame.Entities) || previous[i] != frame.Entities[index].ID {
			return false
		}
	}
	return true
}

func desiredIndex(ids []world.EntityID, id world.EntityID) int {
	index := sort.Search(len(ids), func(i int) bool { return ids[i] >= id })
	if index < len(ids) && ids[index] == id {
		return index
	}
	return -1
}

func containsDesiredID(ids []world.EntityID, id world.EntityID) bool {
	return desiredIndex(ids, id) >= 0
}
