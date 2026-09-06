package worldruntime

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/li41/astrahold-server/internal/characteridentity"
	"github.com/li41/astrahold-server/internal/characterstate"
	"github.com/li41/astrahold-server/internal/movement"
	"github.com/li41/astrahold-server/internal/navigation"
	"github.com/li41/astrahold-server/internal/respawnpolicy"
	"github.com/li41/astrahold-server/internal/session"
	"github.com/li41/astrahold-server/internal/simulation"
	"github.com/li41/astrahold-server/internal/spatial"
	"github.com/li41/astrahold-server/internal/world"
)

func TestTrustedCharacterAutosaveWaitsIntervalAndCapturesPostSimulationState(t *testing.T) {
	outbox, _ := characterstate.NewOutbox(8)
	rt, identities := makeAutosaveRuntime(t, outbox, 1, 2, 1)
	registerAutosaveSessions(t, rt, identities, 10)
	if outbox.Depth() != 0 {
		t.Fatalf("join tick autosaved immediately depth=%d", outbox.Depth())
	}
	if _, err := rt.characters.ApplyDamage(1, 125); err != nil {
		t.Fatal(err)
	}
	if err := rt.world.Teleport(1, world.Position{X: 21, Z: -9, Layer: 4}); err != nil {
		t.Fatal(err)
	}
	if report := rt.Step(11, 50*time.Millisecond); report.Metrics.CharacterStateAutosaveAttempts != 0 || outbox.Depth() != 0 {
		t.Fatalf("early autosave report=%#v depth=%d", report, outbox.Depth())
	}
	report := rt.Step(12, 50*time.Millisecond)
	if len(report.CommandErrors) != 0 || report.Metrics.CharacterStateAutosaveBudget != 1 || report.Metrics.CharacterStateAutosaveAttempts != 1 || report.Metrics.CharacterStateAutosaveEnqueued != 1 || report.Metrics.CharacterStateSaveIntentsEnqueued != 1 {
		t.Fatalf("report=%#v", report)
	}
	pending := outbox.Pending(1)
	if len(pending) != 1 {
		t.Fatalf("pending=%#v", pending)
	}
	got := pending[0]
	if got.Identity != identities[0] || got.Snapshot.HP != 875 || got.Snapshot.Position != (world.Position{X: 21, Y: 0, Z: -9, Layer: 4}) {
		t.Fatalf("intent=%#v", got)
	}
}

func TestTrustedCharacterAutosaveBudgetRoundRobinEventuallyVisitsAllDueSessions(t *testing.T) {
	outbox, _ := characterstate.NewOutbox(16)
	rt, identities := makeAutosaveRuntime(t, outbox, 3, 1, 1)
	registerAutosaveSessions(t, rt, identities, 1)

	for tick := uint64(2); tick <= 4; tick++ {
		report := rt.Step(tick, 50*time.Millisecond)
		if report.Metrics.CharacterStateAutosaveAttempts != 1 || report.Metrics.CharacterStateAutosaveEnqueued != 1 || report.Metrics.CharacterStateSaveIntentsEnqueued != 1 {
			t.Fatalf("tick=%d report=%#v", tick, report)
		}
		if !report.Metrics.CharacterStateAutosaveBudgetExhausted {
			t.Fatalf("tick=%d budget did not report exhaustion", tick)
		}
	}
	pending := outbox.Pending(0)
	if len(pending) != 3 {
		t.Fatalf("pending=%#v", pending)
	}
	seen := map[characteridentity.ID]bool{}
	for _, intent := range pending {
		seen[intent.Identity.ID] = true
	}
	for _, identity := range identities {
		if !seen[identity.ID] {
			t.Fatalf("character %s starved pending=%#v", identity.ID, pending)
		}
	}
}

