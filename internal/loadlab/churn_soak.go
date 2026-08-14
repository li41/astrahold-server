package loadlab

import (
	"github.com/li41/astrahold-server/internal/gameplayworld"
	"github.com/li41/astrahold-server/internal/world"
)

// TeleportChurnRestoreTargets 回傳 S3-E.8 repeated churn 的 restore plan。
// movers 與 TeleportChurnTargets 完全相同，但 target 是各自原始 cluster 的同一 grid slot，
// 因此 load server 可以 swap -> restore -> swap...，每一輪都真正改變 AOI membership。
func TeleportChurnRestoreTargets(def gameplayworld.Definition, totalClients int) (map[world.EntityID]world.Position, error) {
	layout, err := buildLayout(def, ScenarioTeleportChurn)
	if err != nil {
		return nil, err
	}
	if err := validateTeleportChurnLayout(layout, totalClients); err != nil {
		return nil, err
	}

	west, east := teleportChurnBounds(layout)
	groupSize := totalClients / 2
	moversPerGroup := groupSize / 2
	targets := make(map[world.EntityID]world.Position, moversPerGroup*2)
	for localIndex := 0; localIndex < moversPerGroup; localIndex++ {
		westID := world.EntityID(localIndex + 1)
		eastID := world.EntityID(groupSize + localIndex + 1)
		targets[westID] = pointOnSurface(layout.ground, gridPoint(west, localIndex, groupSize, 0.25))
		targets[eastID] = pointOnSurface(layout.ground, gridPoint(east, localIndex, groupSize, 0.25))
	}
	return targets, nil
}
