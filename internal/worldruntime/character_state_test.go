package worldruntime

import (
	"errors"
	"testing"
	"time"

	"github.com/li41/astrahold-server/internal/characteridentity"
	"github.com/li41/astrahold-server/internal/characterstate"
	"github.com/li41/astrahold-server/internal/movement"
	"github.com/li41/astrahold-server/internal/navigation"
	"github.com/li41/astrahold-server/internal/session"
	"github.com/li41/astrahold-server/internal/simulation"
	"github.com/li41/astrahold-server/internal/spatial"
	"github.com/li41/astrahold-server/internal/world"
)

const characterStateTestSHA = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

var characterStateTestWorld = characterstate.WorldRef{
	WorldID: "castle-sandbox", Revision: "s3d-001", GameplaySHA256: characterStateTestSHA,
}

func TestTrustedLeaveEnqueuesAuthoritativeCharacterState(t *testing.T) {
	outbox, err := characterstate.NewOutbox(4)
	if err != nil {
		t.Fatal(err)
	}
	rt := makeCharacterStateRuntime(t, outbox)
	identity, err := characteridentity.NewTrusted("character:alpha")
	if err != nil {
		t.Fatal(err)
	}
	conn := session.NewQueueConnection(32, 32)
	s, err := session.NewWithCharacterIdentity(1, 1, identity, 64, conn)
	if err != nil {
		t.Fatal(err)
	}
	if err := rt.EnqueueRegister(s); err != nil {
		t.Fatal(err)
	}
	if report := rt.Step(1, 50*time.Millisecond); len(report.CommandErrors) != 0 {
		t.Fatalf("register=%#v", report.CommandErrors)
	}
	if _, err := rt.characters.ApplyDamage(1, 125); err != nil {
		t.Fatal(err)
	}

	if err := rt.EnqueueLeave(1); err != nil {
		t.Fatal(err)
	}
	report := rt.Step(2, 50*time.Millisecond)
	if len(report.CommandErrors) != 0 || report.Metrics.CharacterStateSaveIntentsEnqueued != 1 || report.Metrics.CharacterStateSaveIntentFailures != 0 {
		t.Fatalf("leave=%#v", report)
	}
	pending := outbox.Pending(1)
	if len(pending) != 1 {
		t.Fatalf("pending=%#v", pending)
	}
	intent := pending[0]
	wantPosition := world.Position{X: 7, Y: 2, Z: -3, Layer: 4}
	if intent.Identity != identity || intent.Snapshot.World != characterStateTestWorld || intent.Snapshot.HP != 875 || intent.Snapshot.MaxHP != 1000 || intent.Snapshot.Defeated || intent.Snapshot.Position != wantPosition || intent.Snapshot.Yaw != 1.25 {
		t.Fatalf("intent=%#v", intent)
	}
	if _, ok := rt.world.Entity(1); ok {
		t.Fatal("leave did not remove world entity")
	}
	if _, ok := rt.characters.State(1); ok {
		t.Fatal("leave did not remove character state")
	}
}

func TestEphemeralLeaveDoesNotEnterDurableSaveOutbox(t *testing.T) {
	outbox, _ := characterstate.NewOutbox(2)
	rt := makeCharacterStateRuntime(t, outbox)
	conn := session.NewQueueConnection(32, 32)
	s, err := session.New(1, 1, 64, conn)
	if err != nil {
		t.Fatal(err)
	}
	if err := rt.EnqueueRegister(s); err != nil {
		t.Fatal(err)
	}
	rt.Step(1, 50*time.Millisecond)
	if err := rt.EnqueueLeave(1); err != nil {
		t.Fatal(err)
	}
	report := rt.Step(2, 50*time.Millisecond)
	if len(report.CommandErrors) != 0 || report.Metrics.CharacterStateSaveIntentsEnqueued != 0 || report.Metrics.CharacterStateSaveIntentFailures != 0 || outbox.Depth() != 0 {
		t.Fatalf("report=%#v depth=%d", report, outbox.Depth())
	}
}

func TestCharacterStateOutboxFailureDoesNotRollbackLeave(t *testing.T) {
	outbox, _ := characterstate.NewOutbox(1)
	blocker, err := characteridentity.NewTrusted("character:blocker")
	if err != nil {
		t.Fatal(err)
	}
	blockerSnapshot := characterstate.Snapshot{
		World: characterStateTestWorld, HP: 1000, MaxHP: 1000,
		Position: world.Position{Layer: 0},
	}
	if _, err := outbox.Enqueue(blocker, blockerSnapshot); err != nil {
		t.Fatal(err)
	}

	rt := makeCharacterStateRuntime(t, outbox)
	identity, _ := characteridentity.NewTrusted("character:alpha")
	conn := session.NewQueueConnection(32, 32)
	s, err := session.NewWithCharacterIdentity(1, 1, identity, 64, conn)
	if err != nil {
		t.Fatal(err)
	}
	if err := rt.EnqueueRegister(s); err != nil {
		t.Fatal(err)
	}
	rt.Step(1, 50*time.Millisecond)
	if err := rt.EnqueueLeave(1); err != nil {
		t.Fatal(err)
	}
	report := rt.Step(2, 50*time.Millisecond)
	if len(report.CommandErrors) != 1 || !errors.Is(report.CommandErrors[0].Err, characterstate.ErrSaveOutboxFull) {
		t.Fatalf("errors=%#v", report.CommandErrors)
	}
	if report.Metrics.CharacterStateSaveIntentsEnqueued != 0 || report.Metrics.CharacterStateSaveIntentFailures != 1 {
		t.Fatalf("metrics=%#v", report.Metrics)
	}
	if _, ok := rt.sessions.Get(1); ok {
		t.Fatal("save failure kept session registered")
	}
	if _, ok := rt.world.Entity(1); ok {
		t.Fatal("save failure kept world entity alive")
	}
	if outbox.Depth() != 1 || outbox.Pending(1)[0].Identity != blocker {
		t.Fatalf("outbox=%#v", outbox.Pending(0))
	}
}

func makeCharacterStateRuntime(t *testing.T, outbox *characterstate.Outbox) *Runtime {
	t.Helper()
	nav := navigation.Plane{MinX: -100, MaxX: 100, MinZ: -100, MaxZ: 100, Layer: 4}
	sim := simulation.New(spatial.NewGrid(16), movement.NewService(nav, 0.1))
	entity := world.EntityState{
		ID: 1, Kind: world.EntityPlayer,
		Transform: world.Transform{Position: world.Position{X: 7, Y: 2, Z: -3, Layer: 4}, Yaw: 1.25},
	}
	if err := sim.Spawn(entity, 6, 0.35, 0.5); err != nil {
		t.Fatal(err)
	}
	cfg := DefaultConfig()
	cfg.SnapshotEveryTicks = 1
	return New(sim, cfg, WithCharacterStateOutbox(outbox, characterStateTestWorld))
}
