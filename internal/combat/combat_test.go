package combat

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func testAction() ActionDefinition {
	return ActionDefinition{
		ID:              "basic-attack",
		Targets:         []TargetKind{TargetGate},
		Range:           4.5,
		BaseDamage:      100,
		DamageType:      DamagePhysical,
		CooldownSeconds: 0.5,
	}
}

func testResurrectAction() ActionDefinition {
	return ActionDefinition{
		ID:              "resurrect",
		Effect:          EffectResurrect,
		Targets:         []TargetKind{TargetEntity},
		Range:           4.5,
		ReviveHPPercent: 30,
		CooldownSeconds: 10,
	}
}

func TestLoadRejectsUnknownFields(t *testing.T) {
	_, err := Load(strings.NewReader(`{
		"schema_version":3,
		"revision":"r1",
		"actions":[{
			"id":"basic-attack",
			"effect":"damage",
			"targets":["gate"],
			"range":4.5,
			"base_damage":100,
			"damage_type":"physical",
			"cooldown_seconds":0.5,
			"client_damage":999
		}]
	}`))
	if err == nil {
		t.Fatal("expected unknown field to be rejected")
	}
}

func TestLoadNormalizesDamageAndAcceptsResurrection(t *testing.T) {
	loaded, err := Load(strings.NewReader(`{
		"schema_version":3,
		"revision":"r2",
		"actions":[
			{
				"id":"basic-attack",
				"effect":"damage",
				"targets":["entity"],
				"range":4.5,
				"base_damage":100,
				"damage_type":"physical",
				"blockable":true,
				"cooldown_seconds":0.5
			},
			{
				"id":"fireball",
				"effect":"damage",
				"targets":["point"],
				"range":12,
				"hit_radius":1,
				"base_damage":150,
				"damage_type":"magic",
				"cooldown_seconds":2
			},
			{
				"id":"resurrect",
				"effect":"resurrect",
				"targets":["entity"],
				"range":4.5,
				"revive_hp_percent":30,
				"cooldown_seconds":10
			}
		]
	}`))
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Definition.Actions[0].Effect != EffectDamage || !loaded.Definition.Actions[0].Blockable || loaded.Definition.Actions[1].DamageType != DamageMagic || loaded.Definition.Actions[2].Effect != EffectResurrect {
		t.Fatalf("actions=%#v", loaded.Definition.Actions)
	}
}

func TestPrepareBuildsServerOwnedDamageSource(t *testing.T) {
	action := ActionDefinition{
		ID:              "basic-attack",
		Targets:         []TargetKind{TargetEntity},
		Range:           4.5,
		BaseDamage:      100,
		DamageType:      DamagePhysical,
		Blockable:       true,
		CooldownSeconds: 0.5,
	}
	svc, err := NewService([]ActionDefinition{action})
	if err != nil {
		t.Fatal(err)
	}

	prepared, err := svc.Prepare(42, "basic-attack", Target{Kind: TargetEntity, ID: "43"}, 10)
	if err != nil {
		t.Fatal(err)
	}
	if prepared.Definition.Effect != EffectDamage || prepared.Definition.Range != 4.5 || prepared.Damage.Amount != 100 || prepared.Damage.Type != DamagePhysical || !prepared.Damage.Blockable {
		t.Fatalf("prepared=%+v", prepared)
	}
	if prepared.Damage.Source.ActorEntityID != 42 || prepared.Damage.Source.ActionID != "basic-attack" {
		t.Fatalf("source=%+v", prepared.Damage.Source)
	}
}

