package worldruntime

import (
	"errors"
	"sort"

	"github.com/li41/astrahold-server/internal/loot"
	"github.com/li41/astrahold-server/internal/world"
)

var ErrInvalidMonsterLoot = errors.New("worldruntime: invalid monster loot configuration")

type monsterLootState struct {
	sourceArchetypeID string
	configured        bool
	awarded           bool
}

// WithMonsterLootCatalog installs server-authored loot data. Combat and monster lifecycle do not
// know concrete loot tables; worldruntime observes the same authoritative Defeated transition and
// materializes generic item-drop entities through the existing pickup path.
func WithMonsterLootCatalog(catalog *loot.Catalog) Option {
	if catalog == nil {
		panic(ErrInvalidMonsterLoot)
	}
	return func(r *Runtime) {
		if r.monsterLootCatalog != nil {
			panic(ErrInvalidMonsterLoot)
		}
		r.monsterLootCatalog = catalog
	}
}

// trackMonsterLootEntity starts a fresh loot incarnation only after the normal world + character
// spawn path succeeds. Lifecycle respawn uses applySpawnEntity too, so the same EntityID is re-armed
// without introducing a monster-specific spawn pipeline.
func (r *Runtime) trackMonsterLootEntity(entity world.EntityState) {
	if r.monsterLootCatalog == nil || entity.Kind != world.EntityMonster {
		return
	}
	_, configured := r.monsterLootCatalog.DropsFor(entity.ArchetypeID)
	state, exists := r.monsterLootStates[entity.ID]
	if !exists {
		state = &monsterLootState{}
		r.monsterLootStates[entity.ID] = state
		r.monsterLootEntityIDs = append(r.monsterLootEntityIDs, entity.ID)
		sort.Slice(r.monsterLootEntityIDs, func(i, j int) bool {
			return r.monsterLootEntityIDs[i] < r.monsterLootEntityIDs[j]
		})
	}
	state.sourceArchetypeID = entity.ArchetypeID
	state.configured = configured
	state.awarded = false
}

// stepMonsterLoot runs after authoritative simulation/combat updates and before corpse lifecycle.
// The first observed Defeated state wins exactly once for the current spawn incarnation.
func (r *Runtime) stepMonsterLoot(report *StepReport) {
	if r.monsterLootCatalog == nil {
		return
	}
	for _, entityID := range r.monsterLootEntityIDs {
		state := r.monsterLootStates[entityID]
		if state == nil || !state.configured || state.awarded {
			continue
		}
		monster, exists := r.world.Entity(entityID)
		if !exists || monster.Kind != world.EntityMonster || monster.ArchetypeID != state.sourceArchetypeID {
			continue
		}
		vitals, exists := r.characters.State(entityID)
		if !exists || !vitals.Defeated {
			continue
		}
		drops, configured := r.monsterLootCatalog.DropsFor(state.sourceArchetypeID)
		if !configured || len(drops) == 0 {
			continue
		}

		spawned := make([]world.EntityID, 0, len(drops))
		failed := false
		for index, drop := range drops {
			dropID, err := r.spawnItemDrop(drop.ItemArchetypeID, monsterLootDropPosition(monster.Transform.Position, index))
			if err != nil {
				for _, spawnedID := range spawned {
					r.world.Remove(spawnedID)
				}
				report.CommandErrors = append(report.CommandErrors, CommandError{Command: "monster_loot", Err: err})
				failed = true
				break
			}
			spawned = append(spawned, dropID)
		}
		if !failed {
			state.awarded = true
		}
	}
}

func monsterLootDropPosition(source world.Position, index int) world.Position {
	position := source
	ring := float32(index/4+1) * itemDropSpawnOffsetMeters
	switch index % 4 {
	case 0:
		position.X += ring
	case 1:
		position.Z += ring
	case 2:
		position.X -= ring
	case 3:
		position.Z -= ring
	}
	return position
}
