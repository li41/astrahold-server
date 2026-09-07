package worldruntime

import (
	"errors"
	"testing"
	"time"

	"github.com/li41/astrahold-server/internal/characterstate"
	"github.com/li41/astrahold-server/internal/protocol"
)

const manaSiphonStaffArchetypeID = "item_mana_siphon_staff"

func equipManaSiphonStaffForCombatTest(t *testing.T, rt *Runtime, sessionID uint64) {
	t.Helper()
	s, ok := rt.sessions.Get(session.ID(sessionID))
	if !ok {
		t.Fatalf("session %d not found", sessionID)
	}
	inv := rt.inventories[s.CharacterIdentity.ID]
	if inv == nil {
		t.Fatal("inventory missing")
	}
	if err := inv.Add(manaSiphonStaffArchetypeID, 1); err != nil {
		t.Fatal(err)
	}
	if err := inv.EquipMainHand(manaSiphonStaffArchetypeID); err != nil {
		t.Fatal(err)
	}
}

func TestManaSiphonStaffRestoresMPOnlyAfterAuthoritativeDamageHit(t *testing.T) {
	rt, conn1, conn2 := makeCharacterCombatRuntime(t)
	report := rt.Step(1, 50*time.Millisecond)
	if len(report.CommandErrors) != 0 {
		t.Fatalf("initial errors = %#v", report.CommandErrors)
	}
	drainConnection(conn1)
	drainConnection(conn2)
	equipManaSiphonStaffForCombatTest(t, rt, 1)
	if _, err := rt.characters.SpendMP(1, 60); err != nil {
		t.Fatal(err)
	}

	if err := rt.EnqueueUseAction(1, 1, protocol.ClientUseAction{ActionID: "basic-attack", TargetKind: protocol.ActionTargetEntity, TargetID: "2"}); err != nil {
		t.Fatal(err)
	}
	hit := rt.Step(2, 50*time.Millisecond)
	if len(hit.CommandErrors) != 0 || len(hit.ActionRejections) != 0 {
		t.Fatalf("hit report = %#v", hit)
	}
	actor, _ := rt.characters.State(1)
	if actor.MP != 45 {
		t.Fatalf("actor MP after hit = %d, want 45", actor.MP)
	}
	target, _ := rt.characters.State(2)
	if target.HP != 100 {
		t.Fatalf("target HP after hit = %d, want 100", target.HP)
	}
	assertReplicatedMP(t, conn1, 1, 45)

	if err := rt.EnqueueUseAction(1, 2, protocol.ClientUseAction{ActionID: "basic-attack", TargetKind: protocol.ActionTargetEntity, TargetID: "2"}); err != nil {
		t.Fatal(err)
	}
	rejected := rt.Step(3, 50*time.Millisecond)
	if len(rejected.ActionRejections) != 1 {
		t.Fatalf("cooldown rejections = %#v", rejected.ActionRejections)
	}
	actor, _ = rt.characters.State(1)
	if actor.MP != 45 {
		t.Fatalf("actor MP changed on rejected attack = %d, want 45", actor.MP)
	}
}

func TestManaSiphonStaffClampsRestoreToMaxMP(t *testing.T) {
	rt, conn1, conn2 := makeCharacterCombatRuntime(t)
	rt.Step(1, 50*time.Millisecond)
	drainConnection(conn1)
	drainConnection(conn2)
	equipManaSiphonStaffForCombatTest(t, rt, 1)
	if _, err := rt.characters.SpendMP(1, 3); err != nil {
		t.Fatal(err)
	}

	if err := rt.EnqueueUseAction(1, 1, protocol.ClientUseAction{ActionID: "basic-attack", TargetKind: protocol.ActionTargetEntity, TargetID: "2"}); err != nil {
		t.Fatal(err)
	}
	report := rt.Step(2, 50*time.Millisecond)
	if len(report.CommandErrors) != 0 || len(report.ActionRejections) != 0 {
		t.Fatalf("hit report = %#v", report)
	}
	actor, _ := rt.characters.State(1)
	if actor.MP != actor.MaxMP || actor.MP != 100 {
		t.Fatalf("actor MP after clamped siphon = %d/%d, want 100/100", actor.MP, actor.MaxMP)
	}
}

