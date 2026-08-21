package worldruntime

import (
	"github.com/li41/astrahold-server/internal/character"
	"github.com/li41/astrahold-server/internal/world"
)

// combatActorKind identifies world entities that may originate an action. Siege objects are
// valid sources (for example siege engines / existing death-context fixtures) even though
// they do not use the generic entity-target HP path as targets.
func combatActorKind(kind world.EntityKind) bool {
	switch kind {
	case world.EntityPlayer, world.EntityNPC, world.EntityMonster, world.EntitySiegeObject:
		return true
	default:
		return false
	}
}

// combatantKind identifies entity kinds that may be targeted by generic entity combat.
// Siege objects keep their dedicated topology/HP path and are intentionally excluded.
func combatantKind(kind world.EntityKind) bool {
	switch kind {
	case world.EntityPlayer, world.EntityNPC, world.EntityMonster:
		return true
	default:
		return false
	}
}

// combatantState is the runtime boundary between generic combat and the current health-state
// implementation. The underlying character.Service is already an EntityID -> HP state map;
// player-only durability/respawn/death policies remain outside this adapter and branch on Kind.
func (r *Runtime) combatantState(id world.EntityID) (character.State, bool) {
	return r.characters.State(id)
}

func (r *Runtime) reduceCombatantHP(id world.EntityID, amount uint32) (character.State, error) {
	return r.characters.ReduceHP(id, amount)
}
