package worldruntime

import (
	"errors"
	"testing"
	"time"

	"github.com/li41/astrahold-server/internal/character"
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

type characterCombatDynamic struct{ los bool }

func (d *characterCombatDynamic) SetBlockerEnabled(string, bool) error { return nil }
func (d *characterCombatDynamic) BlockerEnabled(string) (bool, error) { return false, errors.New("unused") }
func (d *characterCombatDynamic) BlockerDefinition(string) (gameplayworld.Blocker, error) { return gameplayworld.Blocker{}, errors.New("unused") }
func (d *characterCombatDynamic) BlockerStates() []gameplayworld.BlockerState { return nil }
func (d *characterCombatDynamic) HasLineOfSight(world.Position, world.Position) bool { return d.los }
func (d *characterCombatDynamic) HasLineOfSightIgnoringBlocker(world.Position, world.Position, string) bool { return d.los }

func TestEntityActionDamagesCooldownsAndDefeats(t *testing.T) {
	rt, conn1, conn2 := makeCharacterCombatRuntime(t)
	initial := rt.Step(1, 50*time.Millisecond)
	if len(initial.CommandErrors) != 0 { t.Fatalf("initial errors=%#v", initial.CommandErrors) }
	drainConnection(conn1)
	drainConnection(conn2)

	if err := rt.EnqueueUseAction(1, 1, protocol.ClientUseAction{ActionID:"basic-attack",TargetKind:protocol.ActionTargetEntity,TargetID:"2"}); err != nil { t.Fatal(err) }
	first := rt.Step(2, 50*time.Millisecond)
	if len(first.CommandErrors) != 0 || len(first.ActionRejections) != 0 { t.Fatalf("first=%#v", first) }
	state, ok := rt.characters.State(2)
	if !ok || state.HP != 100 || state.Defeated { t.Fatalf("after first=%#v ok=%v", state, ok) }
	assertVitals(t, conn1, 2, 100, false)
	assertVitals(t, conn2, 2, 100, false)

	if err := rt.EnqueueUseAction(1, 2, protocol.ClientUseAction{ActionID:"basic-attack",TargetKind:protocol.ActionTargetEntity,TargetID:"2"}); err != nil { t.Fatal(err) }
	cooldown := rt.Step(3, 50*time.Millisecond)
	if len(cooldown.ActionRejections) != 1 || !errors.Is(cooldown.ActionRejections[0].Err, combat.ErrActionCooldown) {
		t.Fatalf("cooldown=%#v", cooldown.ActionRejections)
	}

	if err := rt.EnqueueUseAction(1, 3, protocol.ClientUseAction{ActionID:"basic-attack",TargetKind:protocol.ActionTargetEntity,TargetID:"2"}); err != nil { t.Fatal(err) }
	defeat := rt.Step(12, 50*time.Millisecond)
	if len(defeat.CommandErrors) != 0 || len(defeat.ActionRejections) != 0 { t.Fatalf("defeat=%#v", defeat) }
	state, _ = rt.characters.State(2)
	if state.HP != 0 || !state.Defeated { t.Fatalf("defeated state=%#v", state) }
	assertVitals(t, conn1, 2, 0, true)
	assertVitals(t, conn2, 2, 0, true)

	if err := rt.EnqueueUseAction(1, 4, protocol.ClientUseAction{ActionID:"basic-attack",TargetKind:protocol.ActionTargetEntity,TargetID:"2"}); err != nil { t.Fatal(err) }
	dead := rt.Step(22, 50*time.Millisecond)
	if len(dead.ActionRejections) != 1 || !errors.Is(dead.ActionRejections[0].Err, character.ErrCharacterDefeated) {
		t.Fatalf("defeated rejection=%#v", dead.ActionRejections)
	}
}

func TestEntityActionRejectsSelfTargetWithoutCooldownCommit(t *testing.T) {
	rt, conn1, conn2 := makeCharacterCombatRuntime(t)
	rt.Step(1, 50*time.Millisecond)
	drainConnection(conn1)
	drainConnection(conn2)

	if err := rt.EnqueueUseAction(1, 1, protocol.ClientUseAction{ActionID:"basic-attack",TargetKind:protocol.ActionTargetEntity,TargetID:"1"}); err != nil { t.Fatal(err) }
	self := rt.Step(2, 50*time.Millisecond)
	if len(self.ActionRejections) != 1 || !errors.Is(self.ActionRejections[0].Err, ErrSelfTarget) { t.Fatalf("self=%#v", self.ActionRejections) }

	if err := rt.EnqueueUseAction(1, 2, protocol.ClientUseAction{ActionID:"basic-attack",TargetKind:protocol.ActionTargetEntity,TargetID:"2"}); err != nil { t.Fatal(err) }
	next := rt.Step(3, 50*time.Millisecond)
	if len(next.ActionRejections) != 0 || len(next.CommandErrors) != 0 { t.Fatalf("next=%#v", next) }
	state, _ := rt.characters.State(2)
	if state.HP != 100 { t.Fatalf("target state=%#v", state) }
}

func makeCharacterCombatRuntime(t *testing.T) (*Runtime, *session.QueueConnection, *session.QueueConnection) {
	t.Helper()
	nav := navigation.Plane{MinX:-100,MaxX:100,MinZ:-100,MaxZ:100,Layer:0}
	sim := simulation.New(spatial.NewGrid(16), movement.NewService(nav, 0.1))
	for _, entity := range []world.EntityState{
		{ID:1,Kind:world.EntityPlayer,Transform:world.Transform{Position:world.Position{X:0,Layer:0}}},
		{ID:2,Kind:world.EntityPlayer,Transform:world.Transform{Position:world.Position{X:2,Layer:0}}},
	} {
		if err := sim.Spawn(entity, 6, 0.35, 0.5); err != nil { t.Fatal(err) }
	}
	combatService, err := combat.NewService([]combat.ActionDefinition{{ID:"basic-attack",Targets:[]combat.TargetKind{combat.TargetEntity},Range:4.5,BaseDamage:100,DamageType:combat.DamagePhysical,CooldownSeconds:0.5}})
	if err != nil { t.Fatal(err) }
	cfg := DefaultConfig()
	cfg.SnapshotEveryTicks = 1
	cfg.CharacterMaxHP = 200
	rt := New(sim, cfg, WithDynamicWorld(&characterCombatDynamic{los:true}), WithCombatService(combatService))
	conn1 := session.NewQueueConnection(64,64)
	conn2 := session.NewQueueConnection(64,64)
	s1, err := session.New(1,1,20,conn1); if err != nil { t.Fatal(err) }
	s2, err := session.New(2,2,20,conn2); if err != nil { t.Fatal(err) }
	if err := rt.EnqueueRegister(s1); err != nil { t.Fatal(err) }
	if err := rt.EnqueueRegister(s2); err != nil { t.Fatal(err) }
	return rt, conn1, conn2
}

func assertVitals(t *testing.T, conn *session.QueueConnection, entityID world.EntityID, hp uint32, defeated bool) {
	t.Helper()
	for {
		select {
		case env := <-conn.Reliable():
			if state, ok := env.Message.(protocol.EntityVitalsState); ok && state.EntityID == entityID {
				if state.HP != hp || state.MaxHP != 200 || state.Defeated != defeated { t.Fatalf("vitals=%#v", state) }
				return
			}
		default:
			t.Fatalf("missing vitals entity=%d hp=%d defeated=%v", entityID, hp, defeated)
		}
	}
}

func drainConnection(conn *session.QueueConnection) {
	for {
		select { case <-conn.Reliable(): default: goto realtime }
	}
realtime:
	for {
		select { case <-conn.Realtime(): default: return }
	}
}
