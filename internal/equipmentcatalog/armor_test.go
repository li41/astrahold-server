package equipmentcatalog

import "testing"

func TestDefaultLowTierArmorCatalog(t *testing.T) {
	catalog, err := Default()
	if err != nil {
		t.Fatal(err)
	}
	if got := catalog.Revision(); got != "weapon-progression-v1" {
		t.Fatalf("revision=%q", got)
	}

	classes := []struct {
		name  string
		class ArmorClass
	}{
		{"cloth", ArmorClassCloth},
		{"leather", ArmorClassLeather},
		{"heavy", ArmorClassHeavy},
	}
	slots := []struct {
		name string
		slot Slot
	}{
		{"helmet", SlotHelmet},
		{"chest", SlotChest},
		{"gloves", SlotGloves},
		{"legs", SlotLegs},
		{"boots", SlotBoots},
	}
	for _, class := range classes {
		for _, slot := range slots {
			id := "item_low_" + class.name + "_" + slot.name
			definition, ok := catalog.Resolve(id)
			if !ok {
				t.Fatalf("missing %s", id)
			}
			if definition.Kind != KindArmor || definition.Tier != TierLow || definition.Slot != slot.slot || definition.ArmorClass != class.class {
				t.Fatalf("%s definition=%#v", id, definition)
			}
			if len(definition.StaticModifiers) == 0 {
				t.Fatalf("%s has no defense modifier", id)
			}
		}
	}
}
