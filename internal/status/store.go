// Package status owns reusable authoritative transient-effect lifecycle state.
// It deliberately knows nothing about combat formulas, skills, sessions, networking or presentation.
package status

import "github.com/li41/astrahold-server/internal/world"

// ID is a stable Server-side effect identity. It is not a Client asset or presentation key.
type ID string

type key struct {
	entityID world.EntityID
	id       ID
}

type timedValue struct {
	value     uint8
	untilTick uint64
}

// Store is world-owner state for bounded transient effects. Callers define the gameplay meaning of
// each ID and value; Store only owns apply/refresh, half-open expiry and explicit removal lifecycle.
type Store struct {
	active map[key]timedValue
}

func NewStore() *Store {
	return &Store{active: make(map[key]timedValue)}
}

// Apply replaces the current value/deadline for the same entity/effect ID. This is the minimal
// refresh policy needed by the first formal user: Fortify. Other stacking policies stay out until a
// formal gameplay contract requires them.
func (s *Store) Apply(entityID world.EntityID, id ID, value uint8, untilTick uint64) {
	if s == nil || entityID == 0 || id == "" || value == 0 {
		return
	}
	if s.active == nil {
		s.active = make(map[key]timedValue)
	}
	s.active[key{entityID: entityID, id: id}] = timedValue{value: value, untilTick: untilTick}
}

// Value returns an effect only inside the half-open interval before untilTick. Expired state is
// lazily removed on authoritative read, matching the existing Fortify lifecycle without timers.
func (s *Store) Value(entityID world.EntityID, id ID, tick uint64) (uint8, bool) {
	if s == nil || entityID == 0 || id == "" {
		return 0, false
	}
	k := key{entityID: entityID, id: id}
	state, ok := s.active[k]
	if !ok {
		return 0, false
	}
	if tick >= state.untilTick {
		delete(s.active, k)
		return 0, false
	}
	return state.value, true
}

func (s *Store) Remove(entityID world.EntityID, id ID) {
	if s == nil || entityID == 0 || id == "" {
		return
	}
	delete(s.active, key{entityID: entityID, id: id})
}

// ClearEntity is the lifecycle boundary for defeat/despawn/incarnation replacement when the owner
// chooses to remove all runtime-only effects for an entity.
func (s *Store) ClearEntity(entityID world.EntityID) {
	if s == nil || entityID == 0 {
		return
	}
	for k := range s.active {
		if k.entityID == entityID {
			delete(s.active, k)
		}
	}
}

// Deadline performs saturating tick arithmetic so a transient effect can never wrap into an
// immediately-active ancient deadline.
func Deadline(startTick, durationTicks uint64) uint64 {
	if durationTicks > ^uint64(0)-startTick {
		return ^uint64(0)
	}
	return startTick + durationTicks
}
