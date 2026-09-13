package inventory

import (
	"errors"
	"testing"

	"github.com/li41/astrahold-server/internal/equipmentaffix"
	"github.com/li41/astrahold-server/internal/iteminstance"
)

func testInstance(id, archetype string) iteminstance.Instance {
	return iteminstance.Instance{
		ID:              iteminstance.ID(id),
		ItemArchetypeID: archetype,
		Affixes: []equipmentaffix.Affix{{
			ID:       equipmentaffix.AffixStrength,
			Strength: 1,
			Value:    1,
		}},
	}
}

func TestUniqueInstanceAddEquipUnequipPreservesIdentityAndAffixes(t *testing.T) {
	inv := NewWithWeightPolicy(2, WeightPolicy{
		MaxWeight:   100,
		UnitWeights: map[string]uint32{"item_mid_blade": 7},
	})
	original := testInstance("instance-1", "item_mid_blade")
	if err := inv.AddInstance(original); err != nil {
		t.Fatalf("AddInstance: %v", err)
	}
	if got := inv.CurrentWeight(); got != 7 {
		t.Fatalf("weight=%d want=7", got)
	}
	if err := inv.EquipMainHandInstance(original.ID); err != nil {
		t.Fatalf("EquipMainHandInstance: %v", err)
	}
	if got := inv.MainHand(); got != original.ItemArchetypeID {
		t.Fatalf("main hand archetype=%q", got)
	}
	equipped, ok := inv.MainHandInstance()
	if !ok || equipped.ID != original.ID || len(equipped.Affixes) != 1 || equipped.Affixes[0] != original.Affixes[0] {
		t.Fatalf("equipped=%#v", equipped)
	}
	if _, ok := inv.Instance(original.ID); ok {
		t.Fatal("equipped instance remained in unequipped inventory")
	}
	restored, err := inv.UnequipMainHandInstance()
	if err != nil {
		t.Fatalf("UnequipMainHandInstance: %v", err)
	}
	if restored.ID != original.ID || restored.Affixes[0] != original.Affixes[0] {
		t.Fatalf("restored=%#v", restored)
	}
	if got := inv.CurrentWeight(); got != 7 {
		t.Fatalf("equip cycle changed carried weight: %d", got)
	}
}

func TestUniqueInstanceCannotDuplicateIdentity(t *testing.T) {
	inv := New(2)
	instance := testInstance("instance-1", "item_mid_blade")
	if err := inv.AddInstance(instance); err != nil {
		t.Fatal(err)
	}
	if err := inv.AddInstance(instance); !errors.Is(err, ErrInstanceExists) {
		t.Fatalf("duplicate err=%v want ErrInstanceExists", err)
	}
}

func TestStackAndUniqueInstanceShareCapacity(t *testing.T) {
	inv := New(1)
	if err := inv.Add("item_potion", 5); err != nil {
		t.Fatal(err)
	}
	if err := inv.AddInstance(testInstance("instance-1", "item_mid_blade")); !errors.Is(err, ErrFull) {
		t.Fatalf("instance add err=%v want ErrFull", err)
	}
}

func TestInstanceSnapshotIsCanonicalAndDefensive(t *testing.T) {
	inv := New(3)
	second := testInstance("instance-2", "item_mid_blade")
	first := testInstance("instance-1", "item_mid_blade")
	if err := inv.AddInstance(second); err != nil { t.Fatal(err) }
	if err := inv.AddInstance(first); err != nil { t.Fatal(err) }

	snapshot := inv.InstanceSnapshot()
	if len(snapshot) != 2 || snapshot[0].ID != first.ID || snapshot[1].ID != second.ID {
		t.Fatalf("snapshot=%#v", snapshot)
	}
	snapshot[0].Affixes[0].Value = 99
	again, ok := inv.Instance(first.ID)
	if !ok || again.Affixes[0].Value != 1 {
		t.Fatal("snapshot mutation leaked into authoritative inventory")
	}
}

func TestInstanceUnequipRejectsFullInventoryWithoutLosingEquipment(t *testing.T) {
	inv := New(1)
	instance := testInstance("instance-1", "item_mid_blade")
	if err := inv.AddInstance(instance); err != nil { t.Fatal(err) }
	if err := inv.EquipMainHandInstance(instance.ID); err != nil { t.Fatal(err) }
	if err := inv.Add("item_potion", 1); err != nil { t.Fatal(err) }

	if _, err := inv.UnequipMainHandInstance(); !errors.Is(err, ErrFull) {
		t.Fatalf("unequip err=%v want ErrFull", err)
	}
	equipped, ok := inv.MainHandInstance()
	if !ok || equipped.ID != instance.ID {
		t.Fatalf("equipped instance lost: %#v", equipped)
	}
}
