package worldruntime

import (
	"errors"
	"testing"
	"time"

	"github.com/li41/astrahold-server/internal/combat"
	"github.com/li41/astrahold-server/internal/gameplayworld"
	"github.com/li41/astrahold-server/internal/legacyclassgate"
	"github.com/li41/astrahold-server/internal/movement"
	"github.com/li41/astrahold-server/internal/navigation"
	"github.com/li41/astrahold-server/internal/protocol"
	"github.com/li41/astrahold-server/internal/session"
	"github.com/li41/astrahold-server/internal/simulation"
	"github.com/li41/astrahold-server/internal/spatial"
	"github.com/li41/astrahold-server/internal/world"
)

func TestLegacyV27ClassGateStopsAtClientIntentBoundary(t *testing.T) {
	definition := gameplayworld.Definition{
		SchemaVersion: gameplayworld.SchemaVersion,
		WorldID:       "legacy-v27-action-boundary-test",
		Revision:      "r1",
		Units:         "meters",
		Agent:         gameplayworld.AgentDefaults{Radius: .35, Height: 1.8, MaxStepHeight: .5},
		Surfaces:      []gameplayworld.Surface{{ID: "ground", Layer: 0, Bounds: gameplayworld.BoundsXZ{MinX: -10, MaxX: 10, MinZ: -10, MaxZ: 10}}},
		Blockers:      []gameplayworld.Blocker{{ID: "main-gate", Layer: 0, Bounds: gameplayworld.BoundsXZ{MinX: -1, MaxX: 1, MinZ: 1, MaxZ: 2}, MinY: 0, MaxY: 3, BlocksMovement: true, BlocksLOS: true, Enabled: true}},
		Gates:         []gameplayworld.Gate{{ID: "main-gate", BlockerID: "main-gate", MaxHP: 500, Attack: gameplayworld.GateAttackProfile{Range: 4.5, Damage: 50, CooldownSeconds: .5}}},
	}
	nav, err := navigation.NewGameplayNavigator(definition)
	if err != nil {
		t.Fatal(err)
	}
	sim := simulation.New(spatial.NewGrid(16), movement.NewService(nav, .1))
	if err := sim.Spawn(world.EntityState{ID: 1, Kind: world.EntityPlayer, Transform: world.Transform{Position: world.Position{Layer: 0}}}, 6, .35, .5); err != nil {
		t.Fatal(err)
	}
	const legacyActionID = "oathguard-sword-strike"
	catalog, err := combat.NewService([]combat.ActionDefinition{{
		ID:              legacyActionID,
		Targets:         []combat.TargetKind{combat.TargetGate},
		Range:           4.5,
		BaseDamage:      50,
		DamageType:      combat.DamagePhysical,
		CooldownSeconds: .5,
	}})
	if err != nil {
		t.Fatal(err)
	}
	cfg := DefaultConfig()
	cfg.SnapshotEveryTicks = 100
	rt := New(sim, cfg, WithDynamicWorld(nav), WithSiegeGates(definition.Gates), WithCombatService(catalog))
	conn := session.NewQueueConnection(16, 16)
	s, err := session.New(1, 1, 20, conn)
	if err != nil {
		t.Fatal(err)
	}
	if err := rt.EnqueueRegister(s); err != nil {
		t.Fatal(err)
	}
	if report := rt.Step(1, 50*time.Millisecond); len(report.CommandErrors) != 0 {
		t.Fatalf("register errors=%#v", report.CommandErrors)
	}
	_ = nextDynamicState(t, conn)

	if err := rt.EnqueueUseAction(1, 1, protocol.ClientUseAction{ActionID: legacyActionID, TargetKind: protocol.ActionTargetGate, TargetID: "main-gate"}); err != nil {
		t.Fatal(err)
	}
	clientReport := rt.Step(2, 50*time.Millisecond)
	if len(clientReport.CommandErrors) != 0 {
		t.Fatalf("client command errors=%#v", clientReport.CommandErrors)
	}
	if len(clientReport.ActionRejections) != 1 || !errors.Is(clientReport.ActionRejections[0].Err, legacyclassgate.ErrWrongClass) {
		t.Fatalf("client rejections=%#v", clientReport.ActionRejections)
	}

	serverReport := StepReport{Tick: 3}
	rt.prepareAndDispatchAction(
		"server_owned_action",
		0,
		0,
		combat.Intent{ActorEntityID: 1, ActionID: legacyActionID, Target: combat.Target{Kind: combat.TargetGate, ID: "main-gate"}},
		3,
		50*time.Millisecond,
		&serverReport,
	)
	if len(serverReport.CommandErrors) != 0 || len(serverReport.ActionRejections) != 0 {
		t.Fatalf("shared action path retained legacy class gate: %#v", serverReport)
	}
}
