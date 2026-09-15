package navigation

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/li41/astrahold-server/internal/gameplayworld"
	"github.com/li41/astrahold-server/internal/world"
)

func TestFirstContinentMovementEnvelope(t *testing.T) {
	loaded, err := gameplayworld.LoadFile(filepath.Join("..", "..", "worlds", "castle-sandbox", "gameplay.json"))
	if err != nil {
		t.Fatalf("LoadFile() error = %v", err)
	}
	nav, err := NewGameplayNavigator(loaded.Definition)
	if err != nil {
		t.Fatalf("NewGameplayNavigator() error = %v", err)
	}
	agent := Agent{Radius: loaded.Definition.Agent.Radius, MaxStepHeight: loaded.Definition.Agent.MaxStepHeight}

	start := world.Position{X: 2500, Y: 0, Z: 4000, Layer: 0}
	inside, err := nav.ResolveMove(start, world.Vec3{X: 50}, agent)
	if err != nil {
		t.Fatalf("move inside first-continent envelope error = %v", err)
	}
	if inside.X != 2550 || inside.Z != 4000 {
		t.Fatalf("inside move = %+v", inside)
	}

	if _, err := nav.ResolveMove(inside, world.Vec3{X: 100}, agent); !errors.Is(err, ErrBlocked) {
		t.Fatalf("move beyond maxX error = %v, want ErrBlocked", err)
	}
	if _, err := nav.ResolveMove(world.Position{X: -2500, Y: 0, Z: -850, Layer: 0}, world.Vec3{Z: -100}, agent); !errors.Is(err, ErrBlocked) {
		t.Fatalf("move beyond minZ error = %v, want ErrBlocked", err)
	}
}
