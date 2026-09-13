package loadlab

import (
	"math"
	"testing"
	"time"

	"github.com/li41/astrahold-server/internal/world"
)

func TestS3E9MixedPulseUsesPostTeleportCluster(t *testing.T) {
	const clients = 500
	const diagonal = float32(0.70710677)

	// Entity 3 starts in west group and is part of the authoritative swapped half,
	// so after round-1 teleport it is on east and must first move inward (-x,-z).
	dx, dz := s3e9MixedPulseDirection(world.EntityID(3), clients, 0)
	if dx != -diagonal || dz != -diagonal {
		t.Fatalf("teleported west mover direction=(%f,%f), want inward east (-diag,-diag)", dx, dz)
	}

	// Entity 251 starts in east group, belongs to the moving cohort, and is swapped to west.
	dx, dz = s3e9MixedPulseDirection(world.EntityID(251), clients, 0)
	if dx != diagonal || dz != diagonal {
		t.Fatalf("teleported east mover direction=(%f,%f), want inward west (+diag,+diag)", dx, dz)
	}

	// Entity 127 is outside the swapped half and remains west.
	dx, dz = s3e9MixedPulseDirection(world.EntityID(127), clients, 0)
	if dx != diagonal || dz != diagonal {
		t.Fatalf("retained west mover direction=(%f,%f), want inward west (+diag,+diag)", dx, dz)
	}

	// Hot-combat entities remain stationary so movement cannot invalidate the combat fixture.
	dx, dz = s3e9MixedPulseDirection(world.EntityID(1), clients, 0)
	if dx != 0 || dz != 0 {
		t.Fatalf("stationary hot entity direction=(%f,%f), want zero", dx, dz)
	}
}

func TestS3E9MixedPulseReturnsToOrigin(t *testing.T) {
	const (
		clients  = 500
		entityID = world.EntityID(3)
		speed    = 6.0
		tick     = 50 * time.Millisecond
	)
	var x, z, maxDistance float64
	for elapsed := time.Duration(0); elapsed < 4*time.Second; elapsed += tick {
		dx, dz := s3e9MixedPulseDirection(entityID, clients, elapsed)
		x += float64(dx) * speed * tick.Seconds()
		z += float64(dz) * speed * tick.Seconds()
		if distance := math.Hypot(x, z); distance > maxDistance {
			maxDistance = distance
		}
	}
	if math.Abs(x) > 0.001 || math.Abs(z) > 0.001 {
		t.Fatalf("mixed pulse drifted after full cycle: x=%.6f z=%.6f", x, z)
	}
	if maxDistance < 11.5 || maxDistance > 12.5 {
		t.Fatalf("mixed pulse max excursion=%.3fm, want about 12m", maxDistance)
	}
}

func TestS3E9MixedPulseRejectsInvalidPopulation(t *testing.T) {
	dx, dz := s3e9MixedPulseDirection(world.EntityID(3), 10, 0)
	if dx != 0 || dz != 0 {
		t.Fatalf("invalid population direction=(%f,%f), want zero", dx, dz)
	}
}
