package worldruntime

import (
	"errors"
	"testing"
	"time"

	"github.com/li41/astrahold-server/internal/characteridentity"
	"github.com/li41/astrahold-server/internal/protocol"
	"github.com/li41/astrahold-server/internal/session"
	"github.com/li41/astrahold-server/internal/world"
)

func TestAwaitCharacterOwnershipReturnsCurrentFence(t *testing.T) {
	rt, fence, _ := joinOwnedIdentitySession(t)
	identity := rt.characterIdentities.byEntity[fence.EntityID]
	result := make(chan struct {
		fence SessionOwnershipFence
		err   error
	}, 1)
	go func() {
		got, err := rt.AwaitCharacterOwnership(nil, identity)
		result <- struct {
			fence SessionOwnershipFence
			err   error
		}{got, err}
	}()
	waitForCommandDepthAtLeast(t, rt, 1)
	if report := rt.Step(2, 50*time.Millisecond); len(report.CommandErrors) != 0 {
		t.Fatalf("lookup errors=%#v", report.CommandErrors)
	}
	got := <-result
	if got.err != nil {
		t.Fatal(got.err)
	}
	if got.fence != fence {
		t.Fatalf("lookup fence=%#v want=%#v", got.fence, fence)
	}
}

func TestOwnershipTransferPreservesEntityStateAndAdvancesEpoch(t *testing.T) {
	rt, oldFence, oldSession := joinOwnedIdentitySession(t)
	identity := oldSession.CharacterIdentity
	if err := rt.world.Teleport(oldFence.EntityID, world.Position{X: 7, Y: 2, Z: -3, Layer: 1}); err != nil {
		t.Fatal(err)
	}
	beforeEntity, ok := rt.world.Entity(oldFence.EntityID)
	if !ok {
		t.Fatal("missing entity before transfer")
	}
	beforeCharacter, ok := rt.characters.State(oldFence.EntityID)
	if !ok {
		t.Fatal("missing character before transfer")
	}
	replacement := newTransferSession(t, 2, oldFence.EntityID, identity)

	result := awaitTransferResult(rt, oldFence, replacement)
	waitForCommandDepthAtLeast(t, rt, 1)
	if report := rt.Step(2, 50*time.Millisecond); len(report.CommandErrors) != 0 {
		t.Fatalf("transfer errors=%#v", report.CommandErrors)
	}
	got := <-result
	if got.err != nil {
		t.Fatal(got.err)
	}
	if !got.fence.Valid() || got.fence.SessionID != replacement.ID || got.fence.EntityID != oldFence.EntityID || got.fence.CharacterID != oldFence.CharacterID || got.fence.Epoch <= oldFence.Epoch {
		t.Fatalf("new ownership=%#v old=%#v", got.fence, oldFence)
	}
	if _, ok := rt.sessions.Get(oldFence.SessionID); ok {
		t.Fatal("old session remained registered")
	}
	if current, ok := rt.sessions.Get(replacement.ID); !ok || current != replacement {
		t.Fatalf("replacement session=%#v ok=%v", current, ok)
	}
	afterEntity, ok := rt.world.Entity(oldFence.EntityID)
	if !ok || afterEntity.Transform != beforeEntity.Transform {
		t.Fatalf("entity changed across transfer: before=%#v after=%#v ok=%v", beforeEntity, afterEntity, ok)
	}
	afterCharacter, ok := rt.characters.State(oldFence.EntityID)
	if !ok || afterCharacter != beforeCharacter {
		t.Fatalf("character changed across transfer: before=%#v after=%#v ok=%v", beforeCharacter, afterCharacter, ok)
	}
	if current := rt.characterIdentities.ownershipByCharacter[oldFence.CharacterID]; current != got.fence {
		t.Fatalf("character ownership=%#v want=%#v", current, got.fence)
	}
	if _, ok := rt.characterIdentities.ownershipBySession[oldFence.SessionID]; ok {
		t.Fatal("old ownership-by-session entry remained")
	}
	if current := rt.characterIdentities.ownershipBySession[replacement.ID]; current != got.fence {
		t.Fatalf("replacement ownership=%#v want=%#v", current, got.fence)
	}
	oldConn, ok := oldSession.Connection().(*session.QueueConnection)
	if !ok {
		t.Fatal("old connection is not QueueConnection")
	}
	select {
	case <-oldConn.Done():
		t.Fatal("world-owner transfer closed old transport")
	default:
	}
}

func TestOwnershipTransferClearsPreviousMoveIntentBeforeSimulation(t *testing.T) {
	rt, oldFence, oldSession := joinOwnedIdentitySession(t)
	replacement := newTransferSession(t, 2, oldFence.EntityID, oldSession.CharacterIdentity)
	before, _ := rt.world.Entity(oldFence.EntityID)
	if err := rt.EnqueueFencedMove(oldFence, 1, protocol.ClientMoveInput{DirectionX: 1}); err != nil {
		t.Fatal(err)
	}
	result := awaitTransferResult(rt, oldFence, replacement)
	waitForCommandDepthAtLeast(t, rt, 2)
	if report := rt.Step(2, time.Second); len(report.CommandErrors) != 0 {
		t.Fatalf("step errors=%#v", report.CommandErrors)
	}
	if got := <-result; got.err != nil {
		t.Fatal(got.err)
	}
	after, _ := rt.world.Entity(oldFence.EntityID)
	if after.Transform.Position != before.Transform.Position {
		t.Fatalf("old movement intent crossed handoff: before=%#v after=%#v", before.Transform.Position, after.Transform.Position)
	}
	if oldSession.LastProcessedInputSequence() != 1 {
		t.Fatalf("pre-transfer old move was not processed in FIFO order: seq=%d", oldSession.LastProcessedInputSequence())
	}
}