func TestCharacterAutosaveIgnoresEphemeralSession(t *testing.T) {
	outbox, _ := characterstate.NewOutbox(8)
	nav := navigation.Plane{MinX: -100, MaxX: 100, MinZ: -100, MaxZ: 100, Layer: 4}
	sim := simulation.New(spatial.NewGrid(16), movement.NewService(nav, 0.1))
	for id := world.EntityID(1); id <= 2; id++ {
		if err := sim.Spawn(world.EntityState{ID: id, Kind: world.EntityPlayer, Transform: world.Transform{Position: world.Position{Layer: 4}}}, 6, 0.35, 0.5); err != nil {
			t.Fatal(err)
		}
	}
	cfg := DefaultConfig()
	cfg.SnapshotEveryTicks = 1000
	cfg.CharacterStateAutosaveEveryTicks = 1
	cfg.MaxCharacterStateAutosavesPerTick = 2
	rt := New(sim, cfg, WithCharacterStateOutbox(outbox, characterStateTestWorld))
	ephemeral, _ := session.New(1, 1, 64, session.NewQueueConnection(8, 8))
	trustedIdentity, _ := characteridentity.NewTrusted("character:trusted")
	trusted, _ := session.NewWithCharacterIdentity(2, 2, trustedIdentity, 64, session.NewQueueConnection(8, 8))
	if err := rt.EnqueueRegister(ephemeral); err != nil {
		t.Fatal(err)
	}
	if err := rt.EnqueueRegister(trusted); err != nil {
		t.Fatal(err)
	}
	if report := rt.Step(1, 50*time.Millisecond); len(report.CommandErrors) != 0 {
		t.Fatalf("register=%#v", report.CommandErrors)
	}
	if _, ok := rt.characterStateAutosaveLastTick[1]; ok {
		t.Fatal("ephemeral character entered autosave bookkeeping")
	}
	report := rt.Step(2, 50*time.Millisecond)
	if report.Metrics.CharacterStateAutosaveAttempts != 1 || report.Metrics.CharacterStateAutosaveEnqueued != 1 || outbox.Depth() != 1 {
		t.Fatalf("report=%#v depth=%d", report, outbox.Depth())
	}
	if got := outbox.Pending(1)[0].Identity; got != trustedIdentity {
		t.Fatalf("autosaved identity=%#v", got)
	}
}

func TestCharacterAutosaveOutboxFailureRetriesWithoutAdvancingBaseline(t *testing.T) {
	outbox, _ := characterstate.NewOutbox(1)
	blocker, _ := characteridentity.NewTrusted("character:blocker")
	blockerIntent, err := outbox.Enqueue(blocker, characterstate.Snapshot{
		World: characterStateTestWorld, HP: 1000, MaxHP: 1000, MP: 100, MaxMP: 100, Position: world.Position{Layer: 4},
	})
	if err != nil {
		t.Fatal(err)
	}
	rt, identities := makeAutosaveRuntime(t, outbox, 1, 1, 1)
	registerAutosaveSessions(t, rt, identities, 1)
	report := rt.Step(2, 50*time.Millisecond)
	if report.Metrics.CharacterStateAutosaveAttempts != 1 || report.Metrics.CharacterStateAutosaveEnqueued != 0 || report.Metrics.CharacterStateSaveIntentFailures != 1 || len(report.CommandErrors) != 1 || !errors.Is(report.CommandErrors[0].Err, characterstate.ErrSaveOutboxFull) {
		t.Fatalf("full report=%#v", report)
	}
	if err := outbox.Confirm(blockerIntent.IntentID); err != nil {
		t.Fatal(err)
	}
	report = rt.Step(3, 50*time.Millisecond)
	if len(report.CommandErrors) != 0 || report.Metrics.CharacterStateAutosaveAttempts != 1 || report.Metrics.CharacterStateAutosaveEnqueued != 1 || outbox.Depth() != 1 {
		t.Fatalf("retry report=%#v depth=%d", report, outbox.Depth())
	}
	if got := outbox.Pending(1)[0].Identity; got != identities[0] {
		t.Fatalf("retry identity=%#v", got)
	}
}

