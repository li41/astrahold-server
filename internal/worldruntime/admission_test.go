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

func TestCharacterAdmissionRejectsStillActiveTrustedIdentity(t *testing.T) {
	rt, identity := newIdentityRuntime(t)
	joinIdentityPlayer(t, rt, 1, 1, identity)
	if report := rt.Step(1, 50*time.Millisecond); len(report.CommandErrors) != 0 {
		t.Fatalf("join errors=%#v", report.CommandErrors)
	}

	result := make(chan error, 1)
	go func() { result <- rt.AwaitCharacterAdmission(context.Background(), identity) }()
	waitForCommandDepthAtLeast(t, rt, 1)
	report := rt.Step(2, 50*time.Millisecond)
	if err := <-result; !errors.Is(err, ErrCharacterIdentityActive) {
		t.Fatalf("admission error=%v", err)
	}
	if len(report.CommandErrors) != 1 || report.CommandErrors[0].Command != "admit_character" || !errors.Is(report.CommandErrors[0].Err, ErrCharacterIdentityActive) {
		t.Fatalf("report errors=%#v", report.CommandErrors)
	}
}

func TestCharacterAdmissionRunsAfterEarlierLeaveCapture(t *testing.T) {
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
	result := make(chan error, 1)
	go func() { result <- rt.AwaitCharacterAdmission(context.Background(), identity) }()
	waitForCommandDepthAtLeast(t, rt, 2)
	report := rt.Step(2, 50*time.Millisecond)
	if err := <-result; err != nil {
		t.Fatalf("admission error=%v", err)
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

func TestConcurrentTrustedCandidatesStillCommitAtMostOneJoin(t *testing.T) {
	rt, identity := newIdentityRuntime(t)
	admissionA := make(chan error, 1)
	admissionB := make(chan error, 1)
	go func() { admissionA <- rt.AwaitCharacterAdmission(context.Background(), identity) }()
	go func() { admissionB <- rt.AwaitCharacterAdmission(context.Background(), identity) }()
	waitForCommandDepthAtLeast(t, rt, 2)
	if report := rt.Step(1, 50*time.Millisecond); len(report.CommandErrors) != 0 {
		t.Fatalf("admission errors=%#v", report.CommandErrors)
	}
	if err := <-admissionA; err != nil {
		t.Fatalf("admission A=%v", err)
	}
	if err := <-admissionB; err != nil {
		t.Fatalf("admission B=%v", err)
	}

	requestA := identityJoinRequest(t, 1, 1, identity)
	requestB := identityJoinRequest(t, 2, 2, identity)
	resultA := make(chan error, 1)
	resultB := make(chan error, 1)
	go func() { resultA <- rt.AwaitJoin(context.Background(), requestA) }()
	go func() { resultB <- rt.AwaitJoin(context.Background(), requestB) }()
	waitForCommandDepthAtLeast(t, rt, 2)
	report := rt.Step(2, 50*time.Millisecond)
	errA := <-resultA
	errB := <-resultB
	if (errA == nil) == (errB == nil) {
		t.Fatalf("join results A=%v B=%v", errA, errB)
	}
	if errA != nil && !errors.Is(errA, ErrCharacterIdentityActive) {
		t.Fatalf("join A=%v", errA)
	}
	if errB != nil && !errors.Is(errB, ErrCharacterIdentityActive) {
		t.Fatalf("join B=%v", errB)
	}
	if len(report.CommandErrors) != 1 || !errors.Is(report.CommandErrors[0].Err, ErrCharacterIdentityActive) {
		t.Fatalf("report errors=%#v", report.CommandErrors)
	}
	if len(rt.sessions.List()) != 1 {
		t.Fatalf("active sessions=%d", len(rt.sessions.List()))
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
