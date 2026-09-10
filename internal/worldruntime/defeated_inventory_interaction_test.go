package worldruntime

import (
	"errors"
	"testing"
	"time"

	"github.com/li41/astrahold-server/internal/character"
	"github.com/li41/astrahold-server/internal/protocol"
	"github.com/li41/astrahold-server/internal/session"
	"github.com/li41/astrahold-server/internal/world"
)

func TestDefeatedActorCannotPickupGroundItemAndConsumesActionSequence(t *testing.T) {
	runtime, sim, s := newItemDropTestRuntime(t, world.Position{})
	dropID := spawnTestItemDrop(t, runtime, sim, world.Position{X: 1})
	state, ok := runtime.characters.State(s.EntityID)
	if !ok {
		t.Fatal("character state missing")
	}
	if _, err := runtime.characters.ApplyDamage(s.EntityID, state.MaxHP); err != nil {
		t.Fatal(err)
	}
	inv := runtime.inventories[s.CharacterIdentity.ID]
	beforeRevision := inv.Revision()

	intent := protocol.ClientPickupItem{DropEntityID: dropID}
	if err := runtime.EnqueuePickupItem(s.ID, 1, intent); err != nil {
		t.Fatal(err)
	}
	report := runtime.Step(2, 50*time.Millisecond)
	if len(report.CommandErrors) != 1 || !errors.Is(report.CommandErrors[0].Err, character.ErrCharacterDefeated) {
		t.Fatalf("defeated pickup errors=%#v, want ErrCharacterDefeated", report.CommandErrors)
	}
	if _, ok := sim.Entity(dropID); !ok {
		t.Fatal("defeated pickup removed authoritative ground drop")
	}
	if got := inv.Quantity(testItemDropArchetypeID); got != 0 {
		t.Fatalf("defeated pickup changed item quantity=%d, want 0", got)
	}
	if got := inv.Revision(); got != beforeRevision {
		t.Fatalf("defeated pickup changed inventory revision=%d, want %d", got, beforeRevision)
	}

	if err := runtime.EnqueuePickupItem(s.ID, 1, intent); err != nil {
		t.Fatal(err)
	}
	replay := runtime.Step(3, 50*time.Millisecond)
	if len(replay.CommandErrors) != 1 || !errors.Is(replay.CommandErrors[0].Err, session.ErrStaleAction) {
		t.Fatalf("replayed defeated pickup errors=%#v, want ErrStaleAction", replay.CommandErrors)
	}
	if _, ok := sim.Entity(dropID); !ok {
		t.Fatal("replayed defeated pickup removed authoritative ground drop")
	}
}

func TestDefeatedActorCannotEquipAndConsumesActionSequence(t *testing.T) {
	runtime, _, s := newItemDropTestRuntime(t, world.Position{})
	state, ok := runtime.characters.State(s.EntityID)
	if !ok {
		t.Fatal("character state missing")
	}
	if _, err := runtime.characters.ApplyDamage(s.EntityID, state.MaxHP); err != nil {
		t.Fatal(err)
	}
	inv := runtime.inventories[s.CharacterIdentity.ID]
	beforeRevision := inv.Revision()
	beforeQuantity := inv.Quantity(trainingBladeArchetypeID)

	intent := protocol.ClientEquipmentCommand{
		Operation:       protocol.EquipmentOperationEquip,
		Slot:            protocol.EquipmentSlotMainHand,
		ItemArchetypeID: trainingBladeArchetypeID,
	}
	if err := runtime.EnqueueEquipmentCommand(s.ID, 1, intent); err != nil {
		t.Fatal(err)
	}
	report := runtime.Step(2, 50*time.Millisecond)
	if len(report.CommandErrors) != 1 || !errors.Is(report.CommandErrors[0].Err, character.ErrCharacterDefeated) {
		t.Fatalf("defeated equip errors=%#v, want ErrCharacterDefeated", report.CommandErrors)
	}
	if got := inv.MainHand(); got != "" {
		t.Fatalf("defeated equip changed main hand=%q, want empty", got)
	}
	if got := inv.Quantity(trainingBladeArchetypeID); got != beforeQuantity {
		t.Fatalf("defeated equip changed blade quantity=%d, want %d", got, beforeQuantity)
	}
	if got := inv.Revision(); got != beforeRevision {
		t.Fatalf("defeated equip changed inventory revision=%d, want %d", got, beforeRevision)
	}

	if err := runtime.EnqueueEquipmentCommand(s.ID, 1, intent); err != nil {
		t.Fatal(err)
	}
	replay := runtime.Step(3, 50*time.Millisecond)
	if len(replay.CommandErrors) != 1 || !errors.Is(replay.CommandErrors[0].Err, session.ErrStaleAction) {
		t.Fatalf("replayed defeated equip errors=%#v, want ErrStaleAction", replay.CommandErrors)
	}
	if got := inv.MainHand(); got != "" {
		t.Fatalf("replayed defeated equip changed main hand=%q, want empty", got)
	}
}