func TestDefeatedCharacterAutosavePreservesBoundRespawnTruth(t *testing.T) {
	outbox, _ := characterstate.NewOutbox(4)
	rt := makeDefeatedRestoreRuntime(t)
	rt.characterStateOutbox = outbox
	rt.config.CharacterStateAutosaveEveryTicks = 2
	rt.config.MaxCharacterStateAutosavesPerTick = 1

	entityID := world.EntityID(42)
	identity, _ := characteridentity.NewTrusted("character:defeated-autosave")
	if err := rt.world.Spawn(world.EntityState{ID: entityID, Kind: world.EntityPlayer, Transform: world.Transform{Position: world.Position{X: 2, Layer: 4}}}, 6, 0.35, 0.5); err != nil {
		t.Fatal(err)
	}
	sess, _ := session.NewWithCharacterIdentity(7, entityID, identity, 64, session.NewQueueConnection(32, 32))
	if err := rt.EnqueueRegister(sess); err != nil {
		t.Fatal(err)
	}
	if report := rt.Step(1, 50*time.Millisecond); len(report.CommandErrors) != 0 {
		t.Fatalf("register=%#v", report.CommandErrors)
	}
	if _, err := rt.characters.ApplyDamage(entityID, 1000); err != nil {
		t.Fatal(err)
	}
	if err := rt.respawnPolicy.RestoreCheckpoint(entityID, "checkpoint"); err != nil {
		t.Fatal(err)
	}
	bound := respawnpolicy.Scheduled{
		EntityID: entityID, Context: respawnpolicy.DeathContextPvE,
		SpawnPointID: "checkpoint", SpawnClass: respawnpolicy.SpawnClassCheckpoint,
		Position: world.Position{X: 5, Y: 0, Z: 5, Layer: 4}, DueTick: 20,
	}
	if err := rt.respawnPolicy.RestoreScheduled(bound); err != nil {
		t.Fatal(err)
	}
	rt.respawnPolicy.ClearCheckpoint(entityID)

	report := rt.Step(3, 50*time.Millisecond)
	if len(report.CommandErrors) != 0 || report.Metrics.CharacterStateAutosaveEnqueued != 1 {
		t.Fatalf("autosave=%#v", report)
	}
	got := outbox.Pending(1)[0].Snapshot
	if !got.Defeated || got.HP != 0 || got.Respawn.Context != respawnpolicy.DeathContextPvE || got.Respawn.SpawnPointID != "checkpoint" || got.Respawn.SpawnClass != respawnpolicy.SpawnClassCheckpoint || got.Respawn.Position != bound.Position || got.Respawn.CheckpointID != "" || got.Respawn.RemainingTicks != 17 {
		t.Fatalf("snapshot=%#v", got)
	}
}

func makeAutosaveRuntime(t *testing.T, outbox *characterstate.Outbox, count int, interval uint64, budget int) (*Runtime, []characteridentity.Binding) {
	t.Helper()
	nav := navigation.Plane{MinX: -100, MaxX: 100, MinZ: -100, MaxZ: 100, Layer: 4}
	sim := simulation.New(spatial.NewGrid(16), movement.NewService(nav, 0.1))
	identities := make([]characteridentity.Binding, 0, count)
	for index := 0; index < count; index++ {
		entityID := world.EntityID(index + 1)
		if err := sim.Spawn(world.EntityState{ID: entityID, Kind: world.EntityPlayer, Transform: world.Transform{Position: world.Position{X: float32(index), Layer: 4}}}, 6, 0.35, 0.5); err != nil {
			t.Fatal(err)
		}
		identity, err := characteridentity.NewTrusted(fmt.Sprintf("character:auto-%d", index+1))
		if err != nil {
			t.Fatal(err)
		}
		identities = append(identities, identity)
	}
	cfg := DefaultConfig()
	cfg.SnapshotEveryTicks = 1000
	cfg.CharacterStateAutosaveEveryTicks = interval
	cfg.MaxCharacterStateAutosavesPerTick = budget
	return New(sim, cfg, WithCharacterStateOutbox(outbox, characterStateTestWorld)), identities
}

func registerAutosaveSessions(t *testing.T, rt *Runtime, identities []characteridentity.Binding, tick uint64) {
	t.Helper()
	for index, identity := range identities {
		sess, err := session.NewWithCharacterIdentity(session.ID(index+1), world.EntityID(index+1), identity, 64, session.NewQueueConnection(32, 32))
		if err != nil {
			t.Fatal(err)
		}
		if err := rt.EnqueueRegister(sess); err != nil {
			t.Fatal(err)
		}
	}
	if report := rt.Step(tick, 50*time.Millisecond); len(report.CommandErrors) != 0 {
		t.Fatalf("register report=%#v", report)
	}
}
