package main

import (
	"github.com/li41/astrahold-server/internal/gameplayworld"
	"github.com/li41/astrahold-server/internal/world"
	"github.com/li41/astrahold-server/internal/worldruntime"
)

const (
	playtestMonsterEntityID    world.EntityID = 9001
	playtestMonsterArchetypeID                = "wolf-gray-01"
)

func newPlaytestMonsterSpawn(agent gameplayworld.AgentDefaults) worldruntime.SpawnEntityRequest {
	return worldruntime.SpawnEntityRequest{
		Entity: world.EntityState{
			ID:          playtestMonsterEntityID,
			Kind:        world.EntityMonster,
			ArchetypeID: playtestMonsterArchetypeID,
			Transform:   world.Transform{Position: world.Position{X: 2, Y: 0, Z: -35, Layer: 0}},
		},
		Speed:         4,
		Radius:        agent.Radius,
		MaxStepHeight: agent.MaxStepHeight,
		HP:            200,
		MaxHP:         200,
	}
}
