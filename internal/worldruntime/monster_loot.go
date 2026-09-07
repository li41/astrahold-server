package worldruntime

import (
	"encoding/binary"
	"errors"
	"hash/fnv"
	"sort"

	"github.com/li41/astrahold-server/internal/characteridentity"
	"github.com/li41/astrahold-server/internal/inventory"
	"github.com/li41/astrahold-server/internal/loot"
	"github.com/li41/astrahold-server/internal/session"
	"github.com/li41/astrahold-server/internal/world"
)

const monsterAutoLootRadiusMeters = float32(2.0)

var (
	ErrInvalidMonsterLoot              = errors.New("worldruntime: invalid monster loot configuration")
	ErrMonsterLootInventoryUnavailable = errors.New("worldruntime: monster loot inventory unavailable")
)

type monsterLootContribution struct {
	characterID characteridentity.ID
	damage      uint64
}

type monsterLootState struct {
	sourceArchetypeID string
	configured        bool
	awarded           bool
	incarnation       uint64
	contributions     map[characteridentity.ID]monsterLootContribution
}

type monsterLootCandidate struct {
	sessionID   session.ID
	characterID characteridentity.ID
	damage      uint64
}

// WithMonsterLootCatalog installs server-authored loot data. Combat and monster lifecycle do not
// know concrete loot tables; worldruntime observes the authoritative Defeated transition and then
// applies the Server-owned nearby auto-loot policy or leaves public item-drop entities on the ground.
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
// without introducing a monster-specific spawn pipeline. Damage credit never crosses incarnations.
func (r *Runtime) trackMonsterLootEntity(entity world.EntityState) {
	if r.monsterLootCatalog == nil || entity.Kind != world.EntityMonster {
		return
	}
	_, configured := r.monsterLootCatalog.DropsFor(entity.ArchetypeID)
	state, exists := r.monsterLootStates[entity.ID]
	if !exists {
		state = &monsterLootState{contributions: make(map[characteridentity.ID]monsterLootContribution)}
		r.monsterLootStates[entity.ID] = state
		r.monsterLootEntityIDs = append(r.monsterLootEntityIDs, entity.ID)
		sort.Slice(r.monsterLootEntityIDs, func(i, j int) bool {
			return r.monsterLootEntityIDs[i] < r.monsterLootEntityIDs[j]
		})
	} else if state.contributions == nil {
		state.contributions = make(map[characteridentity.ID]monsterLootContribution)
	}
	if state.incarnation != ^uint64(0) {
		state.incarnation++
	}
	state.sourceArchetypeID = entity.ArchetypeID
	state.configured = configured
	state.awarded = false
	clear(state.contributions)
}

// recordMonsterLootDamage records only actual successful player damage. Rejected attacks, misses,
// Server-owned AI and overkill beyond the target's remaining HP never inflate a player's loot odds.
func (r *Runtime) recordMonsterLootDamage(monsterID, actorID world.EntityID, sourceSessionID session.ID, actualDamage uint32) {
	if sourceSessionID == 0 || actualDamage == 0 {
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
	id := s.CharacterIdentity.ID
	contribution := state.contributions[id]
	contribution.characterID = id
	amount := uint64(actualDamage)
	if contribution.damage > ^uint64(0)-amount {
		contribution.damage = ^uint64(0)
	} else {
		contribution.damage += amount
	}
	state.contributions[id] = contribution
}

// resetMonsterLootContributions is part of encounter reset: damage from a failed leash pull cannot
// survive evade and improve the next attempt's loot probability.
func (r *Runtime) resetMonsterLootContributions(monsterID world.EntityID) {
	state := r.monsterLootStates[monsterID]
	if state == nil || len(state.contributions) == 0 {
		return
	}
	clear(state.contributions)
}

// stepMonsterLoot runs after authoritative simulation/combat updates and before corpse lifecycle.
// Every resolved item is first materialized as an immediately-public ground drop. Eligible nearby
// contributors may then receive it directly; if direct inventory insertion cannot succeed, the same
// public drop simply remains in the world. There is deliberately no pickup reservation timer.
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
		if failed {
			continue
		}

		candidates := r.nearbyMonsterLootCandidates(monster.Transform.Position, state)
		for index, dropID := range spawned {
			if len(candidates) == 0 {
				continue
			}
			winner := candidates[0]
			if len(candidates) > 1 {
				total := monsterLootCandidateTotalDamage(candidates)
				if total == 0 {
					continue
				}
				ticket := monsterLootDeterministicTicket(entityID, state.incarnation, index, total)
				selected, ok := selectDamageWeightedMonsterLootCandidate(candidates, ticket)
				if !ok {
					continue
				}
				winner = selected
			}
			r.tryAutoGrantMonsterLoot(winner, drops[index].ItemArchetypeID, dropID, report)
		}
		state.awarded = true
	}
}

