package worldruntime

import (
	"errors"
	"testing"
	"time"

	"github.com/li41/astrahold-server/internal/characteridentity"
	"github.com/li41/astrahold-server/internal/deathoutcome"
	"github.com/li41/astrahold-server/internal/movement"
	"github.com/li41/astrahold-server/internal/navigation"
	"github.com/li41/astrahold-server/internal/respawnpolicy"
	"github.com/li41/astrahold-server/internal/session"
	"github.com/li41/astrahold-server/internal/simulation"
	"github.com/li41/astrahold-server/internal/spatial"
	"github.com/li41/astrahold-server/internal/world"
)

func TestTrustedCharacterIdentityCannotBeActiveOnTwoEntities(t *testing.T) {
	rt, identity := newIdentityRuntime(t)
	joinIdentityPlayer(t, rt, 1, 1, identity)
	first := rt.Step(1, 50*time.Millisecond)
	if len(first.CommandErrors) != 0 {
		t.Fatalf("first errors=%#v", first.CommandErrors)
	}

	joinIdentityPlayer(t, rt, 2, 2, identity)
	second := rt.Step(2, 50*time.Millisecond)
	if len(second.CommandErrors) != 1 || !errors.Is(second.CommandErrors[0].Err, ErrCharacterIdentityActive) {
		t.Fatalf("second errors=%#v", second.CommandErrors)
	}
	if _, ok := rt.world.Entity(2); ok {
		t.Fatal("conflicting entity must not spawn")
	}
}

func TestCharacterIdentitySurvivesHistoryAcrossEntityIDReuse(t *testing.T) {
	rt, identity := newIdentityRuntime(t)
	outbox, err := deathoutcome.NewOutbox(8)
	if err != nil {
		t.Fatal(err)
	}
	rt.deathOutbox = outbox

	joinIdentityPlayer(t, rt, 1, 1, identity)
	if report := rt.Step(1, 50*time.Millisecond); len(report.CommandErrors) != 0 {
		t.Fatalf("join1 errors=%#v", report.CommandErrors)
	}
	firstReport := StepReport{Tick: 2}
	first, err := rt.beginDeathOutcome(1, 2, respawnpolicy.DeathContextPvE, &firstReport)
	if err != nil {
		t.Fatal(err)
	}
	rt.enqueueDeathOutcomeEvent(first, respawnpolicy.Scheduled{}, false, deathPenaltyOutcome{}, &firstReport)
	if len(firstReport.CommandErrors) != 0 {
		t.Fatalf("first death errors=%#v", firstReport.CommandErrors)
	}

	if err := rt.EnqueueLeave(1); err != nil {
		t.Fatal(err)
	}
	if report := rt.Step(3, 50*time.Millisecond); len(report.CommandErrors) != 0 {
		t.Fatalf("leave errors=%#v", report.CommandErrors)
	}

	joinIdentityPlayer(t, rt, 2, 2, identity)
	if report := rt.Step(4, 50*time.Millisecond); len(report.CommandErrors) != 0 {
		t.Fatalf("join2 errors=%#v", report.CommandErrors)
	}
	secondReport := StepReport{Tick: 5}
	second, err := rt.beginDeathOutcome(2, 5, respawnpolicy.DeathContextPvP, &secondReport)
	if err != nil {
		t.Fatal(err)
	}
	rt.enqueueDeathOutcomeEvent(second, respawnpolicy.Scheduled{}, false, deathPenaltyOutcome{}, &secondReport)
	if len(secondReport.CommandErrors) != 0 {
		t.Fatalf("second death errors=%#v", secondReport.CommandErrors)
	}

	pending := outbox.Pending(0)
	if len(pending) != 2 {
		t.Fatalf("pending=%#v", pending)
	}
	if pending[0].CharacterIdentity() != identity || pending[1].CharacterIdentity() != identity {
		t.Fatalf("identity history=%#v", pending)
	}
	if pending[0].EntityID != 1 || pending[1].EntityID != 2 || pending[0].DefeatRevision != 1 || pending[1].DefeatRevision != 1 {
		t.Fatalf("incarnation truth=%#v", pending)
	}
	if pending[0].EventID == pending[1].EventID {
		t.Fatalf("event ids must differ: %#v", pending)
	}
}

func TestUnregisterDoesNotReleaseWorldCharacterIdentity(t *testing.T) {
	rt, identity := newIdentityRuntime(t)
	entity := world.EntityState{ID: 1, Kind: world.EntityPlayer, Transform: world.Transform{Position: world.Position{Layer: 0}}}
	if err := rt.world.Spawn(entity, 6, 0.35, 0.5); err != nil {
		t.Fatal(err)
	}
	conn := session.NewQueueConnection(8, 8)
	s, err := session.NewWithCharacterIdentity(1, 1, identity, 20, conn)
	if err != nil {
		t.Fatal(err)
	}
	if err := rt.EnqueueRegister(s); err != nil {
		t.Fatal(err)
	}
	if report := rt.Step(1, 50*time.Millisecond); len(report.CommandErrors) != 0 {
		t.Fatalf("register errors=%#v", report.CommandErrors)
	}
	if err := rt.EnqueueUnregister(1); err != nil {
		t.Fatal(err)
	}
	if report := rt.Step(2, 50*time.Millisecond); len(report.CommandErrors) != 0 {
		t.Fatalf("unregister errors=%#v", report.CommandErrors)
	}
	binding, ok := rt.characterIdentities.binding(1)
	if !ok || binding != identity {
		t.Fatalf("binding=%#v ok=%v", binding, ok)
	}
}

func newIdentityRuntime(t *testing.T) (*Runtime, characteridentity.Binding) {
	t.Helper()
	nav := navigation.Plane{MinX: -100, MaxX: 100, MinZ: -100, MaxZ: 100, Layer: 0}
	sim := simulation.New(spatial.NewGrid(16), movement.NewService(nav, 0.1))
	rt := New(sim, DefaultConfig())
	identity, err := characteridentity.NewTrusted("character:alpha")
	if err != nil {
		t.Fatal(err)
	}
	return rt, identity
}

func joinIdentityPlayer(t *testing.T, rt *Runtime, sid session.ID, eid world.EntityID, identity characteridentity.Binding) {
	t.Helper()
	conn := session.NewQueueConnection(8, 8)
	s, err := session.NewWithCharacterIdentity(sid, eid, identity, 20, conn)
	if err != nil {
		t.Fatal(err)
	}
	if err := rt.EnqueueJoin(JoinRequest{
		Session: s,
		Entity: world.EntityState{
			ID:        eid,
			Kind:      world.EntityPlayer,
			Transform: world.Transform{Position: world.Position{Layer: 0}},
		},
		Speed: 6, Radius: 0.35, MaxStepHeight: 0.5,
	}); err != nil {
		t.Fatal(err)
	}
}
