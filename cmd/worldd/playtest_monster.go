package main

import (
	"github.com/li41/astrahold-server/internal/gameplayworld"
	"github.com/li41/astrahold-server/internal/loot"
	"github.com/li41/astrahold-server/internal/map1loot"
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
	return newPlaytestMonsterSpawnAt(agent, playtestMonsterHome())
}

func newPlaytestMonsterSpawnAt(agent gameplayworld.AgentDefaults, position world.Position) worldruntime.SpawnEntityRequest {
	definition := playtestMonsterDefinition()
	request, err := worldruntime.NewMonsterSpawnEntityRequest(definition, world.EntityState{
		ID:          playtestMonsterEntityID,
		Kind:        world.EntityMonster,
		ArchetypeID: definition.ArchetypeID,
		Transform:   world.Transform{Position: position},
	}, agent.Radius, agent.MaxStepHeight)
	if err != nil {
		panic(err)
	}
	return request
}

func newPlaytestMonsterAIConfig() worldruntime.AutonomousMeleeAgentConfig {
	return newPlaytestMonsterAIConfigAt(playtestMonsterHome(), playtestMonsterIdlePatrol())
}

func newPlaytestMonsterAIConfigAt(home world.Position, patrol []world.Position) worldruntime.AutonomousMeleeAgentConfig {
	definition := playtestMonsterDefinition()
	return worldruntime.AutonomousMeleeAgentConfig{
		EntityID:              playtestMonsterEntityID,
		Home:                  home,
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
		IdlePatrol:            append([]world.Position(nil), patrol...),
		PatrolTolerance:       playtestMonsterPatrolToleranceMeters,
	}
}

func newPlaytestMonsterLifecycleConfig(agent gameplayworld.AgentDefaults, tickRate int) worldruntime.MonsterLifecycleConfig {
	return newPlaytestMonsterLifecycleConfigAt(agent, tickRate, playtestMonsterHome())
}

func newPlaytestMonsterLifecycleConfigAt(agent gameplayworld.AgentDefaults, tickRate int, position world.Position) worldruntime.MonsterLifecycleConfig {
	definition := playtestMonsterDefinition()
	return worldruntime.MonsterLifecycleConfig{
		Spawn:             newPlaytestMonsterSpawnAt(agent, position),
		CorpseHoldTicks:   uint64(definition.CorpseHoldSeconds) * uint64(tickRate),
		RespawnDelayTicks: uint64(definition.RespawnDelaySeconds) * uint64(tickRate),
	}
}

type playtestGroundResolver interface {
	ResolveGroundPosition(world.Position) (world.Position, error)
}

type groundedPlaytestMonsterFixture struct {
	Spawn     worldruntime.SpawnEntityRequest
	AI        worldruntime.AutonomousMeleeAgentConfig
	Lifecycle worldruntime.MonsterLifecycleConfig
}

func newGroundedPlaytestMonsterFixture(resolver playtestGroundResolver, agent gameplayworld.AgentDefaults, tickRate int) (groundedPlaytestMonsterFixture, error) {
	home, err := resolver.ResolveGroundPosition(playtestMonsterHome())
	if err != nil {
		return groundedPlaytestMonsterFixture{}, err
	}
	patrol := playtestMonsterIdlePatrol()
	for i := range patrol {
		patrol[i], err = resolver.ResolveGroundPosition(patrol[i])
		if err != nil {
			return groundedPlaytestMonsterFixture{}, err
		}
	}
	return groundedPlaytestMonsterFixture{
		Spawn:     newPlaytestMonsterSpawnAt(agent, home),
		AI:        newPlaytestMonsterAIConfigAt(home, patrol),
		Lifecycle: newPlaytestMonsterLifecycleConfigAt(agent, tickRate, home),
	}, nil
}

func newPlaytestMonsterLootCatalog() *loot.Catalog {
	catalog, err := map1loot.Default()
	if err != nil {
		panic(err)
	}
	return catalog
}
