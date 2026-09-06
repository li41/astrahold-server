package main

import (
	"fmt"

	"github.com/li41/astrahold-server/internal/gameplayworld"
	"github.com/li41/astrahold-server/internal/netadapter/tcpudp"
	"github.com/li41/astrahold-server/internal/respawnpolicy"
	"github.com/li41/astrahold-server/internal/session"
	"github.com/li41/astrahold-server/internal/world"
)

const defaultPlayerGroundSpeedMetersPerSecond float32 = 4.2

func freshPlayerSpawn(definition respawnpolicy.Definition) (respawnpolicy.SpawnPoint, error) {
	var spawnID string
	for _, rule := range definition.Contexts {
		if rule.Context == respawnpolicy.DeathContextPvE {
			spawnID = rule.DefaultSpawnPoint
			break
		}
	}
	if spawnID == "" {
		return respawnpolicy.SpawnPoint{}, fmt.Errorf("worldd: pve default fresh spawn is not configured")
	}
	for _, point := range definition.SpawnPoints {
		if point.ID == spawnID {
			return point, nil
		}
	}
	return respawnpolicy.SpawnPoint{}, fmt.Errorf("worldd: pve default fresh spawn %q is missing", spawnID)
}

func newWorldPlayerFactory(spawn respawnpolicy.SpawnPoint, agent gameplayworld.AgentDefaults) tcpudp.PlayerFactory {
	return func(_ session.ID, entityID world.EntityID) tcpudp.PlayerSpec {
		return tcpudp.PlayerSpec{
			Entity: world.EntityState{
				ID:        entityID,
				Kind:      world.EntityPlayer,
				Transform: world.Transform{Position: spawn.Position()},
			},
			// Normal on-foot movement is Server-authoritative. Unreal/Click-to-Move both
			// consume this same world speed rather than inventing a presentation-only rate.
			Speed:         defaultPlayerGroundSpeedMetersPerSecond,
			Radius:        agent.Radius,
			MaxStepHeight: agent.MaxStepHeight,
			AOIRadius:     64,
		}
	}
}
