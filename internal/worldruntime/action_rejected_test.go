package worldruntime

import (
	"testing"
	"time"

	"github.com/li41/astrahold-server/internal/protocol"
	"github.com/li41/astrahold-server/internal/session"
	"github.com/li41/astrahold-server/internal/world"
)

func TestSkillUXRejectionAuthorityContract(t *testing.T) {
	rt, source, observer := makeCharacterCombatRuntime(t)
	rt.Step(1, 50*time.Millisecond)
	drainConnection(source)
	drainConnection(observer)

	// Invalid target is authored by Server and correlated to the exact processed client sequence.
	if err := rt.EnqueueUseAction(1, 10, protocol.ClientUseAction{ActionID:"basic-attack",TargetKind:protocol.ActionTargetEntity,TargetID:"1"}); err != nil { t.Fatal(err) }
	invalid := rt.Step(2, 50*time.Millisecond)
	if len(invalid.ActionRejections) != 1 { t.Fatalf("invalid rejections=%#v", invalid.ActionRejections) }
	assertActionRejected(t, source, 10, protocol.ActionRejectionInvalidTarget, 0)
	assertNoActionResult(t, observer)

	// Accepted action emits ActionStarted; a subsequent early press is rejected as cooldown and
	// carries the already-committed authoritative ready tick.
	if err := rt.EnqueueUseAction(1, 11, protocol.ClientUseAction{ActionID:"basic-attack",TargetKind:protocol.ActionTargetEntity,TargetID:"2"}); err != nil { t.Fatal(err) }
	accepted := rt.Step(3, 50*time.Millisecond)
	if len(accepted.ActionRejections) != 0 { t.Fatalf("accepted=%#v", accepted.ActionRejections) }
	assertActionStarted(t, source, "basic-attack")
	drainConnection(source)
	drainConnection(observer)

	if err := rt.EnqueueUseAction(1, 12, protocol.ClientUseAction{ActionID:"basic-attack",TargetKind:protocol.ActionTargetEntity,TargetID:"2"}); err != nil { t.Fatal(err) }
	cooldown := rt.Step(4, 50*time.Millisecond)
	if len(cooldown.ActionRejections) != 1 { t.Fatalf("cooldown=%#v", cooldown.ActionRejections) }
	assertActionRejected(t, source, 12, protocol.ActionRejectionCooldown, 13)
	assertNoActionResult(t, observer)

	// Range legality is also Server-owned. Moving the target server-side makes the same otherwise
	// valid client intent reject with out_of_range rather than any client-side guess.
	if err := rt.EnqueueTeleport(2, world.Position{X:20,Layer:0}); err != nil { t.Fatal(err) }
	move := rt.Step(13, 50*time.Millisecond)
	if len(move.CommandErrors) != 0 { t.Fatalf("teleport=%#v", move.CommandErrors) }
	drainConnection(source)
	drainConnection(observer)
	if err := rt.EnqueueUseAction(1, 13, protocol.ClientUseAction{ActionID:"basic-attack",TargetKind:protocol.ActionTargetEntity,TargetID:"2"}); err != nil { t.Fatal(err) }
	outOfRange := rt.Step(14, 50*time.Millisecond)
	if len(outOfRange.ActionRejections) != 1 { t.Fatalf("out of range=%#v", outOfRange.ActionRejections) }
	assertActionRejected(t, source, 13, protocol.ActionRejectionOutOfRange, 0)
	assertNoActionResult(t, observer)
}

func assertActionRejected(t *testing.T, conn *session.QueueConnection, sequence uint32, reason protocol.ActionRejectionReason, readyTick uint64) {
	t.Helper()
	for {
		select {
		case env := <-conn.Reliable():
			switch msg := env.Message.(type) {
			case protocol.ActionRejected:
				if msg.ClientActionSequence != sequence || msg.ActorEntityID != 1 || msg.ActionID != "basic-attack" || msg.TargetKind != protocol.ActionTargetEntity || msg.Reason != reason || msg.CooldownReadyTick != readyTick {
					t.Fatalf("ActionRejected=%#v want sequence=%d reason=%s ready=%d", msg, sequence, reason, readyTick)
				}
				return
			case protocol.ActionStarted:
				t.Fatalf("rejected action emitted ActionStarted: %#v", msg)
			}
		default:
			t.Fatalf("missing ActionRejected sequence=%d reason=%s", sequence, reason)
		}
	}
}

func assertActionStarted(t *testing.T, conn *session.QueueConnection, actionID string) {
	t.Helper()
	for {
		select {
		case env := <-conn.Reliable():
			if msg, ok := env.Message.(protocol.ActionStarted); ok && msg.ActorEntityID == 1 && msg.ActionID == actionID { return }
		default:
			t.Fatalf("missing ActionStarted action=%s", actionID)
		}
	}
}

func assertNoActionResult(t *testing.T, conn *session.QueueConnection) {
	t.Helper()
	for {
		select {
		case env := <-conn.Reliable():
			switch env.Message.(type) {
			case protocol.ActionStarted, protocol.ActionRejected:
				t.Fatalf("observer received source-only rejection/result: %#v", env.Message)
			}
		default:
			return
		}
	}
}
