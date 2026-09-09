package worldruntime

import (
	"errors"
	"testing"
	"time"

	"github.com/li41/astrahold-server/internal/character"
	"github.com/li41/astrahold-server/internal/characteridentity"
	"github.com/li41/astrahold-server/internal/characterstate"
	"github.com/li41/astrahold-server/internal/classid"
	"github.com/li41/astrahold-server/internal/movement"
	"github.com/li41/astrahold-server/internal/navigation"
	"github.com/li41/astrahold-server/internal/session"
	"github.com/li41/astrahold-server/internal/simulation"
	"github.com/li41/astrahold-server/internal/spatial"
	"github.com/li41/astrahold-server/internal/world"
)

func TestInitialClassAssignmentCommitsOnlyAfterDurableCompletion(t *testing.T) {
	rt, outbox, fence := joinInitialClassAssignmentCharacter(t)
	if state, _ := rt.characters.State(fence.EntityID); state.ClassID != "" {
		t.Fatalf("fresh class=%q", state.ClassID)
	}

	if err := rt.EnqueueFencedInitialClassAssignment(fence, classid.Oathguard); err != nil {
		t.Fatal(err)
	}
	report := rt.Step(2, 50*time.Millisecond)
	if len(report.CommandErrors) != 0 {
		t.Fatalf("prepare errors=%#v", report.CommandErrors)
	}
	state, ok := rt.characters.State(fence.EntityID)
	if !ok || state.ClassID != "" {
		t.Fatalf("class mutated before durability state=%#v ok=%v", state, ok)
	}
	pending := outbox.Pending(1)
	if len(pending) != 1 || !pending[0].CompletionRequested || pending[0].Snapshot.ClassID != classid.Oathguard {
		t.Fatalf("pending=%#v", pending)
	}
	intent := pending[0]
	if reserved, ok := outbox.CompletionForCharacter(fence.CharacterID); !ok || reserved != intent {
		t.Fatalf("reservation=%#v ok=%v", reserved, ok)
	}

	if err := rt.EnqueueFencedInitialClassAssignment(fence, classid.Breaker); err != nil {
		t.Fatal(err)
	}
	report = rt.Step(3, 50*time.Millisecond)
	if len(report.CommandErrors) != 1 || !errors.Is(report.CommandErrors[0].Err, ErrInitialClassAssignmentPending) {
		t.Fatalf("duplicate errors=%#v", report.CommandErrors)
	}
	if state, _ := rt.characters.State(fence.EntityID); state.ClassID != "" {
		t.Fatalf("duplicate mutated class=%q", state.ClassID)
	}

	_, projected, ok := rt.captureCharacterStateSnapshot(fence.SessionID, fence.EntityID, &StepReport{Tick: 3})
	if !ok || projected.ClassID != classid.Oathguard {
		t.Fatalf("pending save projection=%#v ok=%v", projected, ok)
	}

	// Runtime tests simulate the persistence worker only at its post-checkpoint boundary:
	// journal Confirm removes the pending save head, Complete publishes the durable acknowledgement.
	if err := outbox.Confirm(intent.IntentID); err != nil {
		t.Fatal(err)
	}
	outbox.Complete(intent)
	if state, _ := rt.characters.State(fence.EntityID); state.ClassID != "" {
		t.Fatalf("durable worker directly mutated gameplay class=%q", state.ClassID)
	}

	report = rt.Step(4, 50*time.Millisecond)
	if len(report.CommandErrors) != 0 {
		t.Fatalf("completion errors=%#v", report.CommandErrors)
	}
	state, ok = rt.characters.State(fence.EntityID)
	if !ok || state.ClassID != classid.Oathguard {
		t.Fatalf("completed state=%#v ok=%v", state, ok)
	}
	if outbox.CompletionDepth() != 0 {
		t.Fatalf("completion depth=%d", outbox.CompletionDepth())
	}
	if _, ok := outbox.CompletionForCharacter(fence.CharacterID); ok {
		t.Fatal("completed assignment retained transaction reservation")
	}

	if err := rt.EnqueueFencedInitialClassAssignment(fence, classid.Breaker); err != nil {
		t.Fatal(err)
	}
	report = rt.Step(5, 50*time.Millisecond)
	if len(report.CommandErrors) != 1 || !errors.Is(report.CommandErrors[0].Err, character.ErrClassAlreadyAssigned) {
		t.Fatalf("transfer errors=%#v", report.CommandErrors)
	}
	if state, _ := rt.characters.State(fence.EntityID); state.ClassID != classid.Oathguard {
		t.Fatalf("transfer changed class=%q", state.ClassID)
	}
}

