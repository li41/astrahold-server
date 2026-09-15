// Package learnedskills owns the Server-side classless learned-skill state.
// It deliberately does not decide skill acquisition, persistence, replication or combat-loadout legality.
package learnedskills

import (
	"errors"
	"fmt"

	"github.com/li41/astrahold-server/internal/skillcatalog"
	"github.com/li41/astrahold-server/internal/world"
)

var (
	ErrInvalidEntity = errors.New("learnedskills: invalid entity")
	ErrNilStore      = errors.New("learnedskills: nil store")
)

// Store is world-owner mutable state. Callers must mutate it only from the authoritative
// gameplay owner path; the store does not add its own locking, acquisition or network semantics.
type Store struct {
	byEntity map[world.EntityID]map[skillcatalog.ID]struct{}
}

func NewStore() *Store {
	return &Store{byEntity: make(map[world.EntityID]map[skillcatalog.ID]struct{})}
}

// SetLearned replaces the complete learned-skill set for an entity. Any formal catalog skill is
// legal here; six-slot combat eligibility is a separate concern. A rejected replacement leaves
// the previous authoritative state intact. An empty set clears runtime learned-skill state.
func (s *Store) SetLearned(entityID world.EntityID, ids []skillcatalog.ID) error {
	if entityID == 0 {
		return ErrInvalidEntity
	}
	if len(ids) == 0 {
		if s != nil {
			delete(s.byEntity, entityID)
		}
		return nil
	}
	if s == nil {
		return ErrNilStore
	}

	learned, err := validateSet(ids)
	if err != nil {
		return err
	}
	if s.byEntity == nil {
		s.byEntity = make(map[world.EntityID]map[skillcatalog.ID]struct{})
	}
	s.byEntity[entityID] = learned
	return nil
}

func validateSet(ids []skillcatalog.ID) (map[skillcatalog.ID]struct{}, error) {
	learned := make(map[skillcatalog.ID]struct{}, len(ids))
	for _, id := range ids {
		if _, ok := skillcatalog.Lookup(id); !ok {
			return nil, fmt.Errorf("%w: %q", skillcatalog.ErrUnknownSkill, id)
		}
		if _, exists := learned[id]; exists {
			return nil, fmt.Errorf("%w: %q", skillcatalog.ErrDuplicateSkill, id)
		}
		learned[id] = struct{}{}
	}
	return learned, nil
}

// Learned returns the learned skills in formal catalog order. The returned slice is newly
// allocated, so callers cannot mutate authoritative state through it.
func (s *Store) Learned(entityID world.EntityID) []skillcatalog.ID {
	if s == nil || entityID == 0 {
		return nil
	}
	learned := s.byEntity[entityID]
	if len(learned) == 0 {
		return nil
	}

	out := make([]skillcatalog.ID, 0, len(learned))
	for _, definition := range skillcatalog.All() {
		if _, ok := learned[definition.ID]; ok {
			out = append(out, definition.ID)
		}
	}
	return out
}

// Contains reports whether the entity has learned the formal skill ID.
func (s *Store) Contains(entityID world.EntityID, id skillcatalog.ID) bool {
	if s == nil || entityID == 0 || id == "" {
		return false
	}
	_, ok := s.byEntity[entityID][id]
	return ok
}

func (s *Store) Count(entityID world.EntityID) int {
	if s == nil || entityID == 0 {
		return 0
	}
	return len(s.byEntity[entityID])
}

// ClearEntity removes runtime learned-skill state at an authoritative entity lifecycle boundary.
// Durable character restoration, when added, must explicitly restore its own persisted state.
func (s *Store) ClearEntity(entityID world.EntityID) {
	if s == nil || entityID == 0 {
		return
	}
	delete(s.byEntity, entityID)
}
