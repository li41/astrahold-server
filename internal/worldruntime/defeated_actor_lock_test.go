package worldruntime

import (
	"errors"
	"testing"
	"time"

	"github.com/li41/astrahold-server/internal/character"
	"github.com/li41/astrahold-server/internal/protocol"
	"github.com/li41/astrahold-server/internal/session"
)

func TestDefeatedActorStopsExistingMovementAndConsumesFutureMoveSequence(t *testing.T) {
	rt, _, _ := makeCharacterCombatRuntime(t)
	initial := rt.Step(1, 50*time.Millisecond)
	if len(initial.CommandErrors) != 0 {
		t.Fatalf("initial errors=%#v", initial.CommandErrors)
	}

	if err := rt.EnqueueUseAction(1, 1, protocol.ClientUseAction{ActionID: "basic-attack", TargetKind: protocol.ActionTargetEntity, TargetID: "2"}); err != nil {
		t.Fatal(err)
	}
	firstHit := rt.Step(2, 50*time.Millisecond)
	if len(firstHit.CommandErrors) != 0 || len(firstHit.ActionRejections) != 0 {
		t.Fatalf("first hit=%#v", firstHit)
	}

	if err := rt.EnqueueMove(2, 1, protocol.ClientMoveInput{DirectionX: 1}); err != nil {
		t.Fatal(err)
	}
	moving := rt.Step(3, 50*time.Millisecond)
	if len(moving.CommandErrors) != 0 {
		t.Fatalf("moving errors=%#v", moving.CommandErrors)
	}
	beforeDefeat, ok := rt.world.Entity(2)
	if !ok {
		t.Fatal("entity 2 missing before defeat")
	}
	if beforeDefeat.Transform.Position.X <= 2 {
		t.Fatalf("expected pre-defeat movement, x=%f", beforeDefeat.Transform.Position.X)
	}

	if err := rt.EnqueueUseAction(1, 2, protocol.ClientUseAction{ActionID: "basic-attack", TargetKind: protocol.ActionTargetEntity, TargetID: "2"}); err != nil {
		t.Fatal(err)
	}
	defeat := rt.Step(12, 50*time.Millisecond)
	if len(defeat.CommandErrors) != 0 || len(defeat.ActionRejections) != 0 {
		t.Fatalf("defeat=%#v", defeat)
	}
	afterDefeat, ok := rt.world.Entity(2)
	if !ok {
		t.Fatal("entity 2 missing after defeat")
	}
	if afterDefeat.Transform.Position != beforeDefeat.Transform.Position {
		t.Fatalf("lethal transition kept old movement input: before=%#v after=%#v", beforeDefeat.Transform.Position, afterDefeat.Transform.Position)
	}
	state, ok := rt.characters.State(2)
	if !ok || !state.Defeated || state.HP != 0 {
		t.Fatalf("defeated state=%#v ok=%v", state, ok)
	}

	if err := rt.EnqueueMove(2, 2, protocol.ClientMoveInput{DirectionX: 1}); err != nil {
		t.Fatal(err)
	}
	blocked := rt.Step(13, 50*time.Millisecond)
	if len(blocked.CommandErrors) != 0 {
		t.Fatalf("defeated move should be normal gameplay handling, errors=%#v", blocked.CommandErrors)
	}
	afterBlocked, _ := rt.world.Entity(2)
	if afterBlocked.Transform.Position != afterDefeat.Transform.Position {
		t.Fatalf("defeated actor moved: defeated=%#v blocked=%#v", afterDefeat.Transform.Position, afterBlocked.Transform.Position)
	}
	s2, ok := rt.sessions.Get(2)
	if !ok || s2.LastProcessedInputSequence() != 2 {
		t.Fatalf("defeated move sequence not consumed: session=%#v ok=%v", s2, ok)
	}

	if err := rt.EnqueueMove(2, 2, protocol.ClientMoveInput{DirectionX: 1}); err != nil {
		t.Fatal(err)
	}
	stale := rt.Step(14, 50*time.Millisecond)
	if len(stale.CommandErrors) != 1 || !errors.Is(stale.CommandErrors[0].Err, session.ErrStaleInput) {
		t.Fatalf("replayed defeated move=%#v", stale.CommandErrors)
	}
}

func TestDefeatedActorCannotUseActionAndConsumesActionSequence(t *testing.T) {
	rt, _, _ := makeCharacterCombatRuntime(t)
	initial := rt.Step(1, 50*time.Millisecond)
	if len(initial.CommandErrors) != 0 {
		t.Fatalf("initial errors=%#v", initial.CommandErrors)
	}

	if err := rt.EnqueueUseAction(1, 1, protocol.ClientUseAction{ActionID: "basic-attack", TargetKind: protocol.ActionTargetEntity, TargetID: "2"}); err != nil {
		t.Fatal(err)
	}
	if report := rt.Step(2, 50*time.Millisecond); len(report.CommandErrors) != 0 || len(report.ActionRejections) != 0 {
		t.Fatalf("first hit=%#v", report)
	}
	if err := rt.EnqueueUseAction(1, 2, protocol.ClientUseAction{ActionID: "basic-attack", TargetKind: protocol.ActionTargetEntity, TargetID: "2"}); err != nil {
		t.Fatal(err)
	}
	if report := rt.Step(12, 50*time.Millisecond); len(report.CommandErrors) != 0 || len(report.ActionRejections) != 0 {
		t.Fatalf("defeat=%#v", report)
	}

	before, _ := rt.characters.State(1)
	if err := rt.EnqueueUseAction(2, 1, protocol.ClientUseAction{ActionID: "basic-attack", TargetKind: protocol.ActionTargetEntity, TargetID: "1"}); err != nil {
		t.Fatal(err)
	}
	blocked := rt.Step(13, 50*time.Millisecond)
	if len(blocked.CommandErrors) != 0 {
		t.Fatalf("defeated action polluted command errors=%#v", blocked.CommandErrors)
	}
	if len(blocked.ActionRejections) != 1 || !errors.Is(blocked.ActionRejections[0].Err, character.ErrCharacterDefeated) {
		t.Fatalf("defeated action rejection=%#v", blocked.ActionRejections)
	}
	if blocked.Metrics.EntityActionsApplied != 0 {
		t.Fatalf("defeated action applied=%d", blocked.Metrics.EntityActionsApplied)
	}
	after, _ := rt.characters.State(1)
	if after != before {
		t.Fatalf("defeated actor damaged target: before=%#v after=%#v", before, after)
	}

	if err := rt.EnqueueUseAction(2, 1, protocol.ClientUseAction{ActionID: "basic-attack", TargetKind: protocol.ActionTargetEntity, TargetID: "1"}); err != nil {
		t.Fatal(err)
	}
	stale := rt.Step(14, 50*time.Millisecond)
	if len(stale.CommandErrors) != 1 || !errors.Is(stale.CommandErrors[0].Err, session.ErrStaleAction) {
		t.Fatalf("replayed defeated action=%#v", stale.CommandErrors)
	}
}
