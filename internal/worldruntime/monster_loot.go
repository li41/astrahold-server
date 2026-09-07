package worldruntime

import (
	"errors"
	"sort"

	"github.com/li41/astrahold-server/internal/characteridentity"
	"github.com/li41/astrahold-server/internal/loot"
	"github.com/li41/astrahold-server/internal/session"
	"github.com/li41/astrahold-server/internal/world"
)

var ErrInvalidMonsterLoot = errors.New("worldruntime: invalid monster loot configuration")

type monsterLootState struct {
	sourceArchetypeID string
	configured        bool
	awarded           bool
	ownerCharacterID  characteridentity.ID
	dropOwners        map[world.EntityID]characteridentity.ID
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
		state = &monsterLootState{dropOwners: make(map[world.EntityID]characteridentity.ID)}
		r.monsterLootStates[entity.ID] = state
		r.monsterLootEntityIDs = append(r.monsterLootEntityIDs, entity.ID)
		sort.Slice(r.monsterLootEntityIDs, func(i, j int) bool {
			return r.monsterLootEntityIDs[i] < r.monsterLootEntityIDs[j]
		})
	} else if state.dropOwners == nil {
		state.dropOwners = make(map[world.EntityID]characteridentity.ID)
	}
	state.sourceArchetypeID = entity.ArchetypeID
	state.configured = configured
	state.awarded = false
	state.ownerCharacterID = ""
}

// recordMonsterLootOwner captures the durable character identity that delivered the authoritative
// defeating hit. Server-owned/environment defeats keep ownerCharacterID empty and therefore produce
// public drops. Ownership is recorded only for a real source Session bound to the attacking entity.
func (r *Runtime) recordMonsterLootOwner(monsterID, actorID world.EntityID, sourceSessionID session.ID) {
	if sourceSessionID == 0 {
		return
	}
	state := r.monsterLootStates[monsterID]
	if state == nil || !state.configured || state.awarded {
		return
	}
	s, ok := r.sessions.Get(sourceSessionID)
	if !ok || s.EntityID != actorID || !s.CharacterIdentity.Valid() {
		return
	}
	state.ownerCharacterID = s.CharacterIdentity.ID
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
					delete(state.dropOwners, spawnedID)
				}
				report.CommandErrors = append(report.CommandErrors, CommandError{Command: "monster_loot", Err: err})
				failed = true
				break
			}
			spawned = append(spawned, dropID)
			if state.ownerCharacterID != "" {
				state.dropOwners[dropID] = state.ownerCharacterID
			}
		}
		if !failed {
			state.awarded = true
		}
	}
}

func (r *Runtime) itemDropPickupOwner(dropID world.EntityID) (characteridentity.ID, bool) {
	for _, monsterID := range r.monsterLootEntityIDs {
		state := r.monsterLootStates[monsterID]
		if state == nil || len(state.dropOwners) == 0 {
			continue
		}
		owner, ok := state.dropOwners[dropID]
		if ok {
			return owner, true
		}
	}
	return "", false
}

func (r *Runtime) clearItemDropPickupOwner(dropID world.EntityID) {
	for _, monsterID := range r.monsterLootEntityIDs {
		state := r.monsterLootStates[monsterID]
		if state == nil || len(state.dropOwners) == 0 {
			continue
		}
		if _, ok := state.dropOwners[dropID]; ok {
			delete(state.dropOwners, dropID)
			return
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
