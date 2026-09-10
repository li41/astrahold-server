package worldruntime

import (
	"errors"

	"github.com/li41/astrahold-server/internal/characterstate"
	"github.com/li41/astrahold-server/internal/learnedskills"
	"github.com/li41/astrahold-server/internal/skillcatalog"
	"github.com/li41/astrahold-server/internal/skillloadout"
	"github.com/li41/astrahold-server/internal/world"
)

var ErrCharacterSkillStateInvalid = errors.New("worldruntime: invalid character skill state")

// characterSkillRuntime owns classless learned-skill and combat-loadout state inside the
// single-threaded world owner. The underlying stores intentionally add no second lock or authority.
type characterSkillRuntime struct {
	learned learnedskills.Store
	loadout skillloadout.Store
}

func validateCharacterSkillValues(learned learnedskills.Set, loadout skillloadout.Slots) error {
	if err := learned.Validate(); err != nil {
		return ErrCharacterSkillStateInvalid
	}
	if err := loadout.Validate(); err != nil {
		return ErrCharacterSkillStateInvalid
	}
	for _, id := range loadout.IDs() {
		if !learned.Contains(id) {
			return ErrCharacterSkillStateInvalid
		}
	}
	return nil
}

// validateCharacterSkillRestore preserves the durable schema boundary. v7 had only the
// combat loadout, so its loader infers exactly those configured skills as learned; earlier
// schemas must not smuggle either field into runtime state.
func validateCharacterSkillRestore(schema uint16, learned learnedskills.Set, loadout skillloadout.Slots) error {
	if schema < characterstate.LoadoutSchemaVersion {
		if learned != (learnedskills.Set{}) || loadout != (skillloadout.Slots{}) {
			return ErrCharacterRestoreInvalid
		}
		return nil
	}
	if err := loadout.Validate(); err != nil {
		return ErrCharacterRestoreInvalid
	}
	if schema < characterstate.LearnedSkillsSchemaVersion {
		inferred, err := learnedskills.NewSet(loadout.IDs())
		if err != nil || inferred != learned {
			return ErrCharacterRestoreInvalid
		}
		return nil
	}
	if err := validateCharacterSkillValues(learned, loadout); err != nil {
		return ErrCharacterRestoreInvalid
	}
	return nil
}

func (s *characterSkillRuntime) restore(entityID world.EntityID, learned learnedskills.Set, loadout skillloadout.Slots) error {
	if s == nil || entityID == 0 {
		return ErrCharacterSkillStateInvalid
	}
	if err := validateCharacterSkillValues(learned, loadout); err != nil {
		return err
	}
	if err := s.learned.SetLearned(entityID, learned.IDs()); err != nil {
		return err
	}
	if err := s.loadout.SetCombat(entityID, loadout.IDs(), func(id skillcatalog.ID) bool {
		return s.learned.Contains(entityID, id)
	}); err != nil {
		s.clear(entityID)
		return err
	}
	return nil
}

func (s *characterSkillRuntime) capture(entityID world.EntityID) (learnedskills.Set, skillloadout.Slots, error) {
	if s == nil || entityID == 0 {
		return learnedskills.Set{}, skillloadout.Slots{}, ErrCharacterSkillStateInvalid
	}
	learned, err := learnedskills.NewSet(s.learned.Learned(entityID))
	if err != nil {
		return learnedskills.Set{}, skillloadout.Slots{}, ErrCharacterSkillStateInvalid
	}
	loadout := s.loadout.Slots(entityID)
	if err := validateCharacterSkillValues(learned, loadout); err != nil {
		return learnedskills.Set{}, skillloadout.Slots{}, err
	}
	return learned, loadout, nil
}

func (s *characterSkillRuntime) clear(entityID world.EntityID) {
	if s == nil || entityID == 0 {
		return
	}
	s.loadout.ClearEntity(entityID)
	s.learned.ClearEntity(entityID)
}
