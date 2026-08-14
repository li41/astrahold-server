package main

import (
	"flag"
	"fmt"
	"strconv"

	"github.com/li41/astrahold-server/internal/loadlab"
	"github.com/li41/astrahold-server/internal/protocol"
	"github.com/li41/astrahold-server/internal/session"
	"github.com/li41/astrahold-server/internal/worldruntime"
)

var churnCombatPairsPerGroup = flag.Int(
	"churn-combat-pairs-per-group",
	0,
	"Deterministic basic-attack pairs per teleport cluster in each churn round; 0 disables combat overlap",
)

type churnCombatMetrics struct {
	ActionsApplied      uint64 `json:"combat_actions_applied"`
	ActionRejections    uint64 `json:"action_rejections"`
	DirtyVitalsSelected uint64 `json:"dirty_vitals_selected"`
}

func enqueueChurnCombatActions(runtime *worldruntime.Runtime, round int, pairs []loadlab.EntityCombatPair) error {
	for _, pair := range pairs {
		action := protocol.ClientUseAction{
			ActionID:   "basic-attack",
			TargetKind: protocol.ActionTargetEntity,
			TargetID:   strconv.FormatUint(uint64(pair.TargetID), 10),
		}
		// Load Lab 的 tcpudp Server 以同一 atomic order 配發 SessionID / EntityID，
		// 因此 deterministic player IDs 可直接對應 SessionID；每輪 round number 也是
		// 每個 actor 嚴格遞增的 action sequence。
		if err := runtime.EnqueueUseAction(session.ID(pair.ActorID), uint32(round), action); err != nil {
			return fmt.Errorf("enqueue churn combat actor=%d target=%d round=%d: %w", pair.ActorID, pair.TargetID, round, err)
		}
	}
	return nil
}
