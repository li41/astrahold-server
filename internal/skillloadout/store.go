// Package skillloadout owns the Server-side classless combat-skill loadout state.
// It deliberately does not decide how skills are learned, persisted or replicated.
package skillloadout

import (
	"errors"

	"github.com/li41/astrahold-server/internal/skillcatalog"
	"github.com/li41/astrahold-server/internal/world"
)

var (
	ErrInvalidEntity     = errors.New("skillloadout: invalid entity")
	ErrNilStore          = errors.New("skillloadout: nil store")
	ErrNonCanonicalSlots = errors.New("skillloadout: non-canonical slots")
)

// Slots is the comparable six-slot value form used across runtime and durable character state.
// Configured skills occupy a contiguous prefix; unused trailing slots are empty.
type Slots [skillcatalog.MaxCombatLoadoutSkills]skillcatalog.ID

// NewSlots validates a combat loadout and converts it to the canonical comparable slot form.
func NewSlots(ids []skillcatalog.ID) (Slots, error) {
	if err := skillcatalog.ValidateCombatLoadout(ids); err != nil {
		return Slots{}, err
	}
	var slots Slots
	copy(slots[:], ids)
	return slots, nil
}

// Validate rejects holes and any skill that is not legal for the formal six-slot combat loadout.
func (slots Slots) Validate() error {
	ids := make([]skillcatalog.ID, 0, len(slots))
	seenEmpty := false
	for _, id := range slots {
		if id == "" {
			seenEmpty = true
			continue
		}
		if seenEmpty {
			return ErrNonCanonicalSlots
		}
		ids = append(ids, id)
	}
	return skillcatalog.ValidateCombatLoadout(ids)
}

// IDs returns the configured contiguous prefix in slot order.
func (slots Slots) IDs() []skillcatalog.ID {
	count := 0
	for count < len(slots) && slots[count] != "" {
		count++
	}
	if count == 0 {
		return nil
	}
	ids := make([]skillcatalog.ID, count)
	copy(ids, slots[:count])
	return ids
}

// Store is world-owner mutable state. Callers must mutate it only from the authoritative
// gameplay owner path; the store does not add its own locking or network semantics.
type Store struct {
	combat map[world.EntityID]Slots
}

func NewStore() *Store {
	return &Store{combat: make(map[world.EntityID]Slots)}
}

// SetCombat replaces the ordered combat loadout for an entity after validating the formal
// six-slot classless rules. A rejected replacement leaves the previous authoritative state intact.
// An empty loadout clears the entity's combat slots, which is useful while editing or restoring
// a character before a complete build is selected.
func (s *Store) SetCombat(entityID world.EntityID, ids []skillcatalog.ID) error {
	if entityID == 0 {
		return ErrInvalidEntity
	}
	if len(ids) == 0 {
		if s != nil {
			delete(s.combat, entityID)
		}
		return nil
	}
	if s == nil {
		return ErrNilStore
	}
	slots, err := NewSlots(ids)
	if err != nil {
		return err
	}
	if s.combat == nil {
		s.combat = make(map[world.EntityID]Slots)
	}
	s.combat[entityID] = slots
	return nil
}

// Combat returns a defensive copy in slot order. Missing entities have an empty loadout.
func (s *Store) Combat(entityID world.EntityID) []skillcatalog.ID {
	if s == nil || entityID == 0 {
		return nil
	}
	return s.combat[entityID].IDs()
}

// Slots returns the comparable canonical slot value for an entity.
func (s *Store) Slots(entityID world.EntityID) Slots {
	if s == nil || entityID == 0 {
		return Slots{}
	}
	return s.combat[entityID]
}

// ContainsCombat reports whether a combat skill is currently configured for the entity.
func (s *Store) ContainsCombat(entityID world.EntityID, id skillcatalog.ID) bool {
	if s == nil || entityID == 0 || id == "" {
		return false
	}
	for _, configured := range s.combat[entityID] {
		if configured == id {
			return true
		}
	}
	return false
}

// ClearEntity removes runtime loadout state at an authoritative entity lifecycle boundary.
// Durable character restoration, when added, must explicitly restore its own persisted state.
func (s *Store) ClearEntity(entityID world.EntityID) {
	if s == nil || entityID == 0 {
		return
	}
	delete(s.combat, entityID)
}
