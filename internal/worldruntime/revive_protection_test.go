package worldruntime

import (
	"errors"
	"testing"
	"time"

	"github.com/li41/astrahold-server/internal/protocol"
	"github.com/li41/astrahold-server/internal/session"
	"github.com/li41/astrahold-server/internal/world"
)

func TestResurrectionProtectionBlocksDamageUntilExactExpiry(t *testing.T) {
	rt, _ := makeResurrectionRuntime(t)
	rt.config.PostReviveProtectionTicks = 3
	if report := rt.Step(1, 50*time.Millisecond); len(report.CommandErrors) != 0 {
		t.Fatalf("bootstrap=%#v", report.CommandErrors)
	}

	if err := rt.EnqueueUseAction(1, 1, protocol.ClientUseAction{ActionID: "basic-attack", TargetKind: protocol.ActionTargetEntity, TargetID: "2"}); err != nil {
		t.Fatal(err)
	}
	defeat := rt.Step(2, 50*time.Millisecond)
	if len(defeat.CommandErrors) != 0 || len(defeat.ActionRejections) != 0 {
		t.Fatalf("defeat=%#v", defeat)
	}

	if err := rt.EnqueueUseAction(3, 1, protocol.ClientUseAction{ActionID: "resurrect", TargetKind: protocol.ActionTargetEntity, TargetID: "2"}); err != nil {
		t.Fatal(err)
	}
	resurrect := rt.Step(3, 50*time.Millisecond)
	if len(resurrect.CommandErrors) != 0 || len(resurrect.ActionRejections) != 0 || resurrect.Metrics.ReviveProtectionsGranted != 1 {
		t.Fatalf("resurrect=%#v", resurrect)
	}
	state, _ := rt.characters.State(2)
	if state.Defeated || state.HP != 60 {
		t.Fatalf("resurrected=%#v", state)
	}

	if err := rt.EnqueueUseAction(3, 2, protocol.ClientUseAction{ActionID: "basic-attack", TargetKind: protocol.ActionTargetEntity, TargetID: "2"}); err != nil {
		t.Fatal(err)
	}
	blocked := rt.Step(4, 50*time.Millisecond)
	if len(blocked.CommandErrors) != 0 || len(blocked.ActionRejections) != 1 || !errors.Is(blocked.ActionRejections[0].Err, ErrEntityReviveProtected) {
		t.Fatalf("blocked=%#v", blocked)
	}
	if blocked.Metrics.ReviveProtectionDamageBlocks != 1 {
		t.Fatalf("blocked metrics=%#v", blocked.Metrics)
	}
	state, _ = rt.characters.State(2)
	if state.HP != 60 || state.Defeated {
		t.Fatalf("protected target changed=%#v", state)
	}

	// Protected rejection still consumes intent sequence.
	if err := rt.EnqueueUseAction(3, 2, protocol.ClientUseAction{ActionID: "basic-attack", TargetKind: protocol.ActionTargetEntity, TargetID: "2"}); err != nil {
		t.Fatal(err)
	}
	replay := rt.Step(5, 50*time.Millisecond)
	if len(replay.CommandErrors) != 1 || !errors.Is(replay.CommandErrors[0].Err, session.ErrStaleAction) {
		t.Fatalf("replay=%#v", replay)
	}

	// Protection interval is [3,6): exact tick 6 is no longer protected. The blocked action did
	// not Commit cooldown, so the next fresh sequence can immediately apply damage at expiry.
	if err := rt.EnqueueUseAction(3, 3, protocol.ClientUseAction{ActionID: "basic-attack", TargetKind: protocol.ActionTargetEntity, TargetID: "2"}); err != nil {
		t.Fatal(err)
	}
	expired := rt.Step(6, 50*time.Millisecond)
	if len(expired.CommandErrors) != 0 || len(expired.ActionRejections) != 0 || expired.Metrics.EntityActionsApplied != 1 {
		t.Fatalf("expired=%#v", expired)
	}
	state, _ = rt.characters.State(2)
	if !state.Defeated || state.HP != 0 {
		t.Fatalf("expired target=%#v", state)
	}
}

