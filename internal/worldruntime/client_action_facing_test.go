package worldruntime

import (
	"errors"
	"testing"
	"time"

	"github.com/li41/astrahold-server/internal/combat"
	"github.com/li41/astrahold-server/internal/protocol"
	"github.com/li41/astrahold-server/internal/world"
)

func TestAcceptedClientEntityActionFacesAuthoritativeTarget(t *testing.T) {
	rt, conn1, conn2 := makeCharacterCombatRuntime(t)
	if report := rt.Step(1, 50*time.Millisecond); len(report.CommandErrors) != 0 {
		t.Fatalf("initial report=%#v", report)
	}
	drainConnection(conn1)
	drainConnection(conn2)

	if err := rt.EnqueueTeleport(2, world.Position{Z: 2, Layer: 0}); err != nil {
		t.Fatal(err)
	}
	if report := rt.Step(2, 50*time.Millisecond); len(report.CommandErrors) != 0 {
		t.Fatalf("teleport report=%#v", report)
	}
	drainConnection(conn1)
	drainConnection(conn2)

	actor, ok := rt.world.Entity(1)
	if !ok || actor.Transform.Yaw != 0 {
		t.Fatalf("actor before action=%+v ok=%v", actor, ok)
	}
	if err := rt.EnqueueUseAction(1, 1, protocol.ClientUseAction{
		ActionID:   "basic-attack",
		TargetKind: protocol.ActionTargetEntity,
		TargetID:   "2",
	}); err != nil {
		t.Fatal(err)
	}
	report := rt.Step(3, 50*time.Millisecond)
	if len(report.CommandErrors) != 0 || len(report.ActionRejections) != 0 {
		t.Fatalf("accepted action report=%#v", report)
	}
	actor, ok = rt.world.Entity(1)
	if !ok || actor.Transform.Yaw < 89.99 || actor.Transform.Yaw > 90.01 {
		t.Fatalf("actor after accepted action=%+v ok=%v; want yaw 90", actor, ok)
	}
}

func TestRejectedClientEntityActionsDoNotChangeAuthoritativeFacing(t *testing.T) {
	rt, conn1, conn2 := makeCharacterCombatRuntime(t)
	if report := rt.Step(1, 50*time.Millisecond); len(report.CommandErrors) != 0 {
		t.Fatalf("initial report=%#v", report)
	}
	drainConnection(conn1)
	drainConnection(conn2)

	// First establish an accepted north-facing action so later rejection has a visible yaw to preserve.
	if err := rt.EnqueueTeleport(2, world.Position{Z: 2, Layer: 0}); err != nil {
		t.Fatal(err)
	}
	if report := rt.Step(2, 50*time.Millisecond); len(report.CommandErrors) != 0 {
		t.Fatalf("teleport report=%#v", report)
	}
	if err := rt.EnqueueUseAction(1, 1, protocol.ClientUseAction{ActionID: "basic-attack", TargetKind: protocol.ActionTargetEntity, TargetID: "2"}); err != nil {
		t.Fatal(err)
	}
	if report := rt.Step(3, 50*time.Millisecond); len(report.CommandErrors) != 0 || len(report.ActionRejections) != 0 {
		t.Fatalf("accepted action report=%#v", report)
	}

	// Move the target west. The immediate repeat is rejected by cooldown before dispatch and must
	// not rotate the actor from north (90) toward west (180/-180).
	if err := rt.EnqueueTeleport(2, world.Position{X: -2, Layer: 0}); err != nil {
		t.Fatal(err)
	}
	if report := rt.Step(4, 50*time.Millisecond); len(report.CommandErrors) != 0 {
		t.Fatalf("second teleport report=%#v", report)
	}
	if err := rt.EnqueueUseAction(1, 2, protocol.ClientUseAction{ActionID: "basic-attack", TargetKind: protocol.ActionTargetEntity, TargetID: "2"}); err != nil {
		t.Fatal(err)
	}
	report := rt.Step(5, 50*time.Millisecond)
	if len(report.ActionRejections) != 1 || !errors.Is(report.ActionRejections[0].Err, combat.ErrActionCooldown) {
		t.Fatalf("cooldown rejection=%#v", report.ActionRejections)
	}
	actor, ok := rt.world.Entity(1)
	if !ok || actor.Transform.Yaw < 89.99 || actor.Transform.Yaw > 90.01 {
		t.Fatalf("cooldown rejection changed actor facing: %+v ok=%v", actor, ok)
	}

	// Once cooldown is ready, put the target north but beyond legal range. Dispatch target validation
	// rejects the action and must likewise preserve the previous accepted yaw.
	if err := rt.EnqueueTeleport(2, world.Position{Z: 10, Layer: 0}); err != nil {
		t.Fatal(err)
	}
	if report := rt.Step(6, 50*time.Millisecond); len(report.CommandErrors) != 0 {
		t.Fatalf("out-of-range teleport report=%#v", report)
	}
	if err := rt.EnqueueUseAction(1, 3, protocol.ClientUseAction{ActionID: "basic-attack", TargetKind: protocol.ActionTargetEntity, TargetID: "2"}); err != nil {
		t.Fatal(err)
	}
	report = rt.Step(13, 50*time.Millisecond)
	if len(report.ActionRejections) != 1 || !errors.Is(report.ActionRejections[0].Err, ErrEntityOutOfRange) {
		t.Fatalf("out-of-range rejection=%#v", report.ActionRejections)
	}
	actor, ok = rt.world.Entity(1)
	if !ok || actor.Transform.Yaw < 89.99 || actor.Transform.Yaw > 90.01 {
		t.Fatalf("out-of-range rejection changed actor facing: %+v ok=%v", actor, ok)
	}
}
