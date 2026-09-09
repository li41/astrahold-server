package worldruntime

import (
	"errors"
	"testing"
	"time"

	"github.com/li41/astrahold-server/internal/character"
	"github.com/li41/astrahold-server/internal/protocol"
	"github.com/li41/astrahold-server/internal/world"
)

func TestClientRespawnRequestKeepsDefeatedSessionUntilPlayerRequestsRestart(t *testing.T) {
	rt, policy := makeRespawnContextRuntime(t, world.EntityMonster)
	if report := rt.Step(1, 50*time.Millisecond); len(report.CommandErrors) != 0 {
		t.Fatalf("bootstrap=%#v", report.CommandErrors)
	}

	if err := rt.EnqueueUseAction(1, 1, protocol.ClientUseAction{ActionID: "kill", TargetKind: protocol.ActionTargetEntity, TargetID: "2"}); err != nil {
		t.Fatal(err)
	}
	defeat := rt.Step(2, 50*time.Millisecond)
	if len(defeat.CommandErrors) != 0 || len(defeat.ActionRejections) != 0 || defeat.Metrics.RespawnsScheduled != 1 {
		t.Fatalf("defeat=%#v", defeat)
	}
	pending, ok := policy.Pending(2)
	if !ok || pending.DueTick != 4 || pending.Position.X != 50 {
		t.Fatalf("pending=%#v ok=%v", pending, ok)
	}

	// Reaching policy due without player consent must leave the authoritative character defeated.
	if report := rt.Step(3, 50*time.Millisecond); report.Metrics.RespawnsApplied != 0 {
		t.Fatalf("pre-due report=%#v", report)
	}
	dueWithoutRequest := rt.Step(4, 50*time.Millisecond)
	if len(dueWithoutRequest.CommandErrors) != 0 || dueWithoutRequest.Metrics.RespawnPolicyDue != 1 || dueWithoutRequest.Metrics.RespawnsApplied != 0 {
		t.Fatalf("due without request=%#v", dueWithoutRequest)
	}
	state, _ := rt.characters.State(2)
	if !state.Defeated || state.HP != 0 {
		t.Fatalf("character revived without request: %#v", state)
	}
	if _, ok := policy.Pending(2); !ok {
		t.Fatal("due schedule was consumed without restart request")
	}

	// A Reliable restart click after due arms consent in command phase and the normal due phase applies
	// the pre-bound Server destination in the same tick. The client never supplies position.
	if err := rt.EnqueueRespawnRequest(2, 1, protocol.ClientRespawnRequest{}); err != nil {
		t.Fatal(err)
	}
	restart := rt.Step(5, 50*time.Millisecond)
	if len(restart.CommandErrors) != 0 || restart.Metrics.RespawnsApplied != 1 {
		t.Fatalf("restart=%#v", restart)
	}
	state, _ = rt.characters.State(2)
	if state.Defeated || state.HP != 200 {
		t.Fatalf("state after restart=%#v", state)
	}
	entity, ok := rt.world.Entity(2)
	if !ok || entity.Transform.Position.X != 50 || entity.Transform.Position.Z != 0 {
		t.Fatalf("authoritative respawn position=%#v ok=%v", entity.Transform.Position, ok)
	}
	if _, ok := policy.Pending(2); ok {
		t.Fatal("pending schedule survived successful restart")
	}

	// Once alive, another restart request is a gameplay error, not a transport-level reason to close
	// the connection. The Reliable sequence is still consumed by the authoritative runtime.
	if err := rt.EnqueueRespawnRequest(2, 2, protocol.ClientRespawnRequest{}); err != nil {
		t.Fatal(err)
	}
	alive := rt.Step(6, 50*time.Millisecond)
	if len(alive.CommandErrors) != 1 || !errors.Is(alive.CommandErrors[0].Err, character.ErrCharacterNotDefeated) {
		t.Fatalf("alive restart errors=%#v", alive.CommandErrors)
	}
}

func TestEarlyClientRespawnRequestDoesNotBypassServerDelay(t *testing.T) {
	rt, _ := makeRespawnContextRuntime(t, world.EntityMonster)
	rt.Step(1, 50*time.Millisecond)
	if err := rt.EnqueueUseAction(1, 1, protocol.ClientUseAction{ActionID: "kill", TargetKind: protocol.ActionTargetEntity, TargetID: "2"}); err != nil {
		t.Fatal(err)
	}
	rt.Step(2, 50*time.Millisecond)

	if err := rt.EnqueueRespawnRequest(2, 1, protocol.ClientRespawnRequest{}); err != nil {
		t.Fatal(err)
	}
	early := rt.Step(3, 50*time.Millisecond)
	if len(early.CommandErrors) != 0 || early.Metrics.RespawnsApplied != 0 {
		t.Fatalf("early request=%#v", early)
	}
	state, _ := rt.characters.State(2)
	if !state.Defeated {
		t.Fatalf("early request bypassed respawn delay: %#v", state)
	}

	due := rt.Step(4, 50*time.Millisecond)
	if len(due.CommandErrors) != 0 || due.Metrics.RespawnsApplied != 1 {
		t.Fatalf("due after armed request=%#v", due)
	}
}
