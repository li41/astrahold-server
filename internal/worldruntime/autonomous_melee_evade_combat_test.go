package worldruntime

import (
	"errors"
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

func TestEvadingAutonomousMeleeMonsterRejectsDamageWithoutResourceOrCooldownCommit(t *testing.T) {
	nav := navigation.Plane{MinX: -100, MaxX: 100, MinZ: -100, MaxZ: 100, Layer: 0}
	sim := simulation.New(spatial.NewGrid(16), movement.NewService(nav, 0.1))
	if err := sim.Spawn(world.EntityState{
		ID: 1,
		Kind: world.EntityPlayer,
		Transform: world.Transform{Position: world.Position{X: 0, Layer: 0}},
	}, 6, 0.35, 0.5); err != nil {
		t.Fatal(err)
	}

	combatService, err := combat.NewService([]combat.ActionDefinition{
		{
			ID: "player-strike", Targets: []combat.TargetKind{combat.TargetEntity}, Range: 4.5,
			BaseDamage: 25, DamageType: combat.DamagePhysical, MPCost: 20, CooldownSeconds: 1,
		},
		{
			ID: "wolf-bite", Targets: []combat.TargetKind{combat.TargetEntity}, Range: 2,
			BaseDamage: 5, DamageType: combat.DamagePhysical, CooldownSeconds: 1,
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	cfg := DefaultConfig()
	cfg.SnapshotEveryTicks = 100
	cfg.CharacterMaxHP = 200
	rt := New(
		sim,
		cfg,
		WithDynamicWorld(&characterCombatDynamic{los: true}),
		WithCombatService(combatService),
		WithAutonomousMeleeAgent(AutonomousMeleeAgentConfig{
			EntityID: 9001,
			Home: world.Position{X: 10, Layer: 0},
			ActionID: "wolf-bite",
			AggroRange: 4,
			LeashRange: 12,
			AttackRange: 1,
			ReturnTolerance: 0.05,
		}),
	)

	conn := session.NewQueueConnection(64, 64)
	s, err := session.New(1, 1, 20, conn)
	if err != nil {
		t.Fatal(err)
	}
	if err := rt.EnqueueRegister(s); err != nil {
		t.Fatal(err)
	}
	if err := rt.EnqueueSpawnEntity(SpawnEntityRequest{
		Entity: world.EntityState{
			ID: 9001,
			Kind: world.EntityMonster,
			ArchetypeID: "wolf-gray-01",
			Transform: world.Transform{Position: world.Position{X: 2, Layer: 0}},
		},
		Speed: 4,
		Radius: 0.35,
		MaxStepHeight: 0.5,
		HP: 200,
		MaxHP: 200,
	}); err != nil {
		t.Fatal(err)
	}

	initial := rt.Step(1, 50*time.Millisecond)
	if len(initial.CommandErrors) != 0 || len(initial.ActionRejections) != 0 {
		t.Fatalf("initial=%#v", initial)
	}
	drainConnection(conn)
	if len(rt.autonomousMeleeAgents) != 1 {
		t.Fatalf("agents=%d; want 1", len(rt.autonomousMeleeAgents))
	}

	rt.autonomousMeleeAgents[0].targetID = 0
	rt.autonomousMeleeAgents[0].returningHome = true
	actorBefore, ok := rt.characters.State(1)
	if !ok {
		t.Fatal("missing player state")
	}
	targetBefore, ok := rt.characters.State(9001)
	if !ok {
		t.Fatal("missing monster state")
	}

	intent := protocol.ClientUseAction{
		ActionID: "player-strike",
		TargetKind: protocol.ActionTargetEntity,
		TargetID: "9001",
	}
	if err := rt.EnqueueUseAction(1, 1, intent); err != nil {
		t.Fatal(err)
	}
	rejected := rt.Step(2, 50*time.Millisecond)
	if len(rejected.CommandErrors) != 0 || len(rejected.ActionRejections) != 1 || !errors.Is(rejected.ActionRejections[0].Err, ErrEntityEvading) {
		t.Fatalf("rejected=%#v", rejected)
	}
	actorAfter, _ := rt.characters.State(1)
	targetAfter, _ := rt.characters.State(9001)
	if actorAfter.MP != actorBefore.MP {
		t.Fatalf("evade rejection changed player MP: before=%d after=%d", actorBefore.MP, actorAfter.MP)
	}
	if targetAfter.HP != targetBefore.HP || targetAfter.Defeated != targetBefore.Defeated {
		t.Fatalf("evade rejection changed monster state: before=%#v after=%#v", targetBefore, targetAfter)
	}
	if ready := rt.combat.ActionCooldownReadyTick(1, "player-strike"); ready != 0 {
		t.Fatalf("evade rejection committed cooldown readyTick=%d", ready)
	}
	assertEvadingActionRejected(t, conn, 1)
	drainConnection(conn)

	// Once the authoritative evade state is over, the same action is legal immediately. This also
	// proves the rejected attempt consumed neither cooldown nor MP.
	rt.autonomousMeleeAgents[0].returningHome = false
	rt.autonomousMeleeAgents[0].targetID = 0
	if err := rt.EnqueueUseAction(1, 2, intent); err != nil {
		t.Fatal(err)
	}
	accepted := rt.Step(3, 50*time.Millisecond)
	if len(accepted.CommandErrors) != 0 || len(accepted.ActionRejections) != 0 {
		t.Fatalf("accepted=%#v", accepted)
	}
	actorAccepted, _ := rt.characters.State(1)
	targetAccepted, _ := rt.characters.State(9001)
	if actorAccepted.MP != actorBefore.MP-20 {
		t.Fatalf("accepted attack MP=%d; want %d", actorAccepted.MP, actorBefore.MP-20)
	}
	if targetAccepted.HP != targetBefore.HP-25 || targetAccepted.Defeated {
		t.Fatalf("accepted attack target=%#v; want hp=%d alive", targetAccepted, targetBefore.HP-25)
	}
	if ready := rt.combat.ActionCooldownReadyTick(1, "player-strike"); ready <= 3 {
		t.Fatalf("accepted attack cooldown readyTick=%d; want > 3", ready)
	}
}

func assertEvadingActionRejected(t *testing.T, conn *session.QueueConnection, sequence uint32) {
	t.Helper()
	for {
		select {
		case env := <-conn.Reliable():
			switch msg := env.Message.(type) {
			case protocol.ActionRejected:
				if msg.ClientActionSequence != sequence || msg.ActorEntityID != 1 || msg.ActionID != "player-strike" || msg.TargetKind != protocol.ActionTargetEntity || msg.Reason != protocol.ActionRejectionInvalidTarget || msg.CooldownReadyTick != 0 {
					t.Fatalf("ActionRejected=%#v; want evade invalid_target ready=0", msg)
				}
				return
			case protocol.ActionStarted:
				t.Fatalf("evade-rejected attack emitted ActionStarted: %#v", msg)
			}
		default:
			t.Fatalf("missing evade ActionRejected sequence=%d", sequence)
		}
	}
}
