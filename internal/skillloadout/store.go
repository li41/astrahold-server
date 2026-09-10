// Package skillloadout owns the Server-side classless combat-skill loadout state.
// It deliberately does not decide how skills are learned, persisted or replicated.
package skillloadout

import (
	"errors"

	"github.com/li41/astrahold-server/internal/skillcatalog"
	"github.com/li41/astrahold-server/internal/world"
)

var ErrInvalidEntity = errors.New("skillloadout: invalid entity")

// Store is world-owner mutable state. Callers must mutate it only from the authoritative
// gameplay owner path; the store does not add its own locking or network semantics.
type Store struct {
	combat map[world.EntityID][]skillcatalog.ID
}

func NewStore() *Store {
	return &Store{combat: make(map[world.EntityID][]skillcatalog.ID)}
}

// SetCombat replaces the ordered combat loadout for an entity after validating the formal
// six-slot classless rules. A rejected replacement leaves the previous authoritative state intact.
// An empty loadout clears the entity's combat slots, which is useful while editing or restoring
// a character before a complete build is selected.
func (s *Store) SetCombat(entityID world.EntityID, ids []skillcatalog.ID) error {
	if entityID == 0 {
		return ErrInvalidEntity
	}
	if err := skillcatalog.ValidateCombatLoadout(ids); err != nil {
		return err
	}
	if len(ids) == 0 {
		if s != nil {
			delete(s.combat, entityID)
		}
		return nil
	}
	if s == nil {
		return errors.New("skillloadout: nil store")
	}
	if s.combat == nil {
		s.combat = make(map[world.EntityID][]skillcatalog.ID)
	}
	loadout := append([]skillcatalog.ID(nil), ids...)
	s.combat[entityID] = loadout
	return nil
}

// Combat returns a defensive copy in slot order. Missing entities have an empty loadout.
func (s *Store) Combat(entityID world.EntityID) []skillcatalog.ID {
	if s == nil || entityID == 0 {
		return nil
	}
	return append([]skillcatalog.ID(nil), s.combat[entityID]...)
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
