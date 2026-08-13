package combat

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func testAction() ActionDefinition {
	return ActionDefinition{
		ID: "basic-attack",
		Targets: []TargetKind{TargetGate},
		Range: 4.5,
		BaseDamage: 100,
		DamageType: DamagePhysical,
		CooldownSeconds: 0.5,
	}
}

func TestLoadRejectsUnknownFields(t *testing.T) {
	_, err := Load(strings.NewReader(`{
		"schema_version":1,
		"revision":"r1",
		"actions":[{
			"id":"basic-attack",
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

func TestPrepareBuildsServerOwnedDamageSource(t *testing.T) {
	svc, err := NewService([]ActionDefinition{testAction()})
	if err != nil {
		t.Fatal(err)
	}

	prepared, err := svc.Prepare(42, "basic-attack", Target{Kind: TargetGate, ID: "main-gate"}, 10)
	if err != nil {
		t.Fatal(err)
	}
	if prepared.Definition.Range != 4.5 || prepared.Damage.Amount != 100 || prepared.Damage.Type != DamagePhysical {
		t.Fatalf("prepared=%+v", prepared)
	}
	if prepared.Damage.Source.ActorEntityID != 42 || prepared.Damage.Source.ActionID != "basic-attack" {
		t.Fatalf("source=%+v", prepared.Damage.Source)
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
	// Target domain 若拒絕，呼叫端不 Commit；同 tick 再 Prepare 仍應合法。
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