func TestBlockableIsIndependentFromPhysicalDamageType(t *testing.T) {
	physicalSkill := ActionDefinition{ID: "shatter", Targets: []TargetKind{TargetEntity}, Range: 4, BaseDamage: 10, DamageType: DamagePhysical, CooldownSeconds: 1}
	svc, err := NewService([]ActionDefinition{physicalSkill})
	if err != nil {
		t.Fatal(err)
	}
	prepared, err := svc.Prepare(1, "shatter", Target{Kind: TargetEntity, ID: "2"}, 1)
	if err != nil {
		t.Fatal(err)
	}
	if prepared.Damage.Blockable {
		t.Fatal("physical damage must not become blockable implicitly")
	}

	magicBlock := physicalSkill
	magicBlock.ID = "bad-magic-block"
	magicBlock.DamageType = DamageMagic
	magicBlock.Blockable = true
	if _, err := NewService([]ActionDefinition{magicBlock}); !errors.Is(err, ErrInvalidDefinition) {
		t.Fatalf("magic blockable err=%v", err)
	}

	pointBlock := physicalSkill
	pointBlock.ID = "bad-point-block"
	pointBlock.Targets = []TargetKind{TargetPoint}
	pointBlock.HitRadius = 1
	pointBlock.Blockable = true
	if _, err := NewService([]ActionDefinition{pointBlock}); !errors.Is(err, ErrInvalidDefinition) {
		t.Fatalf("point blockable err=%v", err)
	}
}

func TestPrepareResurrectionCarriesPolicyWithoutDamage(t *testing.T) {
	svc, err := NewService([]ActionDefinition{testResurrectAction()})
	if err != nil {
		t.Fatal(err)
	}
	prepared, err := svc.Prepare(7, "resurrect", Target{Kind: TargetEntity, ID: "9"}, 20)
	if err != nil {
		t.Fatal(err)
	}
	if prepared.Definition.Effect != EffectResurrect || prepared.Definition.ReviveHPPercent != 30 {
		t.Fatalf("prepared=%+v", prepared)
	}
	if prepared.Damage.Amount != 0 || prepared.Damage.Type != "" || prepared.Damage.Source.ActorEntityID != 0 || prepared.Damage.Blockable {
		t.Fatalf("resurrection unexpectedly carries damage=%+v", prepared.Damage)
	}
}

func TestResurrectionDefinitionRejectsDamageOrGateTarget(t *testing.T) {
	badDamage := testResurrectAction()
	badDamage.BaseDamage = 1
	if _, err := NewService([]ActionDefinition{badDamage}); !errors.Is(err, ErrInvalidDefinition) {
		t.Fatalf("damage field err=%v", err)
	}

	badTarget := testResurrectAction()
	badTarget.Targets = []TargetKind{TargetGate}
	if _, err := NewService([]ActionDefinition{badTarget}); !errors.Is(err, ErrInvalidDefinition) {
		t.Fatalf("gate target err=%v", err)
	}
}

func TestCooldownStartsOnlyAfterCommit(t *testing.T) {
	svc, err := NewService([]ActionDefinition{testAction()})
	if err != nil {
		t.Fatal(err)
	}
	target := Target{Kind: TargetGate, ID: "main-gate"}

	prepared, err := svc.Prepare(1, "basic-attack", target, 10)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Prepare(1, "basic-attack", target, 10); err != nil {
		t.Fatalf("prepare before commit should not consume cooldown: %v", err)
	}

	svc.Commit(prepared, 10, 50*time.Millisecond)
	if _, err := svc.Prepare(1, "basic-attack", target, 19); !errors.Is(err, ErrActionCooldown) {
		t.Fatalf("cooldown error=%v", err)
	}
	if _, err := svc.Prepare(1, "basic-attack", target, 20); err != nil {
		t.Fatalf("action should be ready at tick 20: %v", err)
	}
}

func TestPrepareRejectsUnknownActionAndTarget(t *testing.T) {
	svc, err := NewService([]ActionDefinition{testAction()})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Prepare(1, "missing", Target{Kind: TargetGate, ID: "main-gate"}, 1); !errors.Is(err, ErrUnknownAction) {
		t.Fatalf("unknown action error=%v", err)
	}
	if _, err := svc.Prepare(1, "basic-attack", Target{Kind: TargetKind("actor"), ID: "2"}, 1); !errors.Is(err, ErrTargetNotAllowed) {
		t.Fatalf("target error=%v", err)
	}
}
