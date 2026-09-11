package loadlab

import (
	"math"
	"testing"
	"time"

	"github.com/li41/astrahold-server/internal/gameplayworld"
	"github.com/li41/astrahold-server/internal/session"
	"github.com/li41/astrahold-server/internal/world"
)

func TestCrowdPlayerFactoryUsesGroundOnly(t *testing.T) {
	loaded, err := gameplayworld.LoadFile("../../worlds/castle-sandbox/gameplay.json")
	if err != nil {
		t.Fatal(err)
	}
	factory, err := NewPlayerFactory(loaded.Definition, ScenarioCrowd, 100)
	if err != nil {
		t.Fatal(err)
	}
	for entityID := world.EntityID(1); entityID <= 100; entityID++ {
		player := factory(session.ID(entityID), entityID)
		position := player.Entity.Transform.Position
		if position.Layer != 0 || position.Y != 0 {
			t.Fatalf("entity %d position=%+v, want ground layer", entityID, position)
		}
		if !loaded.Definition.Surfaces[0].Bounds.Contains(position.X, position.Z) {
			t.Fatalf("entity %d position=%+v outside ground", entityID, position)
		}
	}
}

func TestTeleportChurnSwapsHalfOfEachCluster(t *testing.T) {
	loaded, err := gameplayworld.LoadFile("../../worlds/castle-sandbox/gameplay.json")
	if err != nil {
		t.Fatal(err)
	}
	const clients = 100
	factory, err := NewPlayerFactory(loaded.Definition, ScenarioTeleportChurn, clients)
	if err != nil {
		t.Fatal(err)
	}
	targets, err := TeleportChurnTargets(loaded.Definition, clients)
	if err != nil {
		t.Fatal(err)
	}
	if len(targets) != clients/2 {
		t.Fatalf("targets=%d want=%d", len(targets), clients/2)
	}

	groupSize := clients / 2
	moversPerGroup := groupSize / 2
	for local := 0; local < moversPerGroup; local++ {
		westID := world.EntityID(local + 1)
		eastID := world.EntityID(groupSize + local + 1)
		westInitial := factory(session.ID(westID), westID).Entity.Transform.Position
		eastInitial := factory(session.ID(eastID), eastID).Entity.Transform.Position
		if targets[westID] != eastInitial {
			t.Fatalf("west mover %d target=%+v want east slot=%+v", westID, targets[westID], eastInitial)
		}
		if targets[eastID] != westInitial {
			t.Fatalf("east mover %d target=%+v want west slot=%+v", eastID, targets[eastID], westInitial)
		}
	}

	westStationaryID := world.EntityID(moversPerGroup + 1)
	eastStationaryID := world.EntityID(groupSize + moversPerGroup + 1)
	if _, ok := targets[westStationaryID]; ok {
		t.Fatalf("west stationary entity %d unexpectedly moved", westStationaryID)
	}
	if _, ok := targets[eastStationaryID]; ok {
		t.Fatalf("east stationary entity %d unexpectedly moved", eastStationaryID)
	}

	west := factory(1, 1).Entity.Transform.Position
	east := factory(session.ID(groupSize+1), world.EntityID(groupSize+1)).Entity.Transform.Position
	dx := float64(east.X - west.X)
	dz := float64(east.Z - west.Z)
	if math.Sqrt(dx*dx+dz*dz) <= 64 {
		t.Fatalf("cluster sample distance must exceed AOI radius: west=%+v east=%+v", west, east)
	}
}

