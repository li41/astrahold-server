package siege

import (
	"errors"
	"testing"
	"time"

	"github.com/li41/astrahold-server/internal/combat"
	"github.com/li41/astrahold-server/internal/world"
)

func testCombatService(t *testing.T) *combat.Service {
	t.Helper()
	svc, err := combat.NewService([]combat.ActionDefinition{{
		ID:              "basic-attack",
		Targets:         []combat.TargetKind{combat.TargetGate},
		Range:           4.5,
		BaseDamage:      100,
		DamageType:      combat.DamagePhysical,
		CooldownSeconds: 0.5,
	}})
	if err != nil {
		t.Fatal(err)
	}
	return svc
}

func TestPreparedCombatActionDamagesGateAndCommitsCooldown(t *testing.T) {
	gates, scene := testService()
	actions := testCombatService(t)
	position := world.Position{X: 0, Y: 0, Z: 6, Layer: 0}
	target := combat.Target{Kind: combat.TargetGate, ID: "main-gate"}

	prepared, err := actions.Prepare(1, "basic-attack", target, 10)
	if err != nil {
		t.Fatal(err)
	}
	first, err := gates.ApplyPreparedAction(position, prepared, scene)
	if err != nil {
		t.Fatal(err)
	}
	if first.HP != 100 || first.Destroyed || !scene.enabled {
		t.Fatalf("first state=%+v blocker=%v", first, scene.enabled)
	}
	actions.Commit(prepared, 10, 50*time.Millisecond)

	if _, err := actions.Prepare(1, "basic-attack", target, 15); !errors.Is(err, combat.ErrActionCooldown) {
		t.Fatalf("cooldown error=%v", err)
	}
	prepared, err = actions.Prepare(1, "basic-attack", target, 20)
	if err != nil {
		t.Fatal(err)
	}
	second, err := gates.ApplyPreparedAction(position, prepared, scene)
	if err != nil {
		t.Fatal(err)
	}
	if second.HP != 0 || !second.Destroyed || scene.enabled {
		t.Fatalf("destroyed state=%+v blocker=%v", second, scene.enabled)
	}
}

func TestRejectedGateTargetDoesNotConsumeCombatCooldown(t *testing.T) {
	gates, scene := testService()
	actions := testCombatService(t)
	target := combat.Target{Kind: combat.TargetGate, ID: "main-gate"}
	prepared, err := actions.Prepare(1, "basic-attack", target, 10)
	if err != nil {
		t.Fatal(err)
	}

	_, err = gates.ApplyPreparedAction(world.Position{X: 0, Z: 0, Layer: 0}, prepared, scene)
	if !errors.Is(err, ErrGateOutOfRange) {
		t.Fatalf("expected range rejection, got %v", err)
	}
	if _, err := actions.Prepare(1, "basic-attack", target, 10); err != nil {
		t.Fatalf("rejected target must not consume cooldown: %v", err)
	}
}