func TestNonEffectMainHandDoesNotRestoreMP(t *testing.T) {
	rt, conn1, conn2 := makeCharacterCombatRuntime(t)
	rt.Step(1, 50*time.Millisecond)
	drainConnection(conn1)
	drainConnection(conn2)
	s, _ := rt.sessions.Get(1)
	inv := rt.inventories[s.CharacterIdentity.ID]
	if err := inv.EquipMainHand(trainingBladeArchetypeID); err != nil {
		t.Fatal(err)
	}
	if _, err := rt.characters.SpendMP(1, 60); err != nil {
		t.Fatal(err)
	}

	if err := rt.EnqueueUseAction(1, 1, protocol.ClientUseAction{ActionID: "basic-attack", TargetKind: protocol.ActionTargetEntity, TargetID: "2"}); err != nil {
		t.Fatal(err)
	}
	report := rt.Step(2, 50*time.Millisecond)
	if len(report.CommandErrors) != 0 || len(report.ActionRejections) != 0 {
		t.Fatalf("hit report = %#v", report)
	}
	actor, _ := rt.characters.State(1)
	if actor.MP != 40 {
		t.Fatalf("training blade restored MP: got %d want 40", actor.MP)
	}
}

func TestManaSiphonStaffUsesFormalEquipmentAndDurableRestorePaths(t *testing.T) {
	rt, _, s, connection := newNPCTestRuntime(t, world.Position{})
	report := rt.Step(1, 50*time.Millisecond)
	if len(report.CommandErrors) != 0 {
		t.Fatalf("initial errors = %#v", report.CommandErrors)
	}
	drainConnection(connection)
	inv := rt.inventories[s.CharacterIdentity.ID]
	if err := inv.Add(manaSiphonStaffArchetypeID, 1); err != nil {
		t.Fatal(err)
	}

	if err := rt.EnqueueEquipmentCommand(s.ID, 1, protocol.ClientEquipmentCommand{Operation: protocol.EquipmentOperationEquip, Slot: protocol.EquipmentSlotMainHand, ItemArchetypeID: manaSiphonStaffArchetypeID}); err != nil {
		t.Fatal(err)
	}
	equip := rt.Step(2, 50*time.Millisecond)
	if len(equip.CommandErrors) != 0 {
		t.Fatalf("equip errors = %#v", equip.CommandErrors)
	}
	if inv.MainHand() != manaSiphonStaffArchetypeID {
		t.Fatalf("main hand = %q, want staff", inv.MainHand())
	}

	durable, err := durableInventoryState(inv)
	if err != nil {
		t.Fatal(err)
	}
	if !durable.Initialized || durable.MainHand != manaSiphonStaffArchetypeID {
		t.Fatalf("durable inventory = %#v", durable)
	}
	restored, err := restoreCharacterInventory(32, durable)
	if err != nil {
		t.Fatal(err)
	}
	if restored.MainHand() != manaSiphonStaffArchetypeID {
		t.Fatalf("restored main hand = %q, want staff", restored.MainHand())
	}
	if restored.CurrentWeight() != inv.CurrentWeight() {
		t.Fatalf("restored carry weight = %d, want %d", restored.CurrentWeight(), inv.CurrentWeight())
	}

	invalid, err := characterstate.NewInventoryState(nil, "item_not_a_weapon")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := restoreCharacterInventory(32, invalid); !errors.Is(err, ErrEquipmentItemNotAllowed) {
		t.Fatalf("invalid main hand restore error = %v, want ErrEquipmentItemNotAllowed", err)
	}
}

func assertReplicatedMP(t *testing.T, conn *session.QueueConnection, entityID world.EntityID, want uint32) {
	t.Helper()
	for {
		select {
		case envelope := <-conn.Reliable():
			vitals, ok := envelope.Message.(protocol.EntityVitalsState)
			if !ok || vitals.EntityID != entityID {
				continue
			}
			if vitals.MP != want {
				t.Fatalf("replicated MP = %d, want %d", vitals.MP, want)
			}
			return
		default:
			t.Fatalf("missing vitals replication for entity %d MP %d", entityID, want)
		}
	}
}
