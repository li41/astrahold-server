package worldruntime

import (
	"errors"
	"testing"

	"github.com/li41/astrahold-server/internal/characterstats"
	"github.com/li41/astrahold-server/internal/equipmentaffix"
	"github.com/li41/astrahold-server/internal/inventory"
	"github.com/li41/astrahold-server/internal/iteminstance"
)

func TestHighArmorRequirementUsesBaseStatOnly(t *testing.T) {
	inv := newCharacterInventory(16)
	instance := iteminstance.Instance{
		ID:              "item-instance:starforged-test",
		ItemArchetypeID: "item_starforged_bastion_helm",
		Affixes: []equipmentaffix.Affix{
			{ID: equipmentaffix.AffixConstitution, Strength: 1, Value: 1},
			{ID: equipmentaffix.AffixPhysicalDefense, Strength: 1, Value: 1},
		},
	}
	definition, ok := defaultEquipmentCatalog.Resolve(instance.ItemArchetypeID)
	if !ok {
		t.Fatal("starforged helm missing")
	}
	if err := iteminstance.Validate(instance, definition); err != nil {
		t.Fatal(err)
	}
	if err := inv.AddInstance(instance); err != nil {
		t.Fatal(err)
	}
	base := characterstats.DefaultPrimary()
	if err := validateEquipmentInstanceBaseRequirements(inv, instance.ID, base); !errors.Is(err, ErrEquipmentRequirementsNotMet) {
		t.Fatalf("base constitution 10 err=%v want requirement rejection", err)
	}
	base.Constitution = 18
	if err := validateEquipmentInstanceBaseRequirements(inv, instance.ID, base); err != nil {
		t.Fatalf("base constitution 18 err=%v", err)
	}
}

func TestFivePieceArmorSetRecomputesThroughEquipmentStats(t *testing.T) {
	runtime, s := runtimeWithJoinedPlayerForStaticModifierTest(t)
	inv := runtime.inventories[s.CharacterIdentity.ID]
	if inv == nil {
		t.Fatal("inventory missing")
	}
	pieces := []struct {
		id   string
		slot inventory.EquipmentSlot
	}{
		{"item_garrison_steel_helm", inventory.SlotHelmet},
		{"item_garrison_steel_cuirass", inventory.SlotChest},
		{"item_garrison_steel_gauntlets", inventory.SlotGloves},
		{"item_garrison_steel_greaves", inventory.SlotLegs},
		{"item_garrison_steel_boots", inventory.SlotBoots},
	}
	for index, piece := range pieces {
		definition, ok := defaultEquipmentCatalog.Resolve(piece.id)
		if !ok {
			t.Fatalf("missing %s", piece.id)
		}
		instance := iteminstance.Instance{
			ID:              iteminstance.ID("item-instance:garrison-" + string(rune('a'+index))),
			ItemArchetypeID: piece.id,
			Affixes: []equipmentaffix.Affix{{
				ID: equipmentaffix.AffixMagicDefense, Strength: 1, Value: 1,
			}},
		}
		if err := iteminstance.Validate(instance, definition); err != nil {
			t.Fatalf("validate %s: %v", piece.id, err)
		}
		if err := inv.AddInstance(instance); err != nil {
			t.Fatal(err)
		}
		if err := inv.EquipInstance(piece.slot, instance.ID); err != nil {
			t.Fatal(err)
		}
	}
	modifiers, err := runtime.equippedInstanceModifiers(s.EntityID)
	if err != nil {
		t.Fatal(err)
	}
	if modifiers.MaxHP != 40 || modifiers.Strength != 2 || modifiers.PhysicalDefense != 14 || modifiers.MagicDefense != 9 {
		t.Fatalf("five-piece modifiers=%#v", modifiers)
	}
}
