package replication

import (
	"testing"

	"github.com/li41/astrahold-server/internal/session"
	"github.com/li41/astrahold-server/internal/world"
)

func TestConvergenceStatsTracksBuildSpawnAndDespawn(t *testing.T) {
	svc := NewService()
	sid := session.ID(91)
	svc.Register(sid)

	stats := svc.ConvergenceStats()
	if stats.Views != 1 || stats.BuiltViews != 0 {
		t.Fatalf("initial stats=%+v", stats)
	}

	visible := []world.EntityState{
		{ID: 1, Kind: world.EntityPlayer},
		{ID: 2, Kind: world.EntityPlayer},
	}
	_ = svc.Build(sid, 1, 0, 1, visible)
	stats = svc.ConvergenceStats()
	if stats.BuiltViews != 1 || stats.DesiredRelationships != 2 || stats.KnownDesired != 0 || stats.PendingSpawns != 2 || stats.PendingDespawns != 0 {
		t.Fatalf("after build stats=%+v", stats)
	}

	svc.ConfirmSpawn(sid, 1)
	svc.ConfirmSpawn(sid, 2)
	stats = svc.ConvergenceStats()
	if stats.KnownDesired != 2 || stats.PendingSpawns != 0 || stats.PendingDespawns != 0 {
		t.Fatalf("after spawn stats=%+v", stats)
	}

	_ = svc.Build(sid, 1, 0, 2, visible[:1])
	stats = svc.ConvergenceStats()
	if stats.DesiredRelationships != 1 || stats.KnownDesired != 1 || stats.PendingDespawns != 1 {
		t.Fatalf("after departure stats=%+v", stats)
	}
}
