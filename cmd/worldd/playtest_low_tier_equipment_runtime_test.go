package main

import (
	"testing"

	"github.com/li41/astrahold-server/internal/gameplayworld"
	"github.com/li41/astrahold-server/internal/movement"
	"github.com/li41/astrahold-server/internal/navigation"
	"github.com/li41/astrahold-server/internal/simulation"
	"github.com/li41/astrahold-server/internal/spatial"
	"github.com/li41/astrahold-server/internal/worldruntime"
)

func TestPlaytestLowTierEquipmentKitFitsDefaultRuntimeInventoryPolicy(t *testing.T) {
	config := worldruntime.DefaultConfig()
	if err := configurePlaytestLowTierEquipment(&config, worldNetworkBrowserWSDev, true); err != nil {
		t.Fatal(err)
	}

	definition := gameplayworld.Definition{
		SchemaVersion: gameplayworld.SchemaVersion,
		WorldID:       "playtest-low-tier-kit-test",
		Revision:      "r1",
		Units:         "meters",
		Agent:         gameplayworld.AgentDefaults{Radius: .35, Height: 1.8, MaxStepHeight: .5},
		Surfaces: []gameplayworld.Surface{{
			ID:     "ground",
			Layer:  0,
			Bounds: gameplayworld.BoundsXZ{MinX: -2, MaxX: 2, MinZ: -2, MaxZ: 2},
		}},
	}
	nav, err := navigation.NewGameplayNavigator(definition)
	if err != nil {
		t.Fatal(err)
	}
	sim := simulation.New(spatial.NewGrid(8), movement.NewService(nav, .1))

	// Runtime construction runs the same starter-inventory validation used by worldd startup.
	// A future item-weight or stack-policy change that makes this dev kit illegal must fail here;
	// the fixture must not compensate by raising production inventory limits.
	_ = worldruntime.New(sim, config)
}
