package navigation

import (
	"math"
	"testing"

	"github.com/li41/astrahold-server/internal/gameplayworld"
	"github.com/li41/astrahold-server/internal/respawnpolicy"
	"github.com/li41/astrahold-server/internal/world"
)

// This is a product-route regression, not just a short movement unit test. A fresh PvE
// character in the canonical castle-sandbox must be able to travel from the Server-owned
// default spawn to the closed main-gate approach using only authoritative navigation.
func TestCastleSandboxFreshSpawnCanReachMainGateApproach(t *testing.T) {
	gameplay, err := gameplayworld.LoadFile("../../worlds/castle-sandbox/gameplay.json")
	if err != nil {
		t.Fatal(err)
	}
	respawn, err := respawnpolicy.LoadFile("../../config/respawn-policy.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := respawnpolicy.ValidateAgainstWorld(respawn.Definition, gameplay.Definition); err != nil {
		t.Fatalf("respawn/world validation: %v", err)
	}

	var defaultSpawnID string
	for _, rule := range respawn.Definition.Contexts {
		if rule.Context == respawnpolicy.DeathContextPvE {
			defaultSpawnID = rule.DefaultSpawnPoint
			break
		}
	}
	if defaultSpawnID == "" {
		t.Fatal("missing PvE default spawn point")
	}

	var spawn respawnpolicy.SpawnPoint
	foundSpawn := false
	for _, point := range respawn.Definition.SpawnPoints {
		if point.ID == defaultSpawnID {
			spawn = point
			foundSpawn = true
			break
		}
	}
	if !foundSpawn {
		t.Fatalf("PvE default spawn %q missing", defaultSpawnID)
	}

	nav, err := NewGameplayNavigator(gameplay.Definition)
	if err != nil {
		t.Fatal(err)
	}
	agent := Agent{
		Radius:        gameplay.Definition.Agent.Radius,
		MaxStepHeight: gameplay.Definition.Agent.MaxStepHeight,
	}

	start := spawn.Position()
	pos := start
	const approachZ float32 = 8.0
	for stepIndex := 0; stepIndex < 200 && pos.Z < approachZ-0.001; stepIndex++ {
		stepZ := float32(0.5)
		if remaining := approachZ - pos.Z; remaining < stepZ {
			stepZ = remaining
		}
		next, moveErr := nav.ResolveMove(pos, world.Vec3{Z: stepZ}, agent)
		if moveErr != nil {
			t.Fatalf("fresh-spawn route blocked step=%d pos=%+v err=%v", stepIndex, pos, moveErr)
		}
		if next.Z <= pos.Z+0.001 {
			t.Fatalf("fresh-spawn route made no forward progress step=%d pos=%+v next=%+v", stepIndex, pos, next)
		}
		pos = next
	}

	if pos.Layer != 0 {
		t.Fatalf("gate approach layer=%d, want ground layer 0", pos.Layer)
	}
	if math.Abs(float64(pos.Z-approachZ)) > 0.01 {
		t.Fatalf("gate approach z=%g, want %g", pos.Z, approachZ)
	}
	if traveled := pos.Z - start.Z; traveled < 40 {
		t.Fatalf("fresh-spawn route traveled only %gm; expected full castle approach from spawn=%+v to pos=%+v", traveled, start, pos)
	}
}
