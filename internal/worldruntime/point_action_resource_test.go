package worldruntime

import (
	"errors"
	"testing"
	"time"

	"github.com/li41/astrahold-server/internal/character"
	"github.com/li41/astrahold-server/internal/combat"
	"github.com/li41/astrahold-server/internal/movement"
	"github.com/li41/astrahold-server/internal/navigation"
	"github.com/li41/astrahold-server/internal/protocol"
	"github.com/li41/astrahold-server/internal/session"
	"github.com/li41/astrahold-server/internal/simulation"
	"github.com/li41/astrahold-server/internal/spatial"
	"github.com/li41/astrahold-server/internal/world"
)

func TestFireballConsumesMPAndInsufficientResourceHasNoGameplaySideEffects(t *testing.T) {
	nav := navigation.Plane{MinX: -100, MaxX: 100, MinZ: -100, MaxZ: 100, Layer: 0}
	sim := simulation.New(spatial.NewGrid(16), movement.NewService(nav, 0.1))
	for _, entity := range []world.EntityState{
		{ID: 1, Kind: world.EntityPlayer, Transform: world.Transform{Position: world.Position{X: 0, Z: 0, Layer: 0}}},
		{ID: 2, Kind: world.EntityPlayer, Transform: world.Transform{Position: world.Position{X: 0, Z: 5, Layer: 0}}},
	} {
		if err := sim.Spawn(entity, 6, 0.35, 0.5); err != nil {
			t.Fatal(err)
		}
	}

	combatService, err := combat.NewService([]combat.ActionDefinition{{
		ID:              "fireball",
		Targets:         []combat.TargetKind{combat.TargetPoint},
		Range:           12,
		HitRadius:       0.9,
		PointResolution: combat.PointResolutionLineFirst,
		BaseDamage:      150,
		DamageType:      combat.DamagePhysical,
		MPCost:          60,
		CooldownSeconds: 2,
	}})
	if err != nil {
		t.Fatal(err)
	}

	cfg := DefaultConfig()
	cfg.SnapshotEveryTicks = 100
	cfg.CharacterMaxHP = 200
	rt := New(sim, cfg, WithDynamicWorld(&characterCombatDynamic{los: true}), WithCombatService(combatService))

	source := session.NewQueueConnection(64, 64)
	target := session.NewQueueConnection(64, 64)
	for id, conn := range map[uint64]*session.QueueConnection{1: source, 2: target} {
		s, err := session.New(session.ID(id), world.EntityID(id), 20, conn)
		if err != nil {
			t.Fatal(err)
		}
		if err := rt.EnqueueRegister(s); err != nil {
			t.Fatal(err)
		}
	}
	initial := rt.Step(1, 50*time.Millisecond)
	if len(initial.CommandErrors) != 0 {
		t.Fatalf("initial errors=%#v", initial.CommandErrors)
	}
	drainConnection(source)
	drainConnection(target)

	x, z := float32(0), float32(10)
	intent := protocol.ClientUseAction{
		ActionID:   "fireball",
		TargetKind: protocol.ActionTargetPoint,
		TargetX:    &x,
		TargetZ:    &z,
	}

	if err := rt.EnqueueUseAction(1, 1, intent); err != nil {
		t.Fatal(err)
	}
	accepted := rt.Step(2, 50*time.Millisecond)
	if len(accepted.CommandErrors) != 0 || len(accepted.ActionRejections) != 0 {
		t.Fatalf("accepted=%#v", accepted)
	}

	actorState, ok := rt.characters.State(1)
	if !ok || actorState.MP != 40 || actorState.MaxMP != character.DefaultMaxMP {
		t.Fatalf("actor after accepted fireball=%#v ok=%v", actorState, ok)
	}
	targetState, ok := rt.characters.State(2)
	if !ok || targetState.HP != 50 || targetState.Defeated {
		t.Fatalf("target after accepted fireball=%#v ok=%v", targetState, ok)
	}
	assertActionStarted(t, source, "fireball")
	drainConnection(source)
	drainConnection(target)

	// The first cast commits a 2 second cooldown at 50 ms/tick, so tick 42 is the first legal
	// retry. The actor has only 40 MP left and must reject before damage or cooldown commit.
	if err := rt.EnqueueUseAction(1, 2, intent); err != nil {
		t.Fatal(err)
	}
	insufficient := rt.Step(42, 50*time.Millisecond)
	if len(insufficient.ActionRejections) != 1 || !errors.Is(insufficient.ActionRejections[0].Err, character.ErrInsufficientResource) {
		t.Fatalf("insufficient=%#v", insufficient.ActionRejections)
	}
	assertFireballResourceRejected(t, source, 2)

	actorState, _ = rt.characters.State(1)
	if actorState.MP != 40 {
		t.Fatalf("rejected fireball mutated actor MP: %#v", actorState)
	}
	targetState, _ = rt.characters.State(2)
	if targetState.HP != 50 || targetState.Defeated {
		t.Fatalf("rejected fireball mutated target: %#v", targetState)
	}

	// A resource rejection must not consume cooldown. The immediate next tick therefore rejects
	// for the same insufficient-resource reason rather than action cooldown.
	drainConnection(source)
	drainConnection(target)
	if err := rt.EnqueueUseAction(1, 3, intent); err != nil {
		t.Fatal(err)
	}
	retry := rt.Step(43, 50*time.Millisecond)
	if len(retry.ActionRejections) != 1 || !errors.Is(retry.ActionRejections[0].Err, character.ErrInsufficientResource) {
		t.Fatalf("retry after resource rejection=%#v", retry.ActionRejections)
	}
	assertFireballResourceRejected(t, source, 3)
}

func assertFireballResourceRejected(t *testing.T, conn *session.QueueConnection, sequence uint32) {
	t.Helper()
	for {
		select {
		case env := <-conn.Reliable():
			switch msg := env.Message.(type) {
			case protocol.ActionRejected:
				if msg.ClientActionSequence != sequence || msg.ActorEntityID != 1 || msg.ActionID != "fireball" || msg.TargetKind != protocol.ActionTargetPoint || msg.Reason != protocol.ActionRejectionInsufficientResource || msg.CooldownReadyTick != 0 {
					t.Fatalf("ActionRejected=%#v want sequence=%d insufficient_resource ready=0", msg, sequence)
				}
				return
			case protocol.ActionStarted:
				t.Fatalf("resource-rejected fireball emitted ActionStarted: %#v", msg)
			}
		default:
			t.Fatalf("missing fireball ActionRejected sequence=%d", sequence)
		}
	}
}
