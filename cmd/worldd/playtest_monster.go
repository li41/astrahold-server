package main

import (
	"github.com/li41/astrahold-server/internal/gameplayworld"
	"github.com/li41/astrahold-server/internal/world"
	"github.com/li41/astrahold-server/internal/worldruntime"
)

const (
	playtestMonsterEntityID            world.EntityID = 9001
	playtestMonsterArchetypeID                        = "wolf-gray-01"
	playtestMonsterActionID                           = "wolf-bite"
	playtestMonsterSpawnX               float32        = 2
	playtestMonsterSpawnZ               float32        = -35
	playtestMonsterCorpseHoldSeconds                   = 2
	playtestMonsterRespawnDelaySeconds                 = 8
)

func playtestMonsterHome() world.Position {
	return world.Position{X: playtestMonsterSpawnX, Z: playtestMonsterSpawnZ, Layer: 0}
}

func newPlaytestMonsterSpawn(agent gameplayworld.AgentDefaults) worldruntime.SpawnEntityRequest {
	return worldruntime.SpawnEntityRequest{
		Entity: world.EntityState{
			ID:          playtestMonsterEntityID,
			Kind:        world.EntityMonster,
			ArchetypeID: playtestMonsterArchetypeID,
			Transform:   world.Transform{Position: playtestMonsterHome()},
		},
		Speed:         4,
		Radius:        agent.Radius,
		MaxStepHeight: agent.MaxStepHeight,
		HP:            200,
		MaxHP:         200,
	}
}

func newPlaytestMonsterAIConfig() worldruntime.AutonomousMeleeAgentConfig {
	return worldruntime.AutonomousMeleeAgentConfig{
		EntityID:        playtestMonsterEntityID,
		Home:            playtestMonsterHome(),
		ActionID:        playtestMonsterActionID,
		AggroRange:      9,
		LeashRange:      16,
		AttackRange:     1.75,
		ReturnTolerance: 0.25,
	}
}

func newPlaytestMonsterLifecycleConfig(agent gameplayworld.AgentDefaults, tickRate int) worldruntime.MonsterLifecycleConfig {
	return worldruntime.MonsterLifecycleConfig{
		Spawn:             newPlaytestMonsterSpawn(agent),
		CorpseHoldTicks:   uint64(playtestMonsterCorpseHoldSeconds * tickRate),
		RespawnDelayTicks: uint64(playtestMonsterRespawnDelaySeconds * tickRate),
	}
}
