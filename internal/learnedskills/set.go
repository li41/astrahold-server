package learnedskills

import (
	"errors"
	"fmt"

	"github.com/li41/astrahold-server/internal/skillcatalog"
)

const maxDurableSkills = 20

var (
	ErrTooManySkills = errors.New("learnedskills: too many skills")
	ErrInvalidSet    = errors.New("learnedskills: invalid set")
)

// Set is a comparable, canonical value representation of learned skills.
// The fixed capacity intentionally matches the formal classless V1 catalog; expanding
// that catalog requires an explicit durable-schema migration instead of silently changing
// persisted character semantics.
type Set struct {
	ids   [maxDurableSkills]skillcatalog.ID
	count uint8
}

// NewSet validates formal skill IDs, rejects duplicates and canonicalizes catalog order.
func NewSet(ids []skillcatalog.ID) (Set, error) {
	if len(ids) > maxDurableSkills {
		return Set{}, fmt.Errorf("%w: got=%d max=%d", ErrTooManySkills, len(ids), maxDurableSkills)
	}
	if len(ids) == 0 {
		return Set{}, nil
	}

	requested := make(map[skillcatalog.ID]struct{}, len(ids))
	for _, id := range ids {
		if _, ok := skillcatalog.Lookup(id); !ok {
			return Set{}, fmt.Errorf("%w: %q", skillcatalog.ErrUnknownSkill, id)
		}
		if _, exists := requested[id]; exists {
			return Set{}, fmt.Errorf("%w: %q", skillcatalog.ErrDuplicateSkill, id)
		}
		requested[id] = struct{}{}
	}

	var set Set
	for _, definition := range skillcatalog.All() {
		if _, ok := requested[definition.ID]; !ok {
			continue
		}
		if int(set.count) >= len(set.ids) {
			return Set{}, ErrTooManySkills
		}
		set.ids[set.count] = definition.ID
		set.count++
	}
	if int(set.count) != len(requested) {
		return Set{}, ErrInvalidSet
	}
	return set, nil
}

// Validate protects durable snapshot boundaries even though normal callers can only
// construct canonical values through NewSet or the zero value.
func (s Set) Validate() error {
	if int(s.count) > len(s.ids) {
		return ErrInvalidSet
	}
	ids := s.IDs()
	canonical, err := NewSet(ids)
	if err != nil {
		return err
	}
	if canonical != s {
		return ErrInvalidSet
	}
	return nil
}

// IDs returns a fresh slice in formal catalog order.
func (s Set) IDs() []skillcatalog.ID {
	if s.count == 0 {
		return nil
	}
	out := make([]skillcatalog.ID, int(s.count))
	copy(out, s.ids[:s.count])
	return out
}

func (s Set) Contains(id skillcatalog.ID) bool {
	for i := 0; i < int(s.count); i++ {
		if s.ids[i] == id {
			return true
		}
	}
	return false
}

func (s Set) Len() int { return int(s.count) }
