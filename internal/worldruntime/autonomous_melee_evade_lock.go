package worldruntime

import (
	"sort"

	"github.com/li41/astrahold-server/internal/world"
)

// autonomousMeleeReturningHome answers encounter-reset legality without creating a second
// combat rules engine. Autonomous melee agents are kept sorted by EntityID at construction, so
// combat validation can query the Server-owned evade state without a linear scan of all monsters.
func (r *Runtime) autonomousMeleeReturningHome(entityID world.EntityID) bool {
	index := sort.Search(len(r.autonomousMeleeAgents), func(i int) bool {
		return r.autonomousMeleeAgents[i].config.EntityID >= entityID
	})
	return index < len(r.autonomousMeleeAgents) &&
		r.autonomousMeleeAgents[index].config.EntityID == entityID &&
		r.autonomousMeleeAgents[index].returningHome
}
