package combat

import (
	"errors"
	"testing"
	"time"
)

func TestSelfMitigationDefinitionCommitExpiryAndClear(t *testing.T) {
	action := ActionDefinition{
		ID:                         "fortify",
		Effect:                     EffectSelfMitigation,
		Targets:                    []TargetKind{TargetEntity},
		Range:                      0.1,
		SelfDamageReductionPercent: 45,
		DurationSeconds:            3,
		CooldownSeconds:            20,
	}
	svc, err := NewService([]ActionDefinition{action})
	if err != nil {
		t.Fatal(err)
	}
	prepared, err := svc.Prepare(10, action.ID, Target{Kind: TargetEntity, ID: "10"}, 2)
	if err != nil {
		t.Fatal(err)
	}
	svc.Commit(prepared, 2, 50*time.Millisecond)
	if got := svc.SelfDamageReductionPercent(10, 2); got != 45 {
		t.Fatalf("active reduction=%d want=45", got)
	}
	if got := svc.SelfDamageReductionPercent(10, 61); got != 45 {
		t.Fatalf("reduction before expiry=%d want=45", got)
	}
	if got := svc.SelfDamageReductionPercent(10, 62); got != 0 {
		t.Fatalf("reduction at expiry=%d want=0", got)
	}

	prepared, err = svc.Prepare(11, action.ID, Target{Kind: TargetEntity, ID: "11"}, 100)
	if err != nil {
		t.Fatal(err)
	}
	svc.Commit(prepared, 100, 50*time.Millisecond)
	svc.ClearSelfMitigation(11)
	if got := svc.SelfDamageReductionPercent(11, 100); got != 0 {
		t.Fatalf("reduction after clear=%d want=0", got)
	}
}

func TestSelfMitigationDefinitionRejectsInvalidShapes(t *testing.T) {
	base := ActionDefinition{
		ID:                         "fortify",
		Effect:                     EffectSelfMitigation,
		Targets:                    []TargetKind{TargetEntity},
		Range:                      0.1,
		SelfDamageReductionPercent: 45,
		DurationSeconds:            3,
		CooldownSeconds:            20,
	}
	cases := []ActionDefinition{
		func() ActionDefinition { a := base; a.SelfDamageReductionPercent = 0; return a }(),
		func() ActionDefinition { a := base; a.SelfDamageReductionPercent = 100; return a }(),
		func() ActionDefinition { a := base; a.DurationSeconds = 0; return a }(),
		func() ActionDefinition { a := base; a.BaseDamage = 1; return a }(),
		func() ActionDefinition { a := base; a.Targets = []TargetKind{TargetPoint}; a.HitRadius = 1; return a }(),
	}
	for i, action := range cases {
		if _, err := NewService([]ActionDefinition{action}); !errors.Is(err, ErrInvalidDefinition) {
			t.Fatalf("case %d err=%v want ErrInvalidDefinition", i, err)
		}
	}
}
