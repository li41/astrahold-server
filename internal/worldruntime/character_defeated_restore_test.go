package worldruntime

import (
	"errors"
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

func TestJoinRestoresDefeatedRespawnAtRemainingTickBoundary(t *testing.T) {
	rt := makeDefeatedRestoreRuntime(t)
	identity, _ := characteridentity.NewTrusted("character:defeated-restore")
	conn := session.NewQueueConnection(32, 32)
	sess, _ := session.NewWithCharacterIdentity(10, 77, identity, 64, conn)
	restore := defeatedRestoreFixture(identity.ID, 2)
	request := JoinRequest{
		Session: sess,
		Entity: world.EntityState{ID: 77, Kind: world.EntityPlayer, Transform: world.Transform{Position: world.Position{X: -40, Layer: 4}}},
		Speed: 6, Radius: 0.35, MaxStepHeight: 0.5, Restore: &restore,
	}
	if err := rt.EnqueueJoin(request); err != nil {
		t.Fatal(err)
	}
	if report := rt.Step(10, 50*time.Millisecond); len(report.CommandErrors) != 0 {
		t.Fatalf("join errors=%#v", report.CommandErrors)
	}
	state, _ := rt.characters.State(77)
	if !state.Defeated || state.HP != 0 {
		t.Fatalf("joined state=%#v", state)
	}
	pending, ok := rt.respawnPolicy.Pending(77)
	if !ok || pending.DueTick != 12 || pending.Context != respawnpolicy.DeathContextPvP || pending.Position != restore.Respawn.Position {
		t.Fatalf("pending=%#v ok=%v", pending, ok)
	}
	if checkpoint, ok := rt.respawnPolicy.Checkpoint(77); !ok || checkpoint != "checkpoint" {
		t.Fatalf("checkpoint=%q ok=%v", checkpoint, ok)
	}

	if report := rt.Step(11, 50*time.Millisecond); report.Metrics.RespawnsApplied != 0 {
		t.Fatalf("early respawn report=%#v", report)
	}
	state, _ = rt.characters.State(77)
	if !state.Defeated {
		t.Fatal("character respawned before exact due boundary")
	}
	if report := rt.Step(12, 50*time.Millisecond); report.Metrics.RespawnsApplied != 1 {
		t.Fatalf("due report=%#v", report)
	}
	state, _ = rt.characters.State(77)
	if state.Defeated || state.HP != state.MaxHP {
		t.Fatalf("respawned state=%#v", state)
	}
	entity, _ := rt.world.Entity(77)
	if entity.Transform.Position != restore.Respawn.Position {
		t.Fatalf("position=%#v want=%#v", entity.Transform.Position, restore.Respawn.Position)
	}
	if _, ok := rt.respawnPolicy.Pending(77); ok {
		t.Fatal("successful respawn left duplicate pending schedule")
	}
}

func TestJoinRestoresZeroRemainingTicksOnSameDueBoundary(t *testing.T) {
	rt := makeDefeatedRestoreRuntime(t)
	identity, _ := characteridentity.NewTrusted("character:due-now")
	conn := session.NewQueueConnection(32, 32)
	sess, _ := session.NewWithCharacterIdentity(1, 91, identity, 64, conn)
	restore := defeatedRestoreFixture(identity.ID, 0)
	if err := rt.EnqueueJoin(JoinRequest{Session: sess, Entity: world.EntityState{ID: 91, Kind: world.EntityPlayer}, Speed: 6, Radius: 0.35, MaxStepHeight: 0.5, Restore: &restore}); err != nil {
		t.Fatal(err)
	}
	report := rt.Step(50, 50*time.Millisecond)
	if len(report.CommandErrors) != 0 || report.Metrics.RespawnsApplied != 1 {
		t.Fatalf("report=%#v", report)
	}
	state, _ := rt.characters.State(91)
	if state.Defeated {
		t.Fatal("zero remaining ticks did not respawn on join tick due phase")
	}
}

func TestJoinRejectsDurableRespawnBindingDriftBeforeSpawn(t *testing.T) {
	rt := makeDefeatedRestoreRuntime(t)
	identity, _ := characteridentity.NewTrusted("character:binding-drift")
	conn := session.NewQueueConnection(32, 32)
	sess, _ := session.NewWithCharacterIdentity(1, 5, identity, 64, conn)
	restore := defeatedRestoreFixture(identity.ID, 3)
	restore.Respawn.Position.X++
	if err := rt.EnqueueJoin(JoinRequest{Session: sess, Entity: world.EntityState{ID: 5, Kind: world.EntityPlayer}, Speed: 6, Radius: 0.35, MaxStepHeight: 0.5, Restore: &restore}); err != nil {
		t.Fatal(err)
	}
	report := rt.Step(20, 50*time.Millisecond)
	if len(report.CommandErrors) != 1 || !errors.Is(report.CommandErrors[0].Err, ErrCharacterRestoreInvalid) {
		t.Fatalf("errors=%#v", report.CommandErrors)
	}
	if _, ok := rt.world.Entity(5); ok {
		t.Fatal("invalid durable binding partially spawned entity")
	}
	if _, ok := rt.respawnPolicy.Pending(5); ok {
		t.Fatal("invalid durable binding installed pending respawn")
	}
}

func defeatedRestoreFixture(characterID characteridentity.ID, remaining uint64) CharacterRestore {
	return CharacterRestore{
		SchemaVersion: characterstate.SchemaVersion,
		CharacterID: characterID,
		Revision: 3,
		World: characterRestoreWorld,
		HP: 0, MaxHP: 1000, Defeated: true,
		Transform: world.Transform{Position: world.Position{X: 3, Y: 0, Z: 4, Layer: 4}, Yaw: 0.5},
		Respawn: characterstate.DefeatedRespawn{
			Context: respawnpolicy.DeathContextPvP,
			SpawnPointID: "safe", SpawnClass: respawnpolicy.SpawnClassSafe,
			Position: world.Position{X: 12, Y: 0, Z: -6, Layer: 4}, RemainingTicks: remaining,
			CheckpointID: "checkpoint",
		},
	}
}

func makeDefeatedRestoreRuntime(t *testing.T) *Runtime {
	t.Helper()
	nav := navigation.Plane{MinX: -100, MaxX: 100, MinZ: -100, MaxZ: 100, Layer: 4}
	sim := simulation.New(spatial.NewGrid(16), movement.NewService(nav, 0.1))
	policy, err := respawnpolicy.NewService(respawnpolicy.Definition{
		SchemaVersion: respawnpolicy.SchemaVersion,
		Revision: "restore-test",
		SpawnPoints: []respawnpolicy.SpawnPoint{
			{ID: "safe", Class: respawnpolicy.SpawnClassSafe, X: 12, Y: 0, Z: -6, Layer: 4},
			{ID: "checkpoint", Class: respawnpolicy.SpawnClassCheckpoint, X: 5, Y: 0, Z: 5, Layer: 4, CheckpointActivationRadius: 3},
			{ID: "siege", Class: respawnpolicy.SpawnClassSiege, X: -12, Y: 0, Z: 6, Layer: 4},
		},
		Contexts: []respawnpolicy.ContextRule{
			{Context: respawnpolicy.DeathContextPvE, RespawnDelaySeconds: 1, DefaultSpawnPoint: "safe", AllowedSpawnClasses: []respawnpolicy.SpawnClass{respawnpolicy.SpawnClassSafe, respawnpolicy.SpawnClassCheckpoint}},
			{Context: respawnpolicy.DeathContextPvP, RespawnDelaySeconds: 2, DefaultSpawnPoint: "safe", AllowedSpawnClasses: []respawnpolicy.SpawnClass{respawnpolicy.SpawnClassSafe}},
			{Context: respawnpolicy.DeathContextSiege, RespawnDelaySeconds: 3, DefaultSpawnPoint: "siege", AllowedSpawnClasses: []respawnpolicy.SpawnClass{respawnpolicy.SpawnClassSafe, respawnpolicy.SpawnClassSiege}},
		},
	}, 20)
	if err != nil {
		t.Fatal(err)
	}
	cfg := DefaultConfig()
	cfg.SnapshotEveryTicks = 1
	worldRef := characterstate.WorldRef{WorldID: characterRestoreWorld.WorldID, Revision: characterRestoreWorld.Revision, GameplaySHA256: characterRestoreWorld.GameplaySHA256}
	return New(sim, cfg, WithRespawnPolicy(policy), WithCharacterStateOutbox(nil, worldRef))
}
