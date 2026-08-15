package worldruntime

import (
	"errors"
	"testing"
	"time"

	"github.com/li41/astrahold-server/internal/characteridentity"
)

type connectionPlanResult struct {
	plan CharacterConnectionPlan
	err  error
}

func awaitConnectionPlanResult(rt *Runtime, identity characteridentity.Binding) <-chan connectionPlanResult {
	result := make(chan connectionPlanResult, 1)
	go func() {
		plan, err := rt.AwaitCharacterConnectionPlan(nil, identity)
		result <- connectionPlanResult{plan: plan, err: err}
	}()
	return result
}

func TestCharacterConnectionPlanReservesInactiveTrustedCharacter(t *testing.T) {
	rt, identity := newIdentityRuntime(t)
	result := awaitConnectionPlanResult(rt, identity)
	waitForCommandDepthAtLeast(t, rt, 1)
	if report := rt.Step(1, 50*time.Millisecond); len(report.CommandErrors) != 0 {
		t.Fatalf("plan errors=%#v", report.CommandErrors)
	}
	got := <-result
	if got.err != nil {
		t.Fatal(got.err)
	}
	if !got.plan.Valid() || got.plan.Takeover() || !got.plan.AdmissionLease.Valid() || got.plan.Ownership.Valid() {
		t.Fatalf("plan=%#v", got.plan)
	}
	if got.plan.AdmissionLease.CharacterID != identity.ID {
		t.Fatalf("lease character=%s want=%s", got.plan.AdmissionLease.CharacterID, identity.ID)
	}
	current, ok := rt.characterIdentities.admissionByCharacter[identity.ID]
	if !ok || current != got.plan.AdmissionLease {
		t.Fatalf("reservation=%#v ok=%v plan=%#v", current, ok, got.plan)
	}
}

func TestCharacterConnectionPlanReturnsActiveOwnershipWithoutAdmissionError(t *testing.T) {
	rt, ownership, active := joinOwnedIdentitySession(t)
	result := awaitConnectionPlanResult(rt, active.CharacterIdentity)
	waitForCommandDepthAtLeast(t, rt, 1)
	if report := rt.Step(2, 50*time.Millisecond); len(report.CommandErrors) != 0 {
		t.Fatalf("active plan errors=%#v", report.CommandErrors)
	}
	got := <-result
	if got.err != nil {
		t.Fatal(got.err)
	}
	if !got.plan.Valid() || !got.plan.Takeover() || got.plan.Ownership != ownership || got.plan.AdmissionLease.Valid() {
		t.Fatalf("plan=%#v ownership=%#v", got.plan, ownership)
	}
	if _, ok := rt.characterIdentities.admissionByCharacter[ownership.CharacterID]; ok {
		t.Fatal("active connection plan created an admission reservation")
	}
}

func TestCharacterConnectionPlanPreservesInactiveReservationExclusion(t *testing.T) {
	rt, identity := newIdentityRuntime(t)
	firstResult := awaitConnectionPlanResult(rt, identity)
	waitForCommandDepthAtLeast(t, rt, 1)
	if report := rt.Step(1, 50*time.Millisecond); len(report.CommandErrors) != 0 {
		t.Fatalf("first plan errors=%#v", report.CommandErrors)
	}
	first := <-firstResult
	if first.err != nil || !first.plan.AdmissionLease.Valid() {
		t.Fatalf("first=%#v", first)
	}

	secondResult := awaitConnectionPlanResult(rt, identity)
	waitForCommandDepthAtLeast(t, rt, 1)
	report := rt.Step(2, 50*time.Millisecond)
	if len(report.CommandErrors) != 1 || !errors.Is(report.CommandErrors[0].Err, ErrCharacterAdmissionReserved) {
		t.Fatalf("second plan errors=%#v", report.CommandErrors)
	}
	second := <-secondResult
	if !errors.Is(second.err, ErrCharacterAdmissionReserved) {
		t.Fatalf("second=%#v", second)
	}
	current := rt.characterIdentities.admissionByCharacter[identity.ID]
	if current != first.plan.AdmissionLease {
		t.Fatalf("second plan replaced reservation: current=%#v first=%#v", current, first.plan.AdmissionLease)
	}
}
