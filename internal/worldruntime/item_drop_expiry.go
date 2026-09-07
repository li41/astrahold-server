package worldruntime

import (
	"sort"

	"github.com/li41/astrahold-server/internal/world"
)

// The formal worldd currently runs at 20 Hz by default, making this a 60-second public ground window.
// Composition roots that intentionally change tick cadence can override ItemDropLifetimeTicks.
const defaultItemDropLifetimeTicks uint64 = 20 * 60

// spawnExpiringItemDrop is the normal gameplay creation path for public ground loot. The raw
// spawnItemDrop primitive remains responsible only for materializing the authoritative entity.
func (r *Runtime) spawnExpiringItemDrop(itemArchetypeID string, position world.Position, tick uint64) (world.EntityID, error) {
	dropID, err := r.spawnItemDrop(itemArchetypeID, position)
	if err != nil {
		return 0, err
	}
	if r.itemDropExpireTick == nil {
		r.itemDropExpireTick = make(map[world.EntityID]uint64)
	}
	r.itemDropExpireTick[dropID] = itemDropExpiryTick(tick, r.config.ItemDropLifetimeTicks)
	return dropID, nil
}

// stepItemDropExpiry is world-owner-only. It never performs I/O and removes expired entities from
// the same authoritative simulation used by pickup; normal lifecycle replication then emits the
// existing EntityDespawn message to observers without a new Protocol semantic.
func (r *Runtime) stepItemDropExpiry(tick uint64, report *StepReport) {
	if len(r.itemDropExpireTick) == 0 {
		return
	}

	expired := make([]world.EntityID, 0)
	for dropID, expireTick := range r.itemDropExpireTick {
		entity, exists := r.world.Entity(dropID)
		if !exists || entity.Kind != world.EntityItemDrop {
			// Pickup, auto-loot, or rollback already removed it. Clear lifecycle bookkeeping now.
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
	sort.Slice(expired, func(i, j int) bool { return expired[i] < expired[j] })
	for _, dropID := range expired {
		entity, exists := r.world.Entity(dropID)
		if exists && entity.Kind == world.EntityItemDrop {
			r.world.Remove(dropID)
			report.Metrics.ItemDropsExpired++
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
