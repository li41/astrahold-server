package worldruntime

import (
	"testing"
	"time"

	"github.com/li41/astrahold-server/internal/combat"
	"github.com/li41/astrahold-server/internal/gameplayworld"
	"github.com/li41/astrahold-server/internal/monstercatalog"
	"github.com/li41/astrahold-server/internal/movement"
	"github.com/li41/astrahold-server/internal/navigation"
	"github.com/li41/astrahold-server/internal/protocol"
	"github.com/li41/astrahold-server/internal/session"
	"github.com/li41/astrahold-server/internal/simulation"
	"github.com/li41/astrahold-server/internal/spatial"
	"github.com/li41/astrahold-server/internal/world"
)

func newPassiveAssistRuntime(t *testing.T, monsterIDs []world.EntityID) (*Runtime, *session.Session) {
	t.Helper()
	definition := gameplayworld.Definition{
		SchemaVersion: gameplayworld.SchemaVersion,
		WorldID:       "monster-assist-test",
		Revision:      "r1",
		Units:         "meters",
		Agent:         gameplayworld.AgentDefaults{Radius: .35, Height: 1.8, MaxStepHeight: .5},
		Surfaces: []gameplayworld.Surface{{
			ID: "ground", Layer: 0,
			Bounds: gameplayworld.BoundsXZ{MinX: -20, MaxX: 20, MinZ: -20, MaxZ: 20},
		}},
	}
	nav, err := navigation.NewGameplayNavigator(definition)
	if err != nil {
		t.Fatal(err)
	}
	sim := simulation.New(spatial.NewGrid(16), movement.NewService(nav, .1))
	combatService, err := combat.NewService([]combat.ActionDefinition{
		{ID: "jab", Targets: []combat.TargetKind{combat.TargetEntity}, Range: 8, BaseDamage: 1, DamageType: combat.DamagePhysical, CooldownSeconds: .1},
		{ID: monstercatalog.BasicMeleeActionID, Targets: []combat.TargetKind{combat.TargetEntity}, Range: 1, BaseDamage: 1, DamageType: combat.DamagePhysical, CooldownSeconds: 1},
	})
	if err != nil {
		t.Fatal(err)
	}
	options := []Option{WithDynamicWorld(nav), WithCombatService(combatService)}
	for index, id := range monsterIDs {
		x := float32(index + 1)
		options = append(options, WithAutonomousMeleeAgent(AutonomousMeleeAgentConfig{
			EntityID: id,
			Home: world.Position{X: x, Layer: 0},
			ActionID: monstercatalog.BasicMeleeActionID,
			AggroRange: 8,
			LeashRange: 12,
			AttackRange: .2,
			AggroMode: monstercatalog.AggroPassive,
			EncounterGroupID: "ant-pocket",
			AssistFamilyID: "family_redsoil_ant",
			AssistPolicy: monstercatalog.AssistSameFamily,
			AssistRadius: 10,
			MaxAssist: 2,
			ReturnTolerance: .05,
		}))
	}
	cfg := DefaultConfig()
	cfg.SnapshotEveryTicks = 1000
	rt := New(sim, cfg, options...)

	conn := session.NewQueueConnection(128, 16)
	player, err := session.New(1, 1, 32, conn)
	if err != nil {
		t.Fatal(err)
	}
	if err := rt.EnqueueJoin(JoinRequest{
		Session: player,
		Entity: world.EntityState{ID: 1, Kind: world.EntityPlayer, Transform: world.Transform{Position: world.Position{Layer: 0}}},
		Speed: 6, Radius: .35, MaxStepHeight: .5,
	}); err != nil {
		t.Fatal(err)
	}
	for index, id := range monsterIDs {
		if err := rt.EnqueueSpawnEntity(SpawnEntityRequest{
			Entity: world.EntityState{
				ID: id, Kind: world.EntityMonster, ArchetypeID: "assist-test-ant",
				Transform: world.Transform{Position: world.Position{X: float32(index + 1), Layer: 0}},
			},
			Speed: 3, Radius: .35, MaxStepHeight: .5, HP: 20, MaxHP: 20,
		}); err != nil {
			t.Fatal(err)
		}
	}
	if report := rt.Step(1, 50*time.Millisecond); len(report.CommandErrors) != 0 {
		t.Fatalf("initial errors=%#v", report.CommandErrors)
	}
	return rt, player
}

func TestPassiveMonsterDoesNotProximityAggroButRetaliatesWhenDamaged(t *testing.T) {
	rt, player := newPassiveAssistRuntime(t, []world.EntityID{9001})
	agent := rt.autonomousMeleeAgentForEntity(9001)
	if agent == nil {
		t.Fatal("agent missing")
	}
	if agent.targetID != 0 || agent.threat.Len() != 0 {
		t.Fatalf("passive monster proximity-aggroed: target=%d threat=%d", agent.targetID, agent.threat.Len())
	}

	if err := rt.EnqueueUseAction(player.ID, 1, protocol.ClientUseAction{
		ActionID: "jab", TargetKind: protocol.ActionTargetEntity, TargetID: "9001",
	}); err != nil {
		t.Fatal(err)
	}
	report := rt.Step(2, 50*time.Millisecond)
	if len(report.CommandErrors) != 0 || len(report.ActionRejections) != 0 {
		t.Fatalf("damage report=%#v", report)
	}
	if agent.targetID != player.EntityID || agent.threat.Value(player.EntityID) != 1 {
		t.Fatalf("passive retaliation target=%d threat=%d", agent.targetID, agent.threat.Value(player.EntityID))
	}
}

func TestSameFamilyAssistIsNearestCappedAndOneHop(t *testing.T) {
	rt, player := newPassiveAssistRuntime(t, []world.EntityID{9001, 9002, 9003, 9004})

	if err := rt.EnqueueUseAction(player.ID, 1, protocol.ClientUseAction{
		ActionID: "jab", TargetKind: protocol.ActionTargetEntity, TargetID: "9001",
	}); err != nil {
		t.Fatal(err)
	}
	if report := rt.Step(2, 50*time.Millisecond); len(report.CommandErrors) != 0 || len(report.ActionRejections) != 0 {
		t.Fatalf("first damage report=%#v", report)
	}

	for _, id := range []world.EntityID{9001, 9002, 9003} {
		agent := rt.autonomousMeleeAgentForEntity(id)
		if agent == nil || agent.targetID != player.EntityID {
			t.Fatalf("monster %d did not join target: %#v", id, agent)
		}
	}
	far := rt.autonomousMeleeAgentForEntity(9004)
	if far == nil || far.targetID != 0 || far.threat.Len() != 0 {
		t.Fatalf("assist cap failed for monster 9004: %#v", far)
	}

	// 9002 joined through assist and is marked as already alerted. Directly damaging it must not
	// rebroadcast and pull 9004; this is the one-hop anti-chain-aggro fence.
	if err := rt.EnqueueUseAction(player.ID, 2, protocol.ClientUseAction{
		ActionID: "jab", TargetKind: protocol.ActionTargetEntity, TargetID: "9002",
	}); err != nil {
		t.Fatal(err)
	}
	if report := rt.Step(4, 50*time.Millisecond); len(report.CommandErrors) != 0 || len(report.ActionRejections) != 0 {
		t.Fatalf("second damage report=%#v", report)
	}
	if far.targetID != 0 || far.threat.Len() != 0 {
		t.Fatalf("one-hop assist failed; monster 9004 joined after rebroadcast: %#v", far)
	}
}
