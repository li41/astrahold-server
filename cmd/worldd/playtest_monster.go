package main

import (
	"github.com/li41/astrahold-server/internal/gameplayworld"
	"github.com/li41/astrahold-server/internal/loot"
	"github.com/li41/astrahold-server/internal/monstercatalog"
	"github.com/li41/astrahold-server/internal/world"
	"github.com/li41/astrahold-server/internal/worldruntime"
)

const (
	playtestMonsterEntityID                         world.EntityID = 9001
	playtestMonsterArchetypeID                                     = monstercatalog.MonsterGrayWolf
	playtestMonsterActionID                                        = monstercatalog.BasicMeleeActionID
	playtestMonsterDropArchetypeID                                 = "item_gray_wolf_pelt"
	playtestMonsterDropChanceBasisPoints              uint16       = 4_500
	playtestMonsterSpawnX                              float32      = 2
	playtestMonsterSpawnZ                              float32      = -35
	playtestMonsterPatrolToleranceMeters               float32      = 0.2
	playtestMonsterEncounterGroupID                                 = "playtest_emberwatch_gray_wolf"
)

func playtestMonsterDefinition() monstercatalog.Definition {
	definition, ok := monstercatalog.Map1().Resolve(playtestMonsterArchetypeID)
	if !ok {
		panic("worldd: map1 gray wolf definition missing")
	}
	return definition
}

func playtestMonsterHome() world.Position {
	return world.Position{X: playtestMonsterSpawnX, Z: playtestMonsterSpawnZ, Layer: 0}
}

func playtestMonsterIdlePatrol() []world.Position {
	home := playtestMonsterHome()
	return []world.Position{
		{X: home.X + 1.5, Y: home.Y, Z: home.Z, Layer: home.Layer},
		{X: home.X + 0.5, Y: home.Y, Z: home.Z + 1.4, Layer: home.Layer},
		{X: home.X - 1.2, Y: home.Y, Z: home.Z + 0.8, Layer: home.Layer},
		home,
	}
}

func newPlaytestMonsterSpawn(agent gameplayworld.AgentDefaults) worldruntime.SpawnEntityRequest {
	definition := playtestMonsterDefinition()
	request, err := worldruntime.NewMonsterSpawnEntityRequest(definition, world.EntityState{
		ID:          playtestMonsterEntityID,
		Kind:        world.EntityMonster,
		ArchetypeID: definition.ArchetypeID,
		Transform:   world.Transform{Position: playtestMonsterHome()},
	}, agent.Radius, agent.MaxStepHeight)
	if err != nil {
		panic(err)
	}
	return request
}

func newPlaytestMonsterAIConfig() worldruntime.AutonomousMeleeAgentConfig {
	definition := playtestMonsterDefinition()
	return worldruntime.AutonomousMeleeAgentConfig{
		EntityID:              playtestMonsterEntityID,
		Home:                  playtestMonsterHome(),
		ActionID:              definition.MeleeActionID,
		AggroRange:            definition.AggroRange,
		LeashRange:            definition.LeashRange,
		AttackRange:           definition.AttackRange,
		AttackIntervalSeconds: definition.AttackIntervalSeconds,
		AggroMode:             definition.AggroMode,
		EncounterGroupID:      playtestMonsterEncounterGroupID,
		AssistFamilyID:        definition.AssistFamilyID,
		AssistPolicy:          definition.AssistPolicy,
		AssistRadius:          definition.AssistRadius,
		MaxAssist:             definition.MaxAssist,
		ReturnTolerance:       0.25,
		IdlePatrol:            playtestMonsterIdlePatrol(),
		PatrolTolerance:       playtestMonsterPatrolToleranceMeters,
	}
}

func newPlaytestMonsterLifecycleConfig(agent gameplayworld.AgentDefaults, tickRate int) worldruntime.MonsterLifecycleConfig {
	definition := playtestMonsterDefinition()
	return worldruntime.MonsterLifecycleConfig{
		Spawn:             newPlaytestMonsterSpawn(agent),
		CorpseHoldTicks:   uint64(definition.CorpseHoldSeconds) * uint64(tickRate),
		RespawnDelayTicks: uint64(definition.RespawnDelaySeconds) * uint64(tickRate),
	}
}

func newPlaytestMonsterLootCatalog() *loot.Catalog {
	catalog, err := loot.New(loot.Definition{
		Revision: "playtest-monster-loot-v3",
		Tables: []loot.Table{{
			SourceArchetypeID: playtestMonsterArchetypeID,
			Drops: []loot.Drop{{
				ItemArchetypeID:   playtestMonsterDropArchetypeID,
				ChanceBasisPoints: playtestMonsterDropChanceBasisPoints,
			}},
		}},
	})
	if err != nil {
		panic(err)
	}
	return catalog
}
