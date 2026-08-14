package worldruntime

import (
	"testing"

	"github.com/li41/astrahold-server/internal/session"
	"github.com/li41/astrahold-server/internal/world"
)

func TestInitialBootstrapSnapshotBarrierIsStartupOnly(t *testing.T) {
	r := &Runtime{sessionVitalsPending: make(map[session.ID]map[world.EntityID]struct{})}
	if r.suppressInitialBootstrapSnapshots() {
		t.Fatal("unseen startup should not suppress before lifecycle work exists")
	}

	r.observeInitialBootstrapSnapshot(1, true)
	if !r.suppressInitialBootstrapSnapshots() {
		t.Fatal("first lifecycle work should activate startup suppression")
	}

	r.sessionVitalsPending[1] = map[world.EntityID]struct{}{1: {}}
	r.observeInitialBootstrapSnapshot(1, false)
	if !r.suppressInitialBootstrapSnapshots() {
		t.Fatal("pending Initial Vitals should keep startup suppression active")
	}

	delete(r.sessionVitalsPending, 1)
	r.observeInitialBootstrapSnapshot(1, false)
	if r.suppressInitialBootstrapSnapshots() || r.initialBootstrapState != initialBootstrapComplete {
		t.Fatal("drained startup should permanently complete suppression state")
	}

	// Late join/lifecycle work不能重新啟用 initial-only barrier。
	r.observeInitialBootstrapSnapshot(2, true)
	if r.suppressInitialBootstrapSnapshots() {
		t.Fatal("late lifecycle work reactivated startup suppression")
	}
}

func TestInitialBootstrapSnapshotBarrierStopsIfChurnStarts(t *testing.T) {
	r := &Runtime{
		sessionVitalsPending: make(map[session.ID]map[world.EntityID]struct{}),
		initialBootstrapState: initialBootstrapActive,
		lifecycleChurnActive:  true,
	}
	r.observeInitialBootstrapSnapshot(4, true)
	if r.initialBootstrapState != initialBootstrapComplete || r.suppressInitialBootstrapSnapshots() {
		t.Fatal("churn should permanently end initial-only suppression")
	}
}
