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

func TestPointActionHitsFirstAuthoritativeEntityOnCastLine(t *testing.T) {
	nav := navigation.Plane{MinX:-100,MaxX:100,MinZ:-100,MaxZ:100,Layer:0}
	sim := simulation.New(spatial.NewGrid(16), movement.NewService(nav, 0.1))
	for _, entity := range []world.EntityState{
		{ID:1,Kind:world.EntityPlayer,Transform:world.Transform{Position:world.Position{X:0,Z:0,Layer:0}}},
		{ID:2,Kind:world.EntityPlayer,Transform:world.Transform{Position:world.Position{X:.3,Z:5,Layer:0}}},
		{ID:3,Kind:world.EntityPlayer,Transform:world.Transform{Position:world.Position{X:0,Z:8,Layer:0}}},
	} {
		if err := sim.Spawn(entity, 6, 0.35, 0.5); err != nil { t.Fatal(err) }
	}
	combatService, err := combat.NewService([]combat.ActionDefinition{{
		ID:"fireball",Targets:[]combat.TargetKind{combat.TargetPoint},Range:12,HitRadius:.9,
		BaseDamage:150,DamageType:combat.DamagePhysical,CooldownSeconds:2,
	}})
	if err != nil { t.Fatal(err) }
	cfg := DefaultConfig()
	cfg.SnapshotEveryTicks = 100
	cfg.CharacterMaxHP = 200
	rt := New(sim, cfg, WithDynamicWorld(&characterCombatDynamic{los:true}), WithCombatService(combatService))
	for id := uint64(1); id <= 3; id++ {
		conn := session.NewQueueConnection(64,64)
		s, err := session.New(session.ID(id), world.EntityID(id), 20, conn)
		if err != nil { t.Fatal(err) }
		if err := rt.EnqueueRegister(s); err != nil { t.Fatal(err) }
	}
	initial := rt.Step(1, 50*time.Millisecond)
	if len(initial.CommandErrors) != 0 { t.Fatalf("initial errors=%#v", initial.CommandErrors) }

	x, z := float32(0), float32(10)
	if err := rt.EnqueueUseAction(1, 1, protocol.ClientUseAction{
		ActionID:"fireball", TargetKind:protocol.ActionTargetPoint, TargetX:&x, TargetZ:&z,
	}); err != nil { t.Fatal(err) }
	report := rt.Step(2, 50*time.Millisecond)
	if len(report.CommandErrors) != 0 || len(report.ActionRejections) != 0 { t.Fatalf("report=%#v", report) }

	first, ok := rt.characters.State(2)
	if !ok || first.HP != 50 || first.Defeated { t.Fatalf("first target=%#v ok=%v", first, ok) }
	second, ok := rt.characters.State(3)
	if !ok || second.HP != 200 || second.Defeated { t.Fatalf("second target=%#v ok=%v", second, ok) }

	if err := rt.EnqueueUseAction(1, 2, protocol.ClientUseAction{
		ActionID:"fireball", TargetKind:protocol.ActionTargetPoint, TargetX:&x, TargetZ:&z,
	}); err != nil { t.Fatal(err) }
	cooldown := rt.Step(3, 50*time.Millisecond)
	if len(cooldown.ActionRejections) != 1 || !errors.Is(cooldown.ActionRejections[0].Err, combat.ErrActionCooldown) {
		t.Fatalf("cooldown=%#v", cooldown.ActionRejections)
	}
}

func TestPointActionEndpointNearestIgnoresInterveningEntity(t *testing.T) {
	nav := navigation.Plane{MinX:-100,MaxX:100,MinZ:-100,MaxZ:100,Layer:0}
	sim := simulation.New(spatial.NewGrid(16), movement.NewService(nav, 0.1))
	for _, entity := range []world.EntityState{
		{ID:1,Kind:world.EntityPlayer,Transform:world.Transform{Position:world.Position{X:0,Z:0,Layer:0}}},
		{ID:2,Kind:world.EntityPlayer,Transform:world.Transform{Position:world.Position{X:0,Z:5,Layer:0}}},
		{ID:3,Kind:world.EntityPlayer,Transform:world.Transform{Position:world.Position{X:.25,Z:9.4,Layer:0}}},
	} {
		if err := sim.Spawn(entity, 6, 0.35, 0.5); err != nil { t.Fatal(err) }
	}
	combatService, err := combat.NewService([]combat.ActionDefinition{{
		ID:"meteor-strike",Targets:[]combat.TargetKind{combat.TargetPoint},Range:10,HitRadius:1.75,
		PointResolution:combat.PointResolutionEndpointNearest,
		BaseDamage:175,DamageType:combat.DamagePhysical,CooldownSeconds:4.5,
	}})
	if err != nil { t.Fatal(err) }
	cfg := DefaultConfig()
	cfg.SnapshotEveryTicks = 100
	cfg.CharacterMaxHP = 200
	rt := New(sim, cfg, WithDynamicWorld(&characterCombatDynamic{los:true}), WithCombatService(combatService))
	for id := uint64(1); id <= 3; id++ {
		conn := session.NewQueueConnection(64,64)
		s, err := session.New(session.ID(id), world.EntityID(id), 20, conn)
		if err != nil { t.Fatal(err) }
		if err := rt.EnqueueRegister(s); err != nil { t.Fatal(err) }
	}
	initial := rt.Step(1, 50*time.Millisecond)
	if len(initial.CommandErrors) != 0 { t.Fatalf("initial errors=%#v", initial.CommandErrors) }

	x, z := float32(0), float32(10)
	if err := rt.EnqueueUseAction(1, 1, protocol.ClientUseAction{
		ActionID:"meteor-strike", TargetKind:protocol.ActionTargetPoint, TargetX:&x, TargetZ:&z,
	}); err != nil { t.Fatal(err) }
	report := rt.Step(2, 50*time.Millisecond)
	if len(report.CommandErrors) != 0 || len(report.ActionRejections) != 0 { t.Fatalf("report=%#v", report) }

	intervening, ok := rt.characters.State(2)
	if !ok || intervening.HP != 200 || intervening.Defeated { t.Fatalf("intervening target=%#v ok=%v", intervening, ok) }
	impact, ok := rt.characters.State(3)
	if !ok || impact.HP != 25 || impact.Defeated { t.Fatalf("impact target=%#v ok=%v", impact, ok) }
}

func TestPointActionRejectsEndpointPastServerRange(t *testing.T) {
	rt, _, _ := makeCharacterCombatRuntime(t)
	fireball, err := combat.NewService([]combat.ActionDefinition{{
		ID:"fireball",Targets:[]combat.TargetKind{combat.TargetPoint},Range:4,HitRadius:.9,
		BaseDamage:150,DamageType:combat.DamagePhysical,CooldownSeconds:2,
	}})
	if err != nil { t.Fatal(err) }
	rt.combat = fireball
	rt.Step(1, 50*time.Millisecond)
	x, z := float32(0), float32(8)
	if err := rt.EnqueueUseAction(1, 1, protocol.ClientUseAction{ActionID:"fireball",TargetKind:protocol.ActionTargetPoint,TargetX:&x,TargetZ:&z}); err != nil { t.Fatal(err) }
	report := rt.Step(2, 50*time.Millisecond)
	if len(report.ActionRejections) != 1 || !errors.Is(report.ActionRejections[0].Err, ErrPointOutOfRange) {
		t.Fatalf("out of range=%#v", report.ActionRejections)
	}
}
