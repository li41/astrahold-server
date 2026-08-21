package worldruntime

import (
	"testing"
	"time"

	"github.com/li41/astrahold-server/internal/combat"
	"github.com/li41/astrahold-server/internal/gameplayworld"
	"github.com/li41/astrahold-server/internal/movement"
	"github.com/li41/astrahold-server/internal/navigation"
	"github.com/li41/astrahold-server/internal/protocol"
	"github.com/li41/astrahold-server/internal/session"
	"github.com/li41/astrahold-server/internal/simulation"
	"github.com/li41/astrahold-server/internal/spatial"
	"github.com/li41/astrahold-server/internal/world"
)

func TestPlayerAndMonsterShareGenericCombatantActionPath(t *testing.T) {
	definition := gameplayworld.Definition{
		SchemaVersion: gameplayworld.SchemaVersion,
		WorldID: "combatant-test",
		Revision: "r1",
		Units: "meters",
		Agent: gameplayworld.AgentDefaults{Radius: .35, Height: 1.8, MaxStepHeight: .5},
		Surfaces: []gameplayworld.Surface{{ID: "ground", Layer: 0, Bounds: gameplayworld.BoundsXZ{MinX: -10, MaxX: 10, MinZ: -10, MaxZ: 10}}},
	}
	nav, err := navigation.NewGameplayNavigator(definition)
	if err != nil { t.Fatal(err) }
	sim := simulation.New(spatial.NewGrid(16), movement.NewService(nav, .1))
	if err := sim.Spawn(world.EntityState{ID: 1, Kind: world.EntityPlayer, Transform: world.Transform{Position: world.Position{X: 0, Z: 0, Layer: 0}}}, 6, .35, .5); err != nil { t.Fatal(err) }
	if err := sim.Spawn(world.EntityState{ID: 2, Kind: world.EntityMonster, ArchetypeID: "wolf-gray-01", Transform: world.Transform{Position: world.Position{X: 2, Z: 0, Layer: 0}}}, 4, .35, .5); err != nil { t.Fatal(err) }

	catalog, err := combat.NewService([]combat.ActionDefinition{{ID: "basic-attack", Targets: []combat.TargetKind{combat.TargetEntity}, Range: 4.5, BaseDamage: 100, DamageType: combat.DamagePhysical, CooldownSeconds: .5}})
	if err != nil { t.Fatal(err) }
	cfg := DefaultConfig()
	cfg.SnapshotEveryTicks = 100
	rt := New(sim, cfg, WithDynamicWorld(nav), WithCombatService(catalog))
	if err := rt.characters.Register(2); err != nil { t.Fatal(err) }

	conn := session.NewQueueConnection(32, 16)
	s, err := session.New(1, 1, 20, conn)
	if err != nil { t.Fatal(err) }
	if err := rt.EnqueueRegister(s); err != nil { t.Fatal(err) }
	report := rt.Step(1, 50*time.Millisecond)
	if len(report.CommandErrors) != 0 { t.Fatalf("register report=%#v", report) }

	if err := rt.EnqueueUseAction(1, 1, protocol.ClientUseAction{ActionID: "basic-attack", TargetKind: protocol.ActionTargetEntity, TargetID: "2"}); err != nil { t.Fatal(err) }
	report = rt.Step(2, 50*time.Millisecond)
	if len(report.CommandErrors) != 0 || len(report.ActionRejections) != 0 { t.Fatalf("player->monster report=%#v", report) }
	monster, ok := rt.combatantState(2)
	if !ok || monster.HP != 900 { t.Fatalf("monster state=%+v ok=%v, want HP 900", monster, ok) }
	first := waitForCombatEvent(t, conn)
	if first.ActorEntityID != 1 || first.TargetEntityID != 2 || first.Result != protocol.CombatEventHit || first.Damage != 100 {
		t.Fatalf("player event=%+v", first)
	}

	aiReport := StepReport{Tick: 3}
	rt.prepareAndDispatchAction("monster_ai_action", 0, combat.Intent{
		ActorEntityID: 2,
		ActionID: "basic-attack",
		Target: combat.Target{Kind: combat.TargetEntity, ID: "1"},
	}, 3, 50*time.Millisecond, &aiReport)
	if len(aiReport.CommandErrors) != 0 || len(aiReport.ActionRejections) != 0 { t.Fatalf("monster->player report=%#v", aiReport) }
	player, ok := rt.combatantState(1)
	if !ok || player.HP != 900 { t.Fatalf("player state=%+v ok=%v, want HP 900", player, ok) }
	second := waitForCombatEvent(t, conn)
	if second.ActorEntityID != 2 || second.TargetEntityID != 1 || second.Result != protocol.CombatEventHit || second.Damage != 100 || second.ActionInstanceID <= first.ActionInstanceID {
		t.Fatalf("monster event=%+v first=%+v", second, first)
	}
}

func waitForCombatEvent(t *testing.T, conn *session.QueueConnection) protocol.CombatEvent {
	t.Helper()
	timer := time.NewTimer(time.Second)
	defer timer.Stop()
	for {
		select {
		case envelope := <-conn.Reliable():
			if event, ok := envelope.Message.(protocol.CombatEvent); ok {
				return event
			}
		case <-timer.C:
			t.Fatal("timed out waiting for CombatEvent")
		}
	}
}
