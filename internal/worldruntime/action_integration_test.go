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

func TestGenericActionUsesCombatCatalogInsteadOfLegacyGateProfile(t *testing.T) {
	definition := gameplayworld.Definition{
		SchemaVersion: gameplayworld.SchemaVersion, WorldID: "action-test", Revision: "r1", Units: "meters",
		Agent: gameplayworld.AgentDefaults{Radius: .35, Height: 1.8, MaxStepHeight: .5},
		Surfaces: []gameplayworld.Surface{{ID: "ground", Layer: 0, Bounds: gameplayworld.BoundsXZ{MinX: -10, MaxX: 10, MinZ: -10, MaxZ: 10}}},
		Blockers: []gameplayworld.Blocker{{ID: "main-gate", Layer: 0, Bounds: gameplayworld.BoundsXZ{MinX: -1, MaxX: 1, MinZ: 1, MaxZ: 2}, MinY: 0, MaxY: 3, BlocksMovement: true, BlocksLOS: true, Enabled: true}},
		Gates: []gameplayworld.Gate{{ID: "main-gate", BlockerID: "main-gate", MaxHP: 500, Attack: gameplayworld.GateAttackProfile{Range: .1, Damage: 499, CooldownSeconds: 99}}},
	}
	nav, err := navigation.NewGameplayNavigator(definition); if err != nil { t.Fatal(err) }
	sim := simulation.New(spatial.NewGrid(16), movement.NewService(nav, .1))
	if err := sim.Spawn(world.EntityState{ID: 1, Kind: world.EntityPlayer, Transform: world.Transform{Position: world.Position{Layer: 0}}}, 6, .35, .5); err != nil { t.Fatal(err) }
	catalog, err := combat.NewService([]combat.ActionDefinition{{ID: "basic-attack", Targets: []combat.TargetKind{combat.TargetGate}, Range: 4.5, BaseDamage: 100, DamageType: combat.DamagePhysical, CooldownSeconds: .5}}); if err != nil { t.Fatal(err) }
	cfg := DefaultConfig(); cfg.SnapshotEveryTicks = 100
	rt := New(sim, cfg, WithDynamicWorld(nav), WithSiegeGates(definition.Gates), WithCombatService(catalog))
	conn := session.NewQueueConnection(16,16); s, _ := session.New(1,1,20,conn); _ = rt.EnqueueRegister(s); _ = rt.Step(1,50*time.Millisecond); _ = nextDynamicState(t,conn)
	if err := rt.EnqueueUseAction(1,1,protocol.ClientUseAction{ActionID:"basic-attack",TargetKind:protocol.ActionTargetGate,TargetID:"main-gate"}); err != nil { t.Fatal(err) }
	report := rt.Step(2,50*time.Millisecond)
	if len(report.CommandErrors)!=0 || len(report.ActionRejections)!=0 { t.Fatalf("report=%#v",report) }
	started := nextActionStarted(t, conn)
	if started.ActorEntityID != 1 || started.ActionID != "basic-attack" || started.TargetKind != protocol.ActionTargetGate || started.TargetID != "main-gate" {
		t.Fatalf("started=%#v", started)
	}
	state := nextDynamicState(t,conn)
	if got:=state.Gates[0].HP; got!=400 { t.Fatalf("gate hp=%d, want 400 from Combat Catalog",got) }
}
