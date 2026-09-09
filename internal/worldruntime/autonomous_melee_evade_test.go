package worldruntime

import (
	"testing"
	"time"

	"github.com/li41/astrahold-server/internal/combat"
	"github.com/li41/astrahold-server/internal/movement"
	"github.com/li41/astrahold-server/internal/navigation"
	"github.com/li41/astrahold-server/internal/protocol"
	"github.com/li41/astrahold-server/internal/session"
	"github.com/li41/astrahold-server/internal/simulation"
	"github.com/li41/astrahold-server/internal/spatial"
	"github.com/li41/astrahold-server/internal/world"
)

func TestAutonomousMeleeAgentEvadesBeforeRetargetingAndRestoresVitalsAtHome(t *testing.T) {
	nav := navigation.Plane{MinX: -100, MaxX: 100, MinZ: -100, MaxZ: 100, Layer: 0}
	sim := simulation.New(spatial.NewGrid(16), movement.NewService(nav, 0.1))
	for _, player := range []world.EntityState{
		{ID: 1, Kind: world.EntityPlayer, Transform: world.Transform{Position: world.Position{X: 4, Layer: 0}}},
		{ID: 2, Kind: world.EntityPlayer, Transform: world.Transform{Position: world.Position{X: 30, Layer: 0}}},
	} {
		if err := sim.Spawn(player, 0, 0.35, 0.5); err != nil {
			t.Fatal(err)
		}
	}

	combatService, err := combat.NewService([]combat.ActionDefinition{{
		ID: "wolf-bite", Targets: []combat.TargetKind{combat.TargetEntity}, Range: 2,
		BaseDamage: 20, DamageType: combat.DamagePhysical, CooldownSeconds: 1,
	}})
	if err != nil {
		t.Fatal(err)
	}
	cfg := DefaultConfig()
	cfg.SnapshotEveryTicks = 1
	rt := New(
		sim,
		cfg,
		WithDynamicWorld(&characterCombatDynamic{los: true}),
		WithCombatService(combatService),
		WithAutonomousMeleeAgent(AutonomousMeleeAgentConfig{
			EntityID: 9001, Home: world.Position{X: 0, Layer: 0}, ActionID: "wolf-bite",
			AggroRange: 6, LeashRange: 8, AttackRange: 1.5, ReturnTolerance: 0.05,
		}),
	)

	conn1 := session.NewQueueConnection(128, 128)
	conn2 := session.NewQueueConnection(128, 128)
	for _, spec := range []struct {
		id       session.ID
		entityID world.EntityID
		conn     *session.QueueConnection
	}{{1, 1, conn1}, {2, 2, conn2}} {
		s, err := session.New(spec.id, spec.entityID, 40, spec.conn)
		if err != nil {
			t.Fatal(err)
		}
		if err := rt.EnqueueRegister(s); err != nil {
			t.Fatal(err)
		}
	}
	if err := rt.EnqueueSpawnEntity(SpawnEntityRequest{
		Entity: world.EntityState{
			ID: 9001, Kind: world.EntityMonster, ArchetypeID: "wolf-gray-01",
			Transform: world.Transform{Position: world.Position{X: 0, Layer: 0}},
		},
		Speed: 4, Radius: 0.35, MaxStepHeight: 0.5, HP: 200, MaxHP: 200,
	}); err != nil {
		t.Fatal(err)
	}

	initial := rt.Step(1, 50*time.Millisecond)
	if len(initial.CommandErrors) != 0 || len(initial.ActionRejections) != 0 {
		t.Fatalf("initial=%#v", initial)
	}
	if got := rt.autonomousMeleeAgents[0].targetID; got != 1 {
		t.Fatalf("initial target=%d; want player 1", got)
	}
	drainConnection(conn1)
	drainConnection(conn2)

	if _, err := rt.characters.ReduceHP(9001, 120); err != nil {
		t.Fatal(err)
	}
	if _, err := rt.characters.SpendMP(9001, 40); err != nil {
		t.Fatal(err)
	}
	rt.markEntityVitalsDirty(9001)
	damaged := rt.Step(2, 50*time.Millisecond)
	if len(damaged.CommandErrors) != 0 || len(damaged.ActionRejections) != 0 {
		t.Fatalf("damaged=%#v", damaged)
	}
	state, ok := rt.characters.State(9001)
	if !ok || state.HP != 80 || state.MP != 60 {
		t.Fatalf("damaged state=%#v ok=%v; want hp=80 mp=60", state, ok)
	}
	drainConnection(conn1)
	drainConnection(conn2)

	if err := rt.EnqueueTeleport(1, world.Position{X: 20, Layer: 0}); err != nil {
		t.Fatal(err)
	}
	if err := rt.EnqueueTeleport(2, world.Position{X: 2, Layer: 0}); err != nil {
		t.Fatal(err)
	}

	completedTick := uint64(0)
	for tick := uint64(3); tick <= 20; tick++ {
		report := rt.Step(tick, 50*time.Millisecond)
		if len(report.CommandErrors) != 0 || len(report.ActionRejections) != 0 {
			t.Fatalf("tick=%d report=%#v", tick, report)
		}
		agent := rt.autonomousMeleeAgents[0]
		if agent.targetID != 0 {
			t.Fatalf("tick=%d agent=%#v; must ignore nearby player until evade completes", tick, agent)
		}
		state, ok := rt.characters.State(9001)
		if !ok {
			t.Fatalf("tick=%d missing wolf state", tick)
		}
		if agent.returningHome {
			if state.HP != 80 || state.MP != 60 {
				t.Fatalf("tick=%d state=%#v; restore must wait until home", tick, state)
			}
			continue
		}
		if state.HP != 200 || state.MP != state.MaxMP || state.Defeated {
			t.Fatalf("tick=%d restored state=%#v; want full alive vitals", tick, state)
		}
		completedTick = tick
		break
	}
	if completedTick == 0 {
		t.Fatal("wolf did not complete evade and restore at home")
	}

	sawRestoredVitals := false
	for {
		select {
		case env := <-conn2.Reliable():
			message, ok := env.Message.(protocol.EntityVitalsState)
			if ok && message.EntityID == 9001 && message.HP == 200 && message.MaxHP == 200 && message.MP == message.MaxMP && !message.Defeated {
				sawRestoredVitals = true
			}
		default:
			if !sawRestoredVitals {
				t.Fatal("nearby player did not receive authoritative restored monster vitals")
			}
			goto retarget
		}
	}

retarget:
	report := rt.Step(completedTick+1, 50*time.Millisecond)
	if len(report.CommandErrors) != 0 || len(report.ActionRejections) != 0 {
		t.Fatalf("retarget=%#v", report)
	}
	if got := rt.autonomousMeleeAgents[0].targetID; got != 2 {
		t.Fatalf("retarget=%d; want nearby player 2 only after evade completed", got)
	}
}
