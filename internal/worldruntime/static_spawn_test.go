package worldruntime

import (
	"errors"
	"fmt"
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

func TestQueuedMonsterSpawnUsesAOIAndSkillCombatPaths(t *testing.T) {
	nav := navigation.Plane{MinX: -100, MaxX: 100, MinZ: -100, MaxZ: 100, Layer: 0}
	sim := simulation.New(spatial.NewGrid(16), movement.NewService(nav, 0.1))
	if err := sim.Spawn(world.EntityState{
		ID: 1, Kind: world.EntityPlayer,
		Transform: world.Transform{Position: world.Position{X: 0, Z: 0, Layer: 0}},
	}, 6, 0.35, 0.5); err != nil {
		t.Fatal(err)
	}
	actions, err := combat.NewService([]combat.ActionDefinition{{
		ID: "shatter-strike", Targets: []combat.TargetKind{combat.TargetEntity}, Range: 4.5,
		BaseDamage: 150, DamageType: combat.DamagePhysical, CooldownSeconds: 2.5,
	}})
	if err != nil {
		t.Fatal(err)
	}
	config := DefaultConfig()
	config.SnapshotEveryTicks = 1
	config.CharacterMaxHP = 1000
	runtime := New(sim, config, WithDynamicWorld(&characterCombatDynamic{los: true}), WithCombatService(actions))

	const monsterID world.EntityID = 9001
	if err := runtime.EnqueueSpawnEntity(SpawnEntityRequest{
		Entity: world.EntityState{
			ID:          monsterID,
			Kind:        world.EntityMonster,
			ArchetypeID: "wolf-gray-01",
			Transform:   world.Transform{Position: world.Position{X: 2, Z: 0, Layer: 0}},
		},
		Speed: 4, Radius: 0.35, MaxStepHeight: 0.5, HP: 200, MaxHP: 200,
	}); err != nil {
		t.Fatal(err)
	}
	connection := session.NewQueueConnection(128, 128)
	playerSession, err := session.New(1, 1, 64, connection)
	if err != nil {
		t.Fatal(err)
	}
	if err := runtime.EnqueueRegister(playerSession); err != nil {
		t.Fatal(err)
	}
	initial := runtime.Step(1, 50*time.Millisecond)
	if len(initial.CommandErrors) != 0 {
		t.Fatalf("initial command errors=%#v", initial.CommandErrors)
	}

	monster, ok := sim.Entity(monsterID)
	if !ok || monster.Kind != world.EntityMonster || monster.ArchetypeID != "wolf-gray-01" || monster.Transform.Position != (world.Position{X: 2, Z: 0, Layer: 0}) {
		t.Fatalf("spawned monster=%+v ok=%v", monster, ok)
	}
	state, ok := runtime.characters.State(monsterID)
	if !ok || state.HP != 200 || state.MaxHP != 200 || state.Defeated {
		t.Fatalf("spawned monster vitals=%+v ok=%v", state, ok)
	}

	var sawSpawn, sawInitialVitals bool
	for _, message := range drainStaticSpawnReliable(connection) {
		switch value := message.(type) {
		case protocol.EntitySpawn:
			if value.EntityID == monsterID {
				sawSpawn = true
				if value.ArchetypeID != "wolf-gray-01" {
					t.Fatalf("monster spawn archetype=%q", value.ArchetypeID)
				}
			}
		case protocol.EntityVitalsState:
			if value.EntityID == monsterID {
				sawInitialVitals = true
				if value.HP != 200 || value.MaxHP != 200 || value.Defeated {
					t.Fatalf("initial monster vitals=%+v", value)
				}
			}
		}
	}
	if !sawSpawn || !sawInitialVitals {
		t.Fatalf("monster did not enter AOI spawn/vitals replication: spawn=%t vitals=%t", sawSpawn, sawInitialVitals)
	}

	if err := runtime.EnqueueUseAction(1, 1, protocol.ClientUseAction{
		ActionID: "shatter-strike", TargetKind: protocol.ActionTargetEntity, TargetID: fmt.Sprint(monsterID),
	}); err != nil {
		t.Fatal(err)
	}
	first := runtime.Step(2, 50*time.Millisecond)
	if len(first.CommandErrors) != 0 || len(first.ActionRejections) != 0 {
		t.Fatalf("first attack report=%#v", first)
	}
	state, _ = runtime.characters.State(monsterID)
	if state.HP != 50 || state.Defeated {
		t.Fatalf("after first attack vitals=%+v", state)
	}
	assertStaticSpawnCombatMessages(t, drainStaticSpawnReliable(connection), monsterID, "shatter-strike", 150, 50, false, true, false)

	if err := runtime.EnqueueUseAction(1, 2, protocol.ClientUseAction{
		ActionID: "shatter-strike", TargetKind: protocol.ActionTargetEntity, TargetID: fmt.Sprint(monsterID),
	}); err != nil {
		t.Fatal(err)
	}
	cooldown := runtime.Step(3, 50*time.Millisecond)
	if len(cooldown.ActionRejections) != 1 || !errors.Is(cooldown.ActionRejections[0].Err, combat.ErrActionCooldown) {
		t.Fatalf("cooldown report=%#v", cooldown.ActionRejections)
	}
	state, _ = runtime.characters.State(monsterID)
	if state.HP != 50 || state.Defeated {
		t.Fatalf("cooldown changed vitals=%+v", state)
	}
	assertStaticSpawnCombatMessages(t, drainStaticSpawnReliable(connection), monsterID, "shatter-strike", 150, 50, false, false, true)

	if err := runtime.EnqueueUseAction(1, 3, protocol.ClientUseAction{
		ActionID: "shatter-strike", TargetKind: protocol.ActionTargetEntity, TargetID: fmt.Sprint(monsterID),
	}); err != nil {
		t.Fatal(err)
	}
	defeat := runtime.Step(52, 50*time.Millisecond)
	if len(defeat.CommandErrors) != 0 || len(defeat.ActionRejections) != 0 {
		t.Fatalf("defeat attack report=%#v", defeat)
	}
	state, _ = runtime.characters.State(monsterID)
	if state.HP != 0 || !state.Defeated {
		t.Fatalf("defeated monster vitals=%+v", state)
	}
	assertStaticSpawnCombatMessages(t, drainStaticSpawnReliable(connection), monsterID, "shatter-strike", 150, 0, true, true, false)
}

func drainStaticSpawnReliable(connection *session.QueueConnection) []protocol.Message {
	var messages []protocol.Message
	for {
		select {
		case envelope := <-connection.Reliable():
			messages = append(messages, envelope.Message)
		default:
			return messages
		}
	}
}

func assertStaticSpawnCombatMessages(t *testing.T, messages []protocol.Message, monsterID world.EntityID, actionID string, damage, hp uint32, defeated, wantStarted, wantRejected bool) {
	t.Helper()
	var sawStarted, sawCombat, sawVitals, sawRejected bool
	for _, message := range messages {
		switch value := message.(type) {
		case protocol.ActionStarted:
			if value.ActorEntityID == 1 {
				sawStarted = true
				if value.ActionID != actionID {
					t.Fatalf("ActionStarted action=%q want=%q", value.ActionID, actionID)
				}
			}
		case protocol.ActionRejected:
			if value.ActorEntityID == 1 {
				sawRejected = true
				if value.ActionID != actionID || value.Reason != protocol.ActionRejectionCooldown {
					t.Fatalf("ActionRejected=%+v want action=%q reason=%q", value, actionID, protocol.ActionRejectionCooldown)
				}
			}
		case protocol.CombatEvent:
			if value.TargetEntityID == monsterID && value.ActionID == actionID && value.Result == protocol.CombatEventHit && value.Damage == damage {
				sawCombat = true
			}
		case protocol.EntityVitalsState:
			if value.EntityID == monsterID && value.HP == hp && value.MaxHP == 200 && value.Defeated == defeated {
				sawVitals = true
			}
		}
	}
	if sawStarted != wantStarted || sawRejected != wantRejected || (wantStarted && !sawCombat) || (wantStarted && !sawVitals) || (!wantStarted && !wantRejected && sawCombat) {
		t.Fatalf("combat messages=%#v got started=%t combat=%t vitals=%t rejected=%t want started=%t rejected=%t", messages, sawStarted, sawCombat, sawVitals, sawRejected, wantStarted, wantRejected)
	}
}
