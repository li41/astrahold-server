package worldruntime

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/li41/astrahold-server/internal/characteridentity"
	"github.com/li41/astrahold-server/internal/characterstate"
	"github.com/li41/astrahold-server/internal/session"
	"github.com/li41/astrahold-server/internal/world"
)

type admissionResult struct {
	lease CharacterAdmissionLease
	err   error
}

func awaitAdmissionResult(rt *Runtime, identity characteridentity.Binding) <-chan admissionResult {
	result := make(chan admissionResult, 1)
	go func() {
		lease, err := rt.AwaitCharacterAdmission(context.Background(), identity)
		result <- admissionResult{lease: lease, err: err}
	}()
	return result
}

func TestCharacterAdmissionRejectsStillActiveTrustedIdentity(t *testing.T) {
	rt, identity := newIdentityRuntime(t)
	joinIdentityPlayer(t, rt, 1, 1, identity)
	if report := rt.Step(1, 50*time.Millisecond); len(report.CommandErrors) != 0 {
		t.Fatalf("join errors=%#v", report.CommandErrors)
	}

	result := awaitAdmissionResult(rt, identity)
	waitForCommandDepthAtLeast(t, rt, 1)
	report := rt.Step(2, 50*time.Millisecond)
	if got := <-result; !errors.Is(got.err, ErrCharacterIdentityActive) || got.lease.Valid() {
		t.Fatalf("admission result=%#v", got)
	}
	if len(report.CommandErrors) != 1 || report.CommandErrors[0].Command != "admit_character" || !errors.Is(report.CommandErrors[0].Err, ErrCharacterIdentityActive) {
		t.Fatalf("report errors=%#v", report.CommandErrors)
	}
}

func TestCharacterAdmissionRunsAfterEarlierLeaveCaptureAndReserves(t *testing.T) {
	outbox, err := characterstate.NewOutbox(4)
	if err != nil {
		t.Fatal(err)
	}
	rt := makeCharacterStateRuntime(t, outbox)
	identity, err := characteridentity.NewTrusted("character:admission-after-leave")
	if err != nil {
		t.Fatal(err)
	}
	conn := session.NewQueueConnection(32, 32)
	s, err := session.NewWithCharacterIdentity(1, 1, identity, 64, conn)
	if err != nil {
		t.Fatal(err)
	}
	if err := rt.EnqueueRegister(s); err != nil {
		t.Fatal(err)
	}
	if report := rt.Step(1, 50*time.Millisecond); len(report.CommandErrors) != 0 {
		t.Fatalf("register errors=%#v", report.CommandErrors)
	}

	if err := rt.EnqueueLeave(1); err != nil {
		t.Fatal(err)
	}
	result := awaitAdmissionResult(rt, identity)
	waitForCommandDepthAtLeast(t, rt, 2)
	report := rt.Step(2, 50*time.Millisecond)
	got := <-result
	if got.err != nil || !got.lease.Valid() {
		t.Fatalf("admission result=%#v", got)
	}
	if len(report.CommandErrors) != 0 {
		t.Fatalf("step errors=%#v", report.CommandErrors)
	}
	pending := outbox.Pending(0)
	if len(pending) != 1 || pending[0].Identity != identity {
		t.Fatalf("pending=%#v", pending)
	}
	if _, ok := rt.characterIdentities.entityByCharacter[identity.ID]; ok {
		t.Fatal("admission completed before active ownership release")
	}
	current, ok := rt.characterIdentities.admissionByCharacter[identity.ID]
	if !ok || current.Generation != got.lease.Generation {
		t.Fatalf("reservation=%#v lease=%#v", current, got.lease)
	}
}