func TestRespawnProtectionCancelsAfterSuccessfulDamageAction(t *testing.T) {
	rt, _ := makeResurrectionRuntime(t)
	rt.config.PostReviveProtectionTicks = 5
	rt.Step(1, 50*time.Millisecond)

	if err := rt.EnqueueUseAction(1, 1, protocol.ClientUseAction{ActionID: "basic-attack", TargetKind: protocol.ActionTargetEntity, TargetID: "2"}); err != nil {
		t.Fatal(err)
	}
	if report := rt.Step(2, 50*time.Millisecond); len(report.CommandErrors) != 0 || len(report.ActionRejections) != 0 {
		t.Fatalf("defeat=%#v", report)
	}

	if err := rt.EnqueueRespawn(RespawnRequest{EntityID: 2, Position: world.Position{X: 2, Layer: 0}}); err != nil {
		t.Fatal(err)
	}
	respawned := rt.Step(3, 50*time.Millisecond)
	if len(respawned.CommandErrors) != 0 || respawned.Metrics.RespawnsApplied != 1 || respawned.Metrics.ReviveProtectionsGranted != 1 {
		t.Fatalf("respawned=%#v", respawned)
	}
	if !rt.isReviveProtected(2, 4) {
		t.Fatal("respawn did not grant protection")
	}

	// Player 2 successfully deals damage first, which cancels its protection. Player 3 can then
	// damage player 2 later in the same owner command phase.
	if err := rt.EnqueueUseAction(2, 1, protocol.ClientUseAction{ActionID: "basic-attack", TargetKind: protocol.ActionTargetEntity, TargetID: "1"}); err != nil {
		t.Fatal(err)
	}
	if err := rt.EnqueueUseAction(3, 1, protocol.ClientUseAction{ActionID: "basic-attack", TargetKind: protocol.ActionTargetEntity, TargetID: "2"}); err != nil {
		t.Fatal(err)
	}
	report := rt.Step(4, 50*time.Millisecond)
	if len(report.CommandErrors) != 0 || len(report.ActionRejections) != 0 {
		t.Fatalf("damage exchange=%#v", report)
	}
	if report.Metrics.ReviveProtectionsCancelledByDamageAction != 1 {
		t.Fatalf("cancel metrics=%#v", report.Metrics)
	}
	state, _ := rt.characters.State(2)
	if !state.Defeated || state.HP != 0 {
		t.Fatalf("player 2 remained protected after attacking=%#v", state)
	}
}

func TestRejectedDamageDoesNotCancelProtectionAndLeaveCleansState(t *testing.T) {
	rt, _ := makeResurrectionRuntime(t)
	rt.config.PostReviveProtectionTicks = 5
	rt.Step(1, 50*time.Millisecond)

	if err := rt.EnqueueUseAction(1, 1, protocol.ClientUseAction{ActionID: "basic-attack", TargetKind: protocol.ActionTargetEntity, TargetID: "2"}); err != nil {
		t.Fatal(err)
	}
	rt.Step(2, 50*time.Millisecond)
	if err := rt.EnqueueRespawn(RespawnRequest{EntityID: 2, Position: world.Position{X: 2, Layer: 0}}); err != nil {
		t.Fatal(err)
	}
	rt.Step(3, 50*time.Millisecond)

	// Self-target rejection is not a successful damage action, so grace must remain.
	if err := rt.EnqueueUseAction(2, 1, protocol.ClientUseAction{ActionID: "basic-attack", TargetKind: protocol.ActionTargetEntity, TargetID: "2"}); err != nil {
		t.Fatal(err)
	}
	if err := rt.EnqueueUseAction(3, 1, protocol.ClientUseAction{ActionID: "basic-attack", TargetKind: protocol.ActionTargetEntity, TargetID: "2"}); err != nil {
		t.Fatal(err)
	}
	report := rt.Step(4, 50*time.Millisecond)
	if len(report.ActionRejections) != 2 {
		t.Fatalf("rejections=%#v", report.ActionRejections)
	}
	if !errors.Is(report.ActionRejections[0].Err, ErrSelfTarget) || !errors.Is(report.ActionRejections[1].Err, ErrEntityReviveProtected) {
		t.Fatalf("rejections=%#v", report.ActionRejections)
	}
	if report.Metrics.ReviveProtectionsCancelledByDamageAction != 0 || report.Metrics.ReviveProtectionDamageBlocks != 1 {
		t.Fatalf("metrics=%#v", report.Metrics)
	}

	if err := rt.EnqueueLeave(2); err != nil {
		t.Fatal(err)
	}
	left := rt.Step(5, 50*time.Millisecond)
	if len(left.CommandErrors) != 0 {
		t.Fatalf("leave=%#v", left.CommandErrors)
	}
	if _, ok := rt.reviveProtectionUntil[2]; ok {
		t.Fatal("leave retained revive protection state")
	}
}