func TestInitialClassAssignmentRejectsStaleOwnershipAndMissingPersistence(t *testing.T) {
	rt, _, fence := joinInitialClassAssignmentCharacter(t)
	installNewerOwnershipForTest(rt, fence)
	if err := rt.EnqueueFencedInitialClassAssignment(fence, classid.Ranger); err != nil {
		t.Fatal(err)
	}
	report := rt.Step(2, 50*time.Millisecond)
	if len(report.CommandErrors) != 1 || !errors.Is(report.CommandErrors[0].Err, ErrCharacterOwnershipFenceStale) {
		t.Fatalf("stale errors=%#v", report.CommandErrors)
	}
	if state, _ := rt.characters.State(fence.EntityID); state.ClassID != "" {
		t.Fatalf("stale owner changed class=%q", state.ClassID)
	}

	plain, plainFence, _ := joinOwnedIdentitySession(t)
	if err := plain.EnqueueFencedInitialClassAssignment(plainFence, classid.Ranger); err != nil {
		t.Fatal(err)
	}
	report = plain.Step(2, 50*time.Millisecond)
	if len(report.CommandErrors) != 1 || !errors.Is(report.CommandErrors[0].Err, ErrInitialClassAssignmentRequiresPersistence) {
		t.Fatalf("missing persistence errors=%#v", report.CommandErrors)
	}
}

func TestDurableInitialClassCompletionAfterLeaveResolvesWithoutRespawn(t *testing.T) {
	rt, outbox, fence := joinInitialClassAssignmentCharacter(t)
	if err := rt.EnqueueFencedInitialClassAssignment(fence, classid.Shadowblade); err != nil {
		t.Fatal(err)
	}
	if report := rt.Step(2, 50*time.Millisecond); len(report.CommandErrors) != 0 {
		t.Fatalf("prepare errors=%#v", report.CommandErrors)
	}
	intent := outbox.Pending(1)[0]

	if err := rt.EnqueueFencedLeave(fence); err != nil {
		t.Fatal(err)
	}
	if report := rt.Step(3, 50*time.Millisecond); len(report.CommandErrors) != 0 {
		t.Fatalf("leave errors=%#v", report.CommandErrors)
	}
	if _, ok := rt.characters.State(fence.EntityID); ok {
		t.Fatal("leave kept character state active")
	}
	pending := outbox.Pending(0)
	if len(pending) != 2 || pending[1].Snapshot.ClassID != classid.Shadowblade {
		t.Fatalf("leave did not preserve pending target=%#v", pending)
	}

	if err := outbox.Confirm(intent.IntentID); err != nil {
		t.Fatal(err)
	}
	outbox.Complete(intent)
	report := rt.Step(4, 50*time.Millisecond)
	if len(report.CommandErrors) != 0 {
		t.Fatalf("offline completion errors=%#v", report.CommandErrors)
	}
	if outbox.CompletionDepth() != 0 {
		t.Fatalf("offline completion depth=%d", outbox.CompletionDepth())
	}
	if _, ok := rt.world.Entity(fence.EntityID); ok {
		t.Fatal("offline durable completion respawned world entity")
	}
	if _, ok := rt.characters.State(fence.EntityID); ok {
		t.Fatal("offline durable completion recreated character state")
	}
}

func joinInitialClassAssignmentCharacter(t *testing.T) (*Runtime, *characterstate.Outbox, SessionOwnershipFence) {
	t.Helper()
	outbox, err := characterstate.NewOutbox(8)
	if err != nil {
		t.Fatal(err)
	}
	nav := navigation.Plane{MinX: -100, MaxX: 100, MinZ: -100, MaxZ: 100, Layer: 0}
	sim := simulation.New(spatial.NewGrid(16), movement.NewService(nav, 0.1))
	cfg := DefaultConfig()
	cfg.SnapshotEveryTicks = 1000
	cfg.CharacterStateAutosaveEveryTicks = 0
	rt := New(sim, cfg, WithCharacterStateOutbox(outbox, characterStateTestWorld))
	identity, err := characteridentity.NewTrusted("character:initial-class")
	if err != nil {
		t.Fatal(err)
	}
	conn := session.NewQueueConnection(32, 32)
	sess, err := session.NewWithCharacterIdentity(1, 1, identity, 64, conn)
	if err != nil {
		t.Fatal(err)
	}
	request := JoinRequest{
		Session: sess,
		Entity: world.EntityState{ID: 1, Kind: world.EntityPlayer, Transform: world.Transform{Position: world.Position{Layer: 0}}},
		Speed: 6, Radius: 0.35, MaxStepHeight: 0.5,
	}
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
	if got.err != nil || !got.fence.Valid() {
		t.Fatalf("fence=%#v err=%v", got.fence, got.err)
	}
	return rt, outbox, got.fence
}
