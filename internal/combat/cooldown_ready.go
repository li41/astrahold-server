package combat

import "time"

// CooldownReadyTick returns the same Server tick that Commit will enforce for this action.
// It is safe to expose as presentation metadata before Commit because it does not mutate
// cooldown state; unsuccessful actions still never emit/commit it.
func CooldownReadyTick(action ActionDefinition, tick uint64, delta time.Duration) uint64 {
	return tick + cooldownTicks(action.CooldownSeconds, delta)
}
