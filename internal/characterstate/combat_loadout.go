package characterstate

import (
	"github.com/li41/astrahold-server/internal/skillcatalog"
	"github.com/li41/astrahold-server/internal/skillloadout"
)

func combatLoadoutToWire(slots skillloadout.Slots) []string {
	ids := slots.IDs()
	if len(ids) == 0 {
		return nil
	}
	out := make([]string, len(ids))
	for i, id := range ids {
		out[i] = string(id)
	}
	return out
}

func combatLoadoutFromWire(raw []string) (skillloadout.Slots, error) {
	ids := make([]skillcatalog.ID, len(raw))
	for i, id := range raw {
		ids[i] = skillcatalog.ID(id)
	}
	slots, err := skillloadout.NewSlots(ids)
	if err != nil {
		return skillloadout.Slots{}, ErrInvalidSnapshot
	}
	return slots, nil
}

func validateCombatLoadout(slots skillloadout.Slots) error {
	if err := slots.Validate(); err != nil {
		return ErrInvalidSnapshot
	}
	return nil
}
