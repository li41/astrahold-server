package worldruntime

import (
	"testing"
	"time"

	"github.com/li41/astrahold-server/internal/characteridentity"
	"github.com/li41/astrahold-server/internal/characterstate"
	"github.com/li41/astrahold-server/internal/respawnpolicy"
	"github.com/li41/astrahold-server/internal/session"
	"github.com/li41/astrahold-server/internal/world"
)

func TestDefeatedLeaveCapturesImmutableRespawnAndPostPenaltyCheckpointTruth(t *testing.T) {
	outbox, err := characterstate.NewOutbox(4)
	if err != nil {
		t.Fatal(err)
	}
	rt := makeDefeatedRestoreRuntime(t)
	rt.characterStateOutbox = outbox

	entityID := world.EntityID(42)
	identity, _ := characteridentity.NewTrusted("character:defeated-leave")
	if err := rt.world.Spawn(world.EntityState{ID: entityID, Kind: world.EntityPlayer, Transform: world.Transform{Position: world.Position{X: 2, Layer: 4}}}, 6, 0.35, 0.5); err != nil {
		t.Fatal(err)
	}
	conn := session.NewQueueConnection(32, 32)
	sess, _ := session.NewWithCharacterIdentity(7, entityID, identity, 64, conn)
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
	// Models the existing S3-F.7 ordering: the death-time destination is already
	// bound before checkpoint forfeiture clears current checkpoint state.
	rt.respawnPolicy.ClearCheckpoint(entityID)

	if err := rt.EnqueueLeave(sess.ID); err != nil {
		t.Fatal(err)
	}
	report := rt.Step(10, 50*time.Millisecond)
	if len(report.CommandErrors) != 0 || report.Metrics.CharacterStateSaveIntentsEnqueued != 1 {
		t.Fatalf("leave=%#v", report)
	}
	pending := outbox.Pending(1)
	if len(pending) != 1 {
		t.Fatalf("pending=%#v", pending)
	}
	got := pending[0].Snapshot
	if !got.Defeated || got.HP != 0 || got.Respawn.Context != respawnpolicy.DeathContextPvE {
		t.Fatalf("snapshot=%#v", got)
	}
	if got.Respawn.SpawnPointID != "checkpoint" || got.Respawn.SpawnClass != respawnpolicy.SpawnClassCheckpoint || got.Respawn.Position != bound.Position {
		t.Fatalf("bound respawn drifted: %#v", got.Respawn)
	}
	if got.Respawn.CheckpointID != "" {
		t.Fatalf("post-penalty checkpoint resurrected: %#v", got.Respawn)
	}
	if got.Respawn.RemainingTicks != 10 {
		t.Fatalf("remaining=%d want=10", got.Respawn.RemainingTicks)
	}
}
