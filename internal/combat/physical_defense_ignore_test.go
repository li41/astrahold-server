package combat

import (
	"errors"
	"testing"
)

func TestPhysicalDefenseIgnoreIsValidatedAndCarriedByPreparedDamage(t *testing.T) {
	action := ActionDefinition{
		ID:                           "piercing-arrow",
		Effect:                       EffectDamage,
		Targets:                      []TargetKind{TargetEntity},
		Range:                        12,
		BaseDamage:                   210,
		DamageType:                   DamagePhysical,
		Blockable:                    true,
		PhysicalDefenseIgnorePercent: 25,
		CooldownSeconds:              7,
	}
	svc, err := NewService([]ActionDefinition{action})
	if err != nil {
		t.Fatal(err)
	}
	prepared, err := svc.Prepare(10, action.ID, Target{Kind: TargetEntity, ID: "20"}, 1)
	if err != nil {
		t.Fatal(err)
	}
	if prepared.Definition.PhysicalDefenseIgnorePercent != 25 || prepared.Damage.PhysicalDefenseIgnorePercent != 25 {
		t.Fatalf("prepared=%+v damage=%+v", prepared.Definition, prepared.Damage)
	}

	bad := action
	bad.PhysicalDefenseIgnorePercent = 101
	if _, err := NewService([]ActionDefinition{bad}); !errors.Is(err, ErrInvalidDefinition) {
		t.Fatalf("ignore>100 err=%v", err)
	}
	bad = action
	bad.DamageType = DamageMagic
	bad.Blockable = false
	if _, err := NewService([]ActionDefinition{bad}); !errors.Is(err, ErrInvalidDefinition) {
		t.Fatalf("magic ignore err=%v", err)
	}
	bad = action
	bad.Targets = []TargetKind{TargetPoint}
	bad.HitRadius = 1
	bad.Blockable = false
	if _, err := NewService([]ActionDefinition{bad}); !errors.Is(err, ErrInvalidDefinition) {
		t.Fatalf("point ignore err=%v", err)
	}
}
