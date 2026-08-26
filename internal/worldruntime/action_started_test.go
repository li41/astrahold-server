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

func TestActionStartedFansOutToSpawnedObserverBeforeOutcome(t *testing.T) {
	nav := navigation.Plane{MinX:-100,MaxX:100,MinZ:-100,MaxZ:100,Layer:0}
	sim := simulation.New(spatial.NewGrid(16), movement.NewService(nav, 0.1))
	for _, entity := range []world.EntityState{
		{ID:1,Kind:world.EntityPlayer,Transform:world.Transform{Position:world.Position{X:0,Layer:0}}},
		{ID:2,Kind:world.EntityPlayer,Transform:world.Transform{Position:world.Position{X:2,Layer:0}}},
		{ID:3,Kind:world.EntityPlayer,Transform:world.Transform{Position:world.Position{X:3,Layer:0}}},
	} { if err := sim.Spawn(entity, 6, 0.35, 0.5); err != nil { t.Fatal(err) } }
	combatService, err := combat.NewService([]combat.ActionDefinition{{ID:"basic-attack",Targets:[]combat.TargetKind{combat.TargetEntity},Range:4.5,BaseDamage:100,DamageType:combat.DamagePhysical,CooldownSeconds:0.5}})
	if err != nil { t.Fatal(err) }
	cfg := DefaultConfig(); cfg.SnapshotEveryTicks = 1; cfg.CharacterMaxHP = 200
	rt := New(sim, cfg, WithDynamicWorld(&characterCombatDynamic{los:true}), WithCombatService(combatService))
	connections := make([]*session.QueueConnection, 3)
	for i := 1; i <= 3; i++ {
		connections[i-1] = session.NewQueueConnection(64,64)
		s, err := session.New(session.ID(i), world.EntityID(i), 20, connections[i-1]); if err != nil { t.Fatal(err) }
		if err := rt.EnqueueRegister(s); err != nil { t.Fatal(err) }
	}
	initial := rt.Step(1, 50*time.Millisecond)
	if len(initial.CommandErrors) != 0 { t.Fatalf("initial errors=%#v", initial.CommandErrors) }
	for _, conn := range connections { drainConnection(conn) }

	if err := rt.EnqueueUseAction(1, 1, protocol.ClientUseAction{ActionID:"basic-attack",TargetKind:protocol.ActionTargetEntity,TargetID:"2"}); err != nil { t.Fatal(err) }
	report := rt.Step(2, 50*time.Millisecond)
	if len(report.CommandErrors) != 0 || len(report.ActionRejections) != 0 { t.Fatalf("report=%#v", report) }

	for index, conn := range connections {
		seenStart := false
		seenOutcome := false
		for {
			select {
			case env := <-conn.Reliable():
				switch msg := env.Message.(type) {
				case protocol.ActionStarted:
					if msg.ActorEntityID == 1 && msg.ActionID == "basic-attack" {
						seenStart = true
						if msg.TargetKind != protocol.ActionTargetEntity || msg.TargetID != "2" { t.Fatalf("session %d start=%#v", index+1, msg) }
					}
				case protocol.CombatEvent:
					if msg.ActorEntityID == 1 { seenOutcome = true }
				}
			default:
				goto done
			}
		}
	done:
		if !seenStart { t.Fatalf("session %d missing ActionStarted", index+1) }
		if index == 2 && seenOutcome { t.Fatalf("observer received participant-only CombatEvent") }
	}
}

func TestRejectedEntityActionDoesNotEmitActionStarted(t *testing.T) {
	rt, conn1, conn2 := makeCharacterCombatRuntime(t)
	rt.Step(1, 50*time.Millisecond); drainConnection(conn1); drainConnection(conn2)
	if err := rt.EnqueueUseAction(1, 1, protocol.ClientUseAction{ActionID:"basic-attack",TargetKind:protocol.ActionTargetEntity,TargetID:"1"}); err != nil { t.Fatal(err) }
	report := rt.Step(2, 50*time.Millisecond)
	if len(report.ActionRejections) != 1 { t.Fatalf("rejections=%#v", report.ActionRejections) }
	for _, conn := range []*session.QueueConnection{conn1, conn2} {
		for {
			select {
			case env := <-conn.Reliable():
				if _, ok := env.Message.(protocol.ActionStarted); ok { t.Fatalf("rejected action emitted ActionStarted") }
			default:
				goto next
			}
		}
	next:
	}
}
