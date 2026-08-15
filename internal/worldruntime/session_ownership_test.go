package worldruntime

import (
	"errors"
	"testing"
	"time"

	"github.com/li41/astrahold-server/internal/protocol"
	"github.com/li41/astrahold-server/internal/session"
)

func TestAwaitJoinOwnedMintsTrustedOwnershipFence(t *testing.T) {
	rt, identity := newIdentityRuntime(t)
	request := identityJoinRequest(t, 1, 1, identity)
	result := make(chan struct {
		fence SessionOwnershipFence
		err   error
	}, 1)
	go func() {
		fence, err := rt.AwaitJoinOwned(nil, request)
		result <- struct {
			fence SessionOwnershipFence
			err   error
		}{fence: fence, err: err}
	}()
	waitForCommandDepthAtLeast(t, rt, 1)
	if report := rt.Step(1, 50*time.Millisecond); len(report.CommandErrors) != 0 {
		t.Fatalf("join errors=%#v", report.CommandErrors)
	}
	got := <-result
	if got.err != nil {
		t.Fatal(got.err)
	}
	if !got.fence.Valid() || got.fence.SessionID != 1 || got.fence.EntityID != 1 || got.fence.CharacterID != identity.ID {
		t.Fatalf("ownership fence=%#v", got.fence)
	}
	if current := rt.characterIdentities.ownershipByCharacter[identity.ID]; current != got.fence {
		t.Fatalf("character ownership=%#v want=%#v", current, got.fence)
	}
	if current := rt.characterIdentities.ownershipBySession[1]; current != got.fence {
		t.Fatalf("session ownership=%#v want=%#v", current, got.fence)
	}
}

func TestStaleFencedMoveDoesNotConsumeInputSequence(t *testing.T) {
	rt, fence, s := joinOwnedIdentitySession(t)
	installNewerOwnershipForTest(rt, fence)

	if err := rt.EnqueueFencedMove(fence, 1, protocol.ClientMoveInput{DirectionX: 1}); err != nil {
		t.Fatal(err)
	}
	report := rt.Step(2, 50*time.Millisecond)
	if len(report.CommandErrors) != 1 || !errors.Is(report.CommandErrors[0].Err, ErrCharacterOwnershipFenceStale) {
		t.Fatalf("move errors=%#v", report.CommandErrors)
	}
	if s.LastProcessedInputSequence() != 0 {
		t.Fatalf("stale move consumed input sequence=%d", s.LastProcessedInputSequence())
	}
}

func TestStaleFencedActionDoesNotConsumeActionSequence(t *testing.T) {
	rt, fence, s := joinOwnedIdentitySession(t)
	installNewerOwnershipForTest(rt, fence)

	action := protocol.ClientUseAction{ActionID: "basic-attack", TargetKind: protocol.ActionTargetGate, TargetID: "main-gate"}
	if err := rt.EnqueueFencedUseAction(fence, 1, action); err != nil {
		t.Fatal(err)
	}
	report := rt.Step(2, 50*time.Millisecond)
	if len(report.CommandErrors) != 1 || !errors.Is(report.CommandErrors[0].Err, ErrCharacterOwnershipFenceStale) {
		t.Fatalf("action errors=%#v", report.CommandErrors)
	}
	if err := s.ValidateActionSequence(1); err != nil {
		t.Fatalf("stale action consumed sequence: %v", err)
	}
}

func TestStaleFencedLeaveDoesNotRemoveSessionOrEntity(t *testing.T) {
	rt, fence, _ := joinOwnedIdentitySession(t)
	installNewerOwnershipForTest(rt, fence)

	if err := rt.EnqueueFencedLeave(fence); err != nil {
		t.Fatal(err)
	}
	report := rt.Step(2, 50*time.Millisecond)
	if len(report.CommandErrors) != 1 || !errors.Is(report.CommandErrors[0].Err, ErrCharacterOwnershipFenceStale) {
		t.Fatalf("leave errors=%#v", report.CommandErrors)
	}
	if _, ok := rt.sessions.Get(fence.SessionID); !ok {
		t.Fatal("stale leave removed old session before a transfer stage owns cleanup")
	}
	if _, ok := rt.world.Entity(fence.EntityID); !ok {
		t.Fatal("stale leave removed world entity")
	}
}

func TestCurrentFencedLeaveRemovesOwnedSession(t *testing.T) {
	rt, fence, _ := joinOwnedIdentitySession(t)
	if err := rt.EnqueueFencedLeave(fence); err != nil {
		t.Fatal(err)
	}
	if report := rt.Step(2, 50*time.Millisecond); len(report.CommandErrors) != 0 {
		t.Fatalf("leave errors=%#v", report.CommandErrors)
	}
	if _, ok := rt.sessions.Get(fence.SessionID); ok {
		t.Fatal("current fenced leave left session active")
	}
	if _, ok := rt.world.Entity(fence.EntityID); ok {
		t.Fatal("current fenced leave left world entity active")
	}
	if _, ok := rt.characterIdentities.ownershipBySession[fence.SessionID]; ok {
		t.Fatal("current fenced leave left ownership-by-session state")
	}
	if _, ok := rt.characterIdentities.ownershipByCharacter[fence.CharacterID]; ok {
		t.Fatal("current fenced leave left ownership-by-character state")
	}
}

func joinOwnedIdentitySession(t *testing.T) (*Runtime, SessionOwnershipFence, *session.Session) {
	t.Helper()
	rt, identity := newIdentityRuntime(t)
	request := identityJoinRequest(t, 1, 1, identity)
	result := make(chan struct {
		fence SessionOwnershipFence
		err   error
	}, 1)
	go func() {
		fence, err := rt.AwaitJoinOwned(nil, request)
		result <- struct {
			fence SessionOwnershipFence
			err   error
		}{fence: fence, err: err}
	}()
	waitForCommandDepthAtLeast(t, rt, 1)
	if report := rt.Step(1, 50*time.Millisecond); len(report.CommandErrors) != 0 {
		t.Fatalf("join errors=%#v", report.CommandErrors)
	}
	got := <-result
	if got.err != nil {
		t.Fatal(got.err)
	}
	if !got.fence.Valid() {
		t.Fatalf("ownership fence=%#v", got.fence)
	}
	return rt, got.fence, request.Session
}

func installNewerOwnershipForTest(rt *Runtime, old SessionOwnershipFence) SessionOwnershipFence {
	newer := old
	newer.SessionID++
	newer.EntityID++
	newer.Epoch++
	delete(rt.characterIdentities.ownershipBySession, old.SessionID)
	rt.characterIdentities.ownershipByCharacter[old.CharacterID] = newer
	rt.characterIdentities.ownershipBySession[newer.SessionID] = newer
	return newer
}