func TestTeleportChurnBoundsProtectMixedMovement(t *testing.T) {
	loaded, err := gameplayworld.LoadFile("../../worlds/castle-sandbox/gameplay.json")
	if err != nil {
		t.Fatal(err)
	}
	layout, err := buildLayout(loaded.Definition, ScenarioTeleportChurn)
	if err != nil {
		t.Fatal(err)
	}
	west, east := teleportChurnBounds(layout)
	ground := layout.ground.Bounds

	const groundInset = float32(10)
	for name, bounds := range map[string]gameplayworld.BoundsXZ{"west": west, "east": east} {
		if bounds.MinX-ground.MinX < groundInset || ground.MaxX-bounds.MaxX < groundInset || bounds.MinZ-ground.MinZ < groundInset || ground.MaxZ-bounds.MaxZ < groundInset {
			t.Fatalf("%s churn bounds=%+v must stay at least %.1fm inside ground=%+v", name, bounds, groundInset, ground)
		}
	}

	// Eight 250ms legs at 6m/s have a maximum path excursion below 4m. Add
	// agent radius/headroom and guarantee that this load fixture stays clear of
	// current authoritative movement blockers rather than measuring corrections.
	const clearance = float32(4.25)
	for name, bounds := range map[string]gameplayworld.BoundsXZ{"west": west, "east": east} {
		expanded := gameplayworld.BoundsXZ{
			MinX: bounds.MinX - clearance,
			MaxX: bounds.MaxX + clearance,
			MinZ: bounds.MinZ - clearance,
			MaxZ: bounds.MaxZ + clearance,
		}
		for _, blocker := range loaded.Definition.Blockers {
			if !blocker.Enabled || !blocker.BlocksMovement {
				continue
			}
			if boundsOverlap(expanded, blocker.Bounds) {
				t.Fatalf("%s churn movement envelope=%+v overlaps blocker %q bounds=%+v", name, expanded, blocker.ID, blocker.Bounds)
			}
		}
	}
}

func TestS3E9MixedMovementIsBounded(t *testing.T) {
	previous := s3e9MixedMovementEnabled
	s3e9MixedMovementEnabled = true
	defer func() { s3e9MixedMovementEnabled = previous }()

	const (
		entityID = world.EntityID(3) // mover, not a stationary hot-combat entity
		speed    = 6.0
	)
	const tick = 50 * time.Millisecond
	var x, z, maxDistance float64
	for elapsed := time.Duration(0); elapsed < 2*time.Second; elapsed += tick {
		dx, dz := MovementDirection(ScenarioTeleportChurn, entityID, elapsed)
		x += float64(dx) * speed * tick.Seconds()
		z += float64(dz) * speed * tick.Seconds()
		distance := math.Hypot(x, z)
		if distance > maxDistance {
			maxDistance = distance
		}
	}
	if math.Abs(x) > 0.001 || math.Abs(z) > 0.001 {
		t.Fatalf("mixed movement cycle drifted: x=%.6f z=%.6f", x, z)
	}
	if maxDistance <= 0 || maxDistance >= 4 {
		t.Fatalf("mixed movement max excursion=%.3fm, want >0 and <4m", maxDistance)
	}
}

func TestTeleportChurnRequiresQuarterablePopulation(t *testing.T) {
	loaded, err := gameplayworld.LoadFile("../../worlds/castle-sandbox/gameplay.json")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewPlayerFactory(loaded.Definition, ScenarioTeleportChurn, 10); err == nil {
		t.Fatal("expected teleport-churn population validation error")
	}
}

func TestRetiredCastleScenariosAreRejected(t *testing.T) {
	for _, scenario := range []string{"gate-zerg", "vertical-siege"} {
		if _, err := ParseScenario(scenario); err == nil {
			t.Fatalf("retired scenario %q unexpectedly accepted", scenario)
		}
	}
}

func TestMovementDirectionIsDeterministic(t *testing.T) {
	dx1, dz1 := MovementDirection(ScenarioDistributed, world.EntityID(42), 3*time.Second)
	dx2, dz2 := MovementDirection(ScenarioDistributed, world.EntityID(42), 3*time.Second)
	if dx1 != dx2 || dz1 != dz2 {
		t.Fatalf("direction is not deterministic: (%f,%f) != (%f,%f)", dx1, dz1, dx2, dz2)
	}

	dx, dz := MovementDirection(ScenarioCrowd, world.EntityID(1), 0)
	if dx != 0 || dz != -1 {
		t.Fatalf("crowd direction = (%f,%f), want (0,-1)", dx, dz)
	}

	dx, dz = MovementDirection(ScenarioTeleportChurn, world.EntityID(1), 0)
	if dx != 0 || dz != 0 {
		t.Fatalf("teleport-churn direction = (%f,%f), want (0,0)", dx, dz)
	}
}

func boundsOverlap(a, b gameplayworld.BoundsXZ) bool {
	return a.MinX < b.MaxX && a.MaxX > b.MinX && a.MinZ < b.MaxZ && a.MaxZ > b.MinZ
}
