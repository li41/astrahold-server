package replication

import (
	"testing"

	"github.com/li41/astrahold-server/internal/session"
	"github.com/li41/astrahold-server/internal/world"
)

func TestRespawnMembershipDistinguishesDesiredFromKnown(t *testing.T) {
	service := NewService()
	const sessionID session.ID = 1
	service.Register(sessionID)

	visible := []world.EntityState{{ID: 1}, {ID: 2}}
	service.Build(sessionID, 1, 0, 1, visible)
	service.ConfirmSpawn(sessionID, 2)
	if !service.Wants(sessionID, 2) {
		t.Fatal("entity 2 should be desired in initial AOI")
	}
	if service.HasKnownOutsideDesired(2) {
		t.Fatal("known entity should not be stale while still desired")
	}

	service.Build(sessionID, 1, 0, 2, []world.EntityState{{ID: 1}})
	if service.Wants(sessionID, 2) {
		t.Fatal("entity 2 should no longer be desired after AOI rebuild")
	}
	if !service.Knows(sessionID, 2) {
		t.Fatal("known truth must remain until Despawn delivery confirms")
	}
	if !service.HasKnownOutsideDesired(2) {
		t.Fatal("known-but-not-desired entity should be reported as stale knowledge")
	}

	service.ConfirmDespawn(sessionID, 2)
	if service.HasKnownOutsideDesired(2) {
		t.Fatal("stale knowledge should clear after Despawn confirmation")
	}
}