func TestConcurrentOwnershipTransfersFromSameFenceOnlyOneWins(t *testing.T) {
	rt, oldFence, oldSession := joinOwnedIdentitySession(t)
	first := newTransferSession(t, 2, oldFence.EntityID, oldSession.CharacterIdentity)
	second := newTransferSession(t, 3, oldFence.EntityID, oldSession.CharacterIdentity)
	firstResult := awaitTransferResult(rt, oldFence, first)
	secondResult := awaitTransferResult(rt, oldFence, second)
	waitForCommandDepthAtLeast(t, rt, 2)
	report := rt.Step(2, 50*time.Millisecond)
	if len(report.CommandErrors) != 1 || !errors.Is(report.CommandErrors[0].Err, ErrCharacterOwnershipFenceStale) {
		t.Fatalf("transfer errors=%#v", report.CommandErrors)
	}
	one := <-firstResult
	two := <-secondResult
	successes := 0
	failures := 0
	var winner SessionOwnershipFence
	for _, result := range []transferResult{one, two} {
		if result.err == nil {
			successes++
			winner = result.fence
		} else if errors.Is(result.err, ErrCharacterOwnershipFenceStale) {
			failures++
		} else {
			t.Fatalf("unexpected transfer result=%#v", result)
		}
	}
	if successes != 1 || failures != 1 {
		t.Fatalf("successes=%d failures=%d first=%#v second=%#v", successes, failures, one, two)
	}
	if current := rt.characterIdentities.ownershipByCharacter[oldFence.CharacterID]; current != winner {
		t.Fatalf("winner=%#v current=%#v", winner, current)
	}
}

func TestTransferredOldFenceCannotLeaveNewOwner(t *testing.T) {
	rt, oldFence, oldSession := joinOwnedIdentitySession(t)
	replacement := newTransferSession(t, 2, oldFence.EntityID, oldSession.CharacterIdentity)
	result := awaitTransferResult(rt, oldFence, replacement)
	waitForCommandDepthAtLeast(t, rt, 1)
	if report := rt.Step(2, 50*time.Millisecond); len(report.CommandErrors) != 0 {
		t.Fatalf("transfer errors=%#v", report.CommandErrors)
	}
	newOwnership := <-result
	if newOwnership.err != nil {
		t.Fatal(newOwnership.err)
	}

	if err := rt.EnqueueFencedLeave(oldFence); err != nil {
		t.Fatal(err)
	}
	report := rt.Step(3, 50*time.Millisecond)
	if len(report.CommandErrors) != 1 || !errors.Is(report.CommandErrors[0].Err, ErrCharacterOwnershipFenceStale) {
		t.Fatalf("stale leave errors=%#v", report.CommandErrors)
	}
	if current, ok := rt.sessions.Get(replacement.ID); !ok || current != replacement {
		t.Fatal("stale old leave removed replacement session")
	}
	if _, ok := rt.world.Entity(oldFence.EntityID); !ok {
		t.Fatal("stale old leave removed transferred entity")
	}
	if current := rt.characterIdentities.ownershipByCharacter[oldFence.CharacterID]; current != newOwnership.fence {
		t.Fatalf("stale old leave changed ownership=%#v want=%#v", current, newOwnership.fence)
	}
}

func TestOwnershipTransferRejectsDifferentEntityWithoutMutation(t *testing.T) {
	rt, oldFence, oldSession := joinOwnedIdentitySession(t)
	replacement := newTransferSession(t, 2, oldFence.EntityID+1, oldSession.CharacterIdentity)
	result := awaitTransferResult(rt, oldFence, replacement)
	waitForCommandDepthAtLeast(t, rt, 1)
	report := rt.Step(2, 50*time.Millisecond)
	if len(report.CommandErrors) != 1 || !errors.Is(report.CommandErrors[0].Err, ErrCharacterOwnershipTransferEntityMismatch) {
		t.Fatalf("transfer errors=%#v", report.CommandErrors)
	}
	got := <-result
	if !errors.Is(got.err, ErrCharacterOwnershipTransferEntityMismatch) {
		t.Fatalf("transfer result=%#v", got)
	}
	if current, ok := rt.sessions.Get(oldFence.SessionID); !ok || current != oldSession {
		t.Fatal("failed transfer mutated old session ownership")
	}
	if current := rt.characterIdentities.ownershipByCharacter[oldFence.CharacterID]; current != oldFence {
		t.Fatalf("failed transfer changed ownership=%#v", current)
	}
}

type transferResult struct {
	fence SessionOwnershipFence
	err   error
}

func awaitTransferResult(rt *Runtime, expected SessionOwnershipFence, replacement *session.Session) <-chan transferResult {
	result := make(chan transferResult, 1)
	go func() {
		fence, err := rt.AwaitOwnershipTransfer(nil, expected, replacement)
		result <- transferResult{fence: fence, err: err}
	}()
	return result
}

func newTransferSession(t *testing.T, sid session.ID, eid world.EntityID, identity characteridentity.Binding) *session.Session {
	t.Helper()
	s, err := session.NewWithCharacterIdentity(sid, eid, identity, 20, session.NewQueueConnection(32, 32))
	if err != nil {
		t.Fatal(err)
	}
	return s
}
