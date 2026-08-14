package loadlab

import (
	"errors"
	"math"

	"github.com/li41/astrahold-server/internal/world"
)

type EntityCombatPair struct {
	ActorID  world.EntityID
	TargetID world.EntityID
}

// TeleportChurnCombatPairs 從兩個 mover 群各挑 deterministic adjacent pairs。
// actor/target 都是同一原始 cluster 的 movers，因此 swap / restore 後仍一起落在另一側的相鄰 grid slot。
func TeleportChurnCombatPairs(totalClients, pairsPerGroup int) ([]EntityCombatPair, error) {
	if totalClients < 4 || totalClients%4 != 0 {
		return nil, errors.New("loadlab: teleport-churn clients must be >= 4 and divisible by 4")
	}
	if pairsPerGroup < 0 {
		return nil, errors.New("loadlab: combat pairs per group must be >= 0")
	}
	if pairsPerGroup == 0 {
		return nil, nil
	}

	groupSize := totalClients / 2
	moversPerGroup := groupSize / 2
	cols := int(math.Ceil(math.Sqrt(float64(groupSize))))
	pairs := make([]EntityCombatPair, 0, pairsPerGroup*2)
	for group := 0; group < 2; group++ {
		base := group * groupSize
		selected := 0
		for local := 0; local+1 < moversPerGroup && selected < pairsPerGroup; {
			// 不跨 grid row 配對，避免最後一欄與下一列第一欄在空間上其實不相鄰。
			if local%cols == cols-1 {
				local++
				continue
			}
			pairs = append(pairs, EntityCombatPair{
				ActorID:  world.EntityID(base + local + 1),
				TargetID: world.EntityID(base + local + 2),
			})
			selected++
			local += 2
		}
		if selected != pairsPerGroup {
			return nil, errors.New("loadlab: not enough teleport-churn mover pairs")
		}
	}
	return pairs, nil
}