func TestAwaitJoinReportsWorldOwnerDuplicateIdentityFailure(t *testing.T) {
	rt, identity := newIdentityRuntime(t)
	joinIdentityPlayer(t, rt, 1, 1, identity)
	if report := rt.Step(1, 50*time.Millisecond); len(report.CommandErrors) != 0 {
		t.Fatalf("join1 errors=%#v", report.CommandErrors)
	}

	request := identityJoinRequest(t, 2, 2, identity)
	result := make(chan error, 1)
	go func() { result <- rt.AwaitJoin(context.Background(), request) }()
	waitForCommandDepthAtLeast(t, rt, 1)
	report := rt.Step(2, 50*time.Millisecond)
	if err := <-result; !errors.Is(err, ErrCharacterIdentityActive) {
		t.Fatalf("join error=%v", err)
	}
	if len(report.CommandErrors) != 1 || !errors.Is(report.CommandErrors[0].Err, ErrCharacterIdentityActive) {
		t.Fatalf("report errors=%#v", report.CommandErrors)
	}
	if _, ok := rt.world.Entity(2); ok {
		t.Fatal("rejected synchronous join spawned entity")
	}
}

func TestConcurrentTrustedCandidatesOnlyOneGetsAdmissionLease(t *testing.T) {
	rt, identity := newIdentityRuntime(t)
	admissionA := awaitAdmissionResult(rt, identity)
	admissionB := awaitAdmissionResult(rt, identity)
	waitForCommandDepthAtLeast(t, rt, 2)
	report := rt.Step(1, 50*time.Millisecond)
	resultA := <-admissionA
	resultB := <-admissionB

	var winner CharacterAdmissionLease
	switch {
	case resultA.err == nil && errors.Is(resultB.err, ErrCharacterAdmissionReserved):
		winner = resultA.lease
	case resultB.err == nil && errors.Is(resultA.err, ErrCharacterAdmissionReserved):
		winner = resultB.lease
	default:
		t.Fatalf("admission results A=%#v B=%#v", resultA, resultB)
	}
	if !winner.Valid() {
		t.Fatalf("winner lease=%#v", winner)
	}
	if len(report.CommandErrors) != 1 || !errors.Is(report.CommandErrors[0].Err, ErrCharacterAdmissionReserved) {
		t.Fatalf("report errors=%#v", report.CommandErrors)
	}

	request := identityJoinRequest(t, 1, 1, identity)
	request.AdmissionLease = &winner
	joinResult := make(chan error, 1)
	go func() { joinResult <- rt.AwaitJoin(context.Background(), request) }()
	waitForCommandDepthAtLeast(t, rt, 1)
	if report := rt.Step(2, 50*time.Millisecond); len(report.CommandErrors) != 0 {
		t.Fatalf("join errors=%#v", report.CommandErrors)
	}
	if err := <-joinResult; err != nil {
		t.Fatalf("reserved join=%v", err)
	}
	if len(rt.sessions.List()) != 1 {
		t.Fatalf("active sessions=%d", len(rt.sessions.List()))
	}
	if _, ok := rt.characterIdentities.admissionByCharacter[identity.ID]; ok {
		t.Fatal("successful reserved join did not consume lease")
	}
}

func TestLiveAdmissionLeaseBlocksUnreservedTrustedJoin(t *testing.T) {
	rt, identity := newIdentityRuntime(t)
	admission := awaitAdmissionResult(rt, identity)
	waitForCommandDepthAtLeast(t, rt, 1)
	if report := rt.Step(1, 50*time.Millisecond); len(report.CommandErrors) != 0 {
		t.Fatalf("admission errors=%#v", report.CommandErrors)
	}
	lease := (<-admission).lease

	request := identityJoinRequest(t, 1, 1, identity)
	result := make(chan error, 1)
	go func() { result <- rt.AwaitJoin(context.Background(), request) }()
	waitForCommandDepthAtLeast(t, rt, 1)
	report := rt.Step(2, 50*time.Millisecond)
	if err := <-result; !errors.Is(err, ErrCharacterAdmissionLeaseRequired) {
		t.Fatalf("unreserved join=%v", err)
	}
	if len(report.CommandErrors) != 1 || !errors.Is(report.CommandErrors[0].Err, ErrCharacterAdmissionLeaseRequired) {
		t.Fatalf("report errors=%#v", report.CommandErrors)
	}

	request.AdmissionLease = &lease
	go func() { result <- rt.AwaitJoin(context.Background(), request) }()
	waitForCommandDepthAtLeast(t, rt, 1)
	if report := rt.Step(3, 50*time.Millisecond); len(report.CommandErrors) != 0 {
		t.Fatalf("reserved join errors=%#v", report.CommandErrors)
	}
	if err := <-result; err != nil {
		t.Fatalf("reserved join=%v", err)
	}
}

