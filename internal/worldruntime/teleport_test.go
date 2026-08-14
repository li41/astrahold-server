package worldruntime

import (
	"testing"
	"time"

	"github.com/li41/astrahold-server/internal/spatial"
	"github.com/li41/astrahold-server/internal/world"
)

func TestTeleportRunsOnOwnerThreadAndMovesSpatialMembership(t *testing.T) {
	rt, sim, _ := makeRuntime(t)
	first := rt.Step(1, 50*time.Millisecond)
	if len(first.CommandErrors) != 0 {
		t.Fatalf("unexpected initial command errors: %#v", first.CommandErrors)
	}

	target := world.Position{X: 50, Y: 0, Z: 40, Layer: 0}
	if err := rt.EnqueueTeleport(1, target); err != nil {
		t.Fatal(err)
	}
	second := rt.Step(2, 50*time.Millisecond)
	if len(second.CommandErrors) != 0 || len(second.TickErrors) != 0 {
		t.Fatalf("unexpected teleport errors: %#v", second)
	}

	entity, ok := sim.Entity(1)
	if !ok || entity.Transform.Position != target {
		t.Fatalf("teleported position=%+v ok=%v want=%+v", entity.Transform.Position, ok, target)
	}
	if got := sim.QueryAOI(target, 1, spatial.QueryOptions{}); len(got) != 1 || got[0].ID != 1 {
		t.Fatalf("target AOI=%+v want entity 1", got)
	}
	if got := sim.QueryAOI(world.Position{X: 0.3, Layer: 0}, 1, spatial.QueryOptions{}); len(got) != 0 {
		t.Fatalf("old AOI still contains teleported entity: %+v", got)
	}
}
