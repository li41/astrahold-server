package worldruntime

import (
	"sort"

	"github.com/li41/astrahold-server/internal/world"
)

// Public monster loot remains claimable for 60 seconds at the formal 20 Hz world cadence.
// Expiry is authoritative tick state; no wall-clock timer or goroutine participates in gameplay.
const defaultItemDropLifetimeTicks uint64 = 20 * 60

// spawnExpiringItemDrop is the normal gameplay creation path for public ground loot. The raw
// spawnItemDrop primitive remains useful for focused tests and other explicitly managed item-drop
// lifecycles, while gameplay loot always receives a bounded Server-owned deadline.
func (r *Runtime) spawnExpiringItemDrop(itemArchetypeID string, position world.Position, tick uint64) (world.EntityID, error) {
	dropID, err := r.spawnItemDrop(itemArchetypeID, position)
	if err != nil {
		return 0, err
	}
	r.itemDropExpireTick[dropID] = itemDropExpiryTick(tick, defaultItemDropLifetimeTicks)
	return dropID, nil
}

// stepItemDropExpiry is world-owner-only. Missing entities are cleanup cases from successful
// pickup, auto-loot, or a same-tick spawn rollback. Expired entities use the normal world removal
// path so existing lifecycle replication produces EntityDespawn without a new Protocol semantic.
func (r *Runtime) stepItemDropExpiry(tick uint64) {
	if len(r.itemDropExpireTick) == 0 {
		return
	}

	expired := make([]world.EntityID, 0)
	for dropID, expireTick := range r.itemDropExpireTick {
		entity, exists := r.world.Entity(dropID)
		if !exists || entity.Kind != world.EntityItemDrop {
			delete(r.itemDropExpireTick, dropID)
			continue
		}
		if tick >= expireTick {
			expired = append(expired, dropID)
		}
	}
	if len(expired) == 0 {
		return
	}

	// Deterministic removal order keeps world-owner behavior stable when several drops expire
	// on the same tick and avoids depending on Go map iteration order.
	sort.Slice(expired, func(i, j int) bool { return expired[i] < expired[j] })
	for _, dropID := range expired {
		entity, exists := r.world.Entity(dropID)
		if exists && entity.Kind == world.EntityItemDrop {
			r.world.Remove(dropID)
		}
		delete(r.itemDropExpireTick, dropID)
	}
}

func itemDropExpiryTick(spawnTick, lifetimeTicks uint64) uint64 {
	if lifetimeTicks > ^uint64(0)-spawnTick {
		return ^uint64(0)
	}
	return spawnTick + lifetimeTicks
}
