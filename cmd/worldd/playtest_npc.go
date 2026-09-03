package main

import (
	"github.com/li41/astrahold-server/internal/gameplayworld"
	"github.com/li41/astrahold-server/internal/world"
	"github.com/li41/astrahold-server/internal/worldruntime"
)

const (
	playtestNPCEntityID    world.EntityID = 7001
	playtestNPCArchetypeID                = "npc_emberwatch_warden"
)

func newPlaytestNPCSpawn(agent gameplayworld.AgentDefaults) worldruntime.SpawnEntityRequest {
	return worldruntime.SpawnEntityRequest{
		Entity: world.EntityState{
			ID:          playtestNPCEntityID,
			Kind:        world.EntityNPC,
			ArchetypeID: playtestNPCArchetypeID,
			Transform:   world.Transform{Position: world.Position{X: -2, Y: 0, Z: -35, Layer: 0}},
		},
		Speed:         0,
		Radius:        agent.Radius,
		MaxStepHeight: agent.MaxStepHeight,
		HP:            100,
		MaxHP:         100,
	}
}
