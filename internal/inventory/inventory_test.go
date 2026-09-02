package inventory

import (
	"errors"
	"math"
	"reflect"
	"testing"
)

func TestInventoryStackLifecycleAndRevision(t *testing.T) {
	inv := New(2)
	if got := inv.Revision(); got != 0 {
		t.Fatalf("initial revision = %d, want 0", got)
	}
	if err := inv.Add("item_minor_mana_potion", 3); err != nil {
		t.Fatalf("add mana potion: %v", err)
	}
	if err := inv.Add("item_minor_mana_potion", 2); err != nil {
		t.Fatalf("stack mana potion: %v", err)
	}
	if got := inv.Quantity("item_minor_mana_potion"); got != 5 {
		t.Fatalf("quantity = %d, want 5", got)
	}
	if got := inv.Revision(); got != 2 {
		t.Fatalf("revision = %d, want 2", got)
	}

	if err := inv.Remove("item_minor_mana_potion", 5); err != nil {
		t.Fatalf("remove full stack: %v", err)
	}
	if got := inv.Quantity("item_minor_mana_potion"); got != 0 {
		t.Fatalf("quantity after removal = %d, want 0", got)
	}
	if got := inv.Revision(); got != 3 {
		t.Fatalf("revision = %d, want 3", got)
	}
}

func TestInventoryRejectedMutationHasNoSideEffects(t *testing.T) {
	inv := New(1)
	if err := inv.Add("item_minor_healing_potion", 4); err != nil {
		t.Fatalf("seed inventory: %v", err)
	}
	beforeRevision := inv.Revision()
	before := inv.Snapshot()

	if err := inv.Add("item_training_blade", 1); !errors.Is(err, ErrFull) {
		t.Fatalf("full add error = %v, want ErrFull", err)
	}
	if err := inv.Remove("item_minor_healing_potion", 5); !errors.Is(err, ErrInsufficient) {
		t.Fatalf("over-remove error = %v, want ErrInsufficient", err)
	}

	if got := inv.Revision(); got != beforeRevision {
		t.Fatalf("revision changed on rejection: got %d want %d", got, beforeRevision)
	}
	if got := inv.Snapshot(); !reflect.DeepEqual(got, before) {
		t.Fatalf("snapshot changed on rejection: got %#v want %#v", got, before)
	}
}

func TestInventoryQuantityOverflowHasNoSideEffects(t *testing.T) {
	inv := New(1)
	if err := inv.Add("item_currency_shard", math.MaxUint32); err != nil {
		t.Fatalf("seed max stack: %v", err)
	}
	beforeRevision := inv.Revision()

	if err := inv.Add("item_currency_shard", 1); !errors.Is(err, ErrQuantityOverflow) {
		t.Fatalf("overflow error = %v, want ErrQuantityOverflow", err)
	}
	if got := inv.Quantity("item_currency_shard"); got != math.MaxUint32 {
		t.Fatalf("quantity changed after overflow: got %d", got)
	}
	if got := inv.Revision(); got != beforeRevision {
		t.Fatalf("revision changed after overflow: got %d want %d", got, beforeRevision)
	}
}

func TestInventorySnapshotIsDeterministicAndDetached(t *testing.T) {
	inv := New(3)
	for _, stack := range []Stack{
		{ArchetypeID: "item_training_blade", Quantity: 1},
		{ArchetypeID: "item_minor_mana_potion", Quantity: 2},
		{ArchetypeID: "item_minor_healing_potion", Quantity: 3},
	} {
		if err := inv.Add(stack.ArchetypeID, stack.Quantity); err != nil {
			t.Fatalf("add %s: %v", stack.ArchetypeID, err)
		}
	}

	got := inv.Snapshot()
	want := []Stack{
		{ArchetypeID: "item_minor_healing_potion", Quantity: 3},
		{ArchetypeID: "item_minor_mana_potion", Quantity: 2},
		{ArchetypeID: "item_training_blade", Quantity: 1},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("snapshot = %#v, want %#v", got, want)
	}

	got[0].Quantity = 999
	if quantity := inv.Quantity("item_minor_healing_potion"); quantity != 3 {
		t.Fatalf("snapshot mutation leaked into authority: got %d want 3", quantity)
	}
}
