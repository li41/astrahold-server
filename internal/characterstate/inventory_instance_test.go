package characterstate

import (
	"testing"

	"github.com/li41/astrahold-server/internal/equipmentaffix"
	"github.com/li41/astrahold-server/internal/iteminstance"
)

func durableTestInstance(id, archetype string, affix equipmentaffix.Affix) iteminstance.Instance {
	return iteminstance.Instance{
		ID:              iteminstance.ID(id),
		ItemArchetypeID: archetype,
		Affixes:         []equipmentaffix.Affix{affix},
	}
}

func TestInventoryStateCanonicalizesUniqueInstancesWithoutReroll(t *testing.T) {
	bag := durableTestInstance("instance-b", "item_mid_blade", equipmentaffix.Affix{
		ID: equipmentaffix.AffixStrength, Strength: 2, Value: 2,
	})
	main := durableTestInstance("instance-a", "item_mid_blade", equipmentaffix.Affix{
		ID: equipmentaffix.AffixPhysicalDamage, Strength: 1, Value: 1,
	})

	state, err := NewInventoryStateWithInstances(nil, []iteminstance.Instance{bag}, "", "", &main, nil)
	if err != nil {
		t.Fatalf("NewInventoryStateWithInstances: %v", err)
	}
	canonical, err := CanonicalInventoryState(state)
	if err != nil {
		t.Fatalf("CanonicalInventoryState: %v", err)
	}
	if canonical != state {
		t.Fatalf("canonical state changed: got=%#v want=%#v", canonical, state)
	}

	instances, err := canonical.Instances()
	if err != nil || len(instances) != 1 || instances[0].ID != bag.ID || instances[0].Affixes[0] != bag.Affixes[0] {
		t.Fatalf("bag restore=%#v err=%v", instances, err)
	}
	restoredMain, ok, err := canonical.MainHandInstance()
	if err != nil || !ok || restoredMain.ID != main.ID || restoredMain.Affixes[0] != main.Affixes[0] {
		t.Fatalf("main restore=%#v ok=%v err=%v", restoredMain, ok, err)
	}
}

func TestInventoryStateRejectsDuplicateInstanceIdentityAcrossBagAndEquipment(t *testing.T) {
	bag := durableTestInstance("same-id", "item_mid_blade", equipmentaffix.Affix{
		ID: equipmentaffix.AffixStrength, Strength: 1, Value: 1,
	})
	main := durableTestInstance("same-id", "item_mid_blade", equipmentaffix.Affix{
		ID: equipmentaffix.AffixPhysicalDamage, Strength: 1, Value: 1,
	})
	if _, err := NewInventoryStateWithInstances(nil, []iteminstance.Instance{bag}, "", "", &main, nil); err == nil {
		t.Fatal("duplicate item instance identity unexpectedly accepted")
	}
}

func TestInventoryStateRejectsLegacyAndInstanceEquipmentInSameSlot(t *testing.T) {
	main := durableTestInstance("instance-a", "item_mid_blade", equipmentaffix.Affix{
		ID: equipmentaffix.AffixPhysicalDamage, Strength: 1, Value: 1,
	})
	if _, err := NewInventoryStateWithInstances(nil, nil, "item_militia_iron_sword", "", &main, nil); err == nil {
		t.Fatal("legacy and unique main-hand equipment unexpectedly accepted together")
	}
}