func (r *Runtime) nearbyMonsterLootCandidates(position world.Position, state *monsterLootState) []monsterLootCandidate {
	if state == nil || len(state.contributions) == 0 {
		return nil
	}
	radiusSq := monsterAutoLootRadiusMeters * monsterAutoLootRadiusMeters
	candidates := make([]monsterLootCandidate, 0, len(state.contributions))
	for _, s := range r.sessions.List() {
		contribution, contributed := state.contributions[s.CharacterIdentity.ID]
		if !contributed || contribution.damage == 0 {
			continue
		}
		player, exists := r.world.Entity(s.EntityID)
		if !exists || player.Kind != world.EntityPlayer || player.Transform.Position.Layer != position.Layer {
			continue
		}
		vitals, exists := r.characters.State(player.ID)
		if !exists || vitals.Defeated {
			continue
		}
		if player.Transform.Position.DistanceXZSquared(position) > radiusSq {
			continue
		}
		candidates = append(candidates, monsterLootCandidate{
			sessionID: s.ID, characterID: s.CharacterIdentity.ID, damage: contribution.damage,
		})
	}
	// Probability is damage-proportional; stable CharacterIdentity ordering makes deterministic
	// ticket intervals independent from transient session enumeration order.
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].characterID < candidates[j].characterID
	})
	return candidates
}

func monsterLootCandidateTotalDamage(candidates []monsterLootCandidate) uint64 {
	var total uint64
	for _, candidate := range candidates {
		if candidate.damage > ^uint64(0)-total {
			return ^uint64(0)
		}
		total += candidate.damage
	}
	return total
}

// selectDamageWeightedMonsterLootCandidate maps [0,totalDamage) into contiguous damage-sized
// intervals. Therefore a character responsible for 70% of nearby recorded damage has exactly 70%
// of the ticket space for each resolved drop under normal bounded combat totals.
func selectDamageWeightedMonsterLootCandidate(candidates []monsterLootCandidate, ticket uint64) (monsterLootCandidate, bool) {
	total := monsterLootCandidateTotalDamage(candidates)
	if total == 0 || ticket >= total {
		return monsterLootCandidate{}, false
	}
	var cumulative uint64
	for _, candidate := range candidates {
		if candidate.damage == 0 {
			continue
		}
		remaining := total - cumulative
		if candidate.damage >= remaining {
			return candidate, true
		}
		cumulative += candidate.damage
		if ticket < cumulative {
			return candidate, true
		}
	}
	return monsterLootCandidate{}, false
}

func monsterLootDeterministicTicket(monsterID world.EntityID, incarnation uint64, dropIndex int, total uint64) uint64 {
	if total == 0 {
		return 0
	}
	var encoded [24]byte
	binary.LittleEndian.PutUint64(encoded[0:8], uint64(monsterID))
	binary.LittleEndian.PutUint64(encoded[8:16], incarnation)
	binary.LittleEndian.PutUint64(encoded[16:24], uint64(dropIndex))
	h := fnv.New64a()
	_, _ = h.Write(encoded[:])
	return h.Sum64() % total
}

func (r *Runtime) tryAutoGrantMonsterLoot(candidate monsterLootCandidate, itemArchetypeID string, dropID world.EntityID, report *StepReport) bool {
	inv := r.inventories[candidate.characterID]
	if inv == nil {
		report.CommandErrors = append(report.CommandErrors, CommandError{Command: "monster_loot_auto_grant", SessionID: candidate.sessionID, Err: ErrMonsterLootInventoryUnavailable})
		return false
	}
	if err := inv.Add(itemArchetypeID, 1); err != nil {
		// Capacity failure is expected gameplay: the item remains immediately public on the ground.
		if errors.Is(err, inventory.ErrFull) || errors.Is(err, inventory.ErrWeightExceeded) || errors.Is(err, inventory.ErrQuantityOverflow) {
			return false
		}
		report.CommandErrors = append(report.CommandErrors, CommandError{Command: "monster_loot_auto_grant", SessionID: candidate.sessionID, Err: err})
		return false
	}
	r.world.Remove(dropID)
	r.sessionInventoryPending[candidate.sessionID] = struct{}{}
	return true
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