func TestAdmissionReleaseAllowsNewGenerationAndStaleReleaseCannotClearIt(t *testing.T) {
	rt, identity := newIdentityRuntime(t)
	firstResult := awaitAdmissionResult(rt, identity)
	waitForCommandDepthAtLeast(t, rt, 1)
	rt.Step(1, 50*time.Millisecond)
	first := <-firstResult
	if first.err != nil || !first.lease.Valid() {
		t.Fatalf("first=%#v", first)
	}

	releaseDone := make(chan error, 1)
	go func() { releaseDone <- rt.ReleaseCharacterAdmission(context.Background(), first.lease) }()
	waitForCommandDepthAtLeast(t, rt, 1)
	if report := rt.Step(2, 50*time.Millisecond); len(report.CommandErrors) != 0 {
		t.Fatalf("release errors=%#v", report.CommandErrors)
	}
	if err := <-releaseDone; err != nil {
		t.Fatalf("release=%v", err)
	}

	secondResult := awaitAdmissionResult(rt, identity)
	waitForCommandDepthAtLeast(t, rt, 1)
	rt.Step(3, 50*time.Millisecond)
	second := <-secondResult
	if second.err != nil || second.lease.Generation <= first.lease.Generation {
		t.Fatalf("second=%#v first=%#v", second, first)
	}

	go func() { releaseDone <- rt.ReleaseCharacterAdmission(context.Background(), first.lease) }()
	waitForCommandDepthAtLeast(t, rt, 1)
	if report := rt.Step(4, 50*time.Millisecond); len(report.CommandErrors) != 0 {
		t.Fatalf("stale release errors=%#v", report.CommandErrors)
	}
	if err := <-releaseDone; err != nil {
		t.Fatalf("stale release=%v", err)
	}
	current, ok := rt.characterIdentities.admissionByCharacter[identity.ID]
	if !ok || current.Generation != second.lease.Generation {
		t.Fatalf("stale release cleared newer lease: current=%#v second=%#v", current, second.lease)
	}
}

func TestExpiredAdmissionLeaseCanBeReplaced(t *testing.T) {
	rt, identity := newIdentityRuntime(t)
	firstResult := awaitAdmissionResult(rt, identity)
	waitForCommandDepthAtLeast(t, rt, 1)
	rt.Step(1, 50*time.Millisecond)
	first := <-firstResult
	current := rt.characterIdentities.admissionByCharacter[identity.ID]
	current.ExpiresAt = time.Now().Add(-time.Second)
	rt.characterIdentities.admissionByCharacter[identity.ID] = current

	secondResult := awaitAdmissionResult(rt, identity)
	waitForCommandDepthAtLeast(t, rt, 1)
	if report := rt.Step(2, 50*time.Millisecond); len(report.CommandErrors) != 0 {
		t.Fatalf("replacement errors=%#v", report.CommandErrors)
	}
	second := <-secondResult
	if second.err != nil || second.lease.Generation <= first.lease.Generation {
		t.Fatalf("first=%#v second=%#v", first, second)
	}
}

func identityJoinRequest(t *testing.T, sid session.ID, eid world.EntityID, identity characteridentity.Binding) JoinRequest {
	t.Helper()
	conn := session.NewQueueConnection(32, 32)
	s, err := session.NewWithCharacterIdentity(sid, eid, identity, 20, conn)
	if err != nil {
		t.Fatal(err)
	}
	return JoinRequest{
		Session: s,
		Entity: world.EntityState{
			ID:        eid,
			Kind:      world.EntityPlayer,
			Transform: world.Transform{Position: world.Position{Layer: 0}},
		},
		Speed: 6, Radius: 0.35, MaxStepHeight: 0.5,
	}
}

func waitForCommandDepthAtLeast(t *testing.T, rt *Runtime, want int) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for rt.queue.depth() < want {
		if time.Now().After(deadline) {
			t.Fatalf("command queue depth=%d want>=%d", rt.queue.depth(), want)
		}
		time.Sleep(time.Millisecond)
	}
}
