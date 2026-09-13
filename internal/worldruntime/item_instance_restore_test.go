package worldruntime

import (
	"errors"
	"testing"

	"github.com/li41/astrahold-server/internal/characterstate"
	"github.com/li41/astrahold-server/internal/equipmentaffix"
	"github.com/li41/astrahold-server/internal/equipmentcatalog"
	"github.com/li41/astrahold-server/internal/iteminstance"
)

func withMidTierRestoreCatalog(t *testing.T) string {
	t.Helper()
	interval := uint32(900)
	const itemID = "item_mid_restore_sword"
	catalog, err := equipmentcatalog.New(equipmentcatalog.CatalogDefinition{
		Revision: "item-instance-restore-test",
		WeaponTypes: []equipmentcatalog.WeaponTypeDefinition{{
			WeaponType: equipmentcatalog.WeaponTypeOneHandSword, BasicAttackIntervalMS: &interval,
		}},
		Items: []equipmentcatalog.Definition{{
			ItemArchetypeID: itemID,
			Kind: equipmentcatalog.KindWeapon,
			Slot: equipmentcatalog.SlotMainHand,
			Tier: equipmentcatalog.TierMid,
			Weight: 1,
			Material: "iron",
			Weapon: &equipmentcatalog.Weapon{
				WeaponType: equipmentcatalog.WeaponTypeOneHandSword,
				SmallDamage: equipmentcatalog.DamageRange{Min: 6, Max: 9},
				LargeDamage: equipmentcatalog.DamageRange{Min: 6, Max: 8},
			},
		}},
	})
	if err != nil { t.Fatal(err) }
	oldCatalog := defaultEquipmentCatalog
	defaultEquipmentCatalog = catalog
	t.Cleanup(func() { defaultEquipmentCatalog = oldCatalog })
	return itemID
}

func TestDurableInventoryRestoresSameUniqueAffixInstance(t *testing.T) {
	itemID := withMidTierRestoreCatalog(t)
	instance := iteminstance.Instance{
		ID: "item-instance:restore-mid-1",
		ItemArchetypeID: itemID,
		Affixes: []equipmentaffix.Affix{{
			ID: equipmentaffix.AffixPhysicalDamage, Strength: 2, Value: 2,
		}},
	}
	definition, ok := defaultEquipmentCatalog.Resolve(itemID)
	if !ok { t.Fatal("test item missing") }
	if err := iteminstance.Validate(instance, definition); err != nil { t.Fatal(err) }

	inv := newCharacterInventory(16)
	if err := inv.AddInstance(instance); err != nil { t.Fatal(err) }
	if err := inv.EquipMainHandInstance(instance.ID); err != nil { t.Fatal(err) }

	durable, err := durableInventoryState(inv)
	if err != nil { t.Fatal(err) }
	if !durable.HasItemInstances() || durable.MainHand != "" || durable.MainHandInstanceJSON == "" {
		t.Fatalf("durable state lost unique main hand: %#v", durable)
	}

	restored, err := restoreCharacterInventory(16, durable)
	if err != nil { t.Fatal(err) }
	got, ok := restored.MainHandInstance()
	if !ok { t.Fatal("restored unique main hand missing") }
	if got.ID != instance.ID || got.ItemArchetypeID != instance.ItemArchetypeID || len(got.Affixes) != 1 || got.Affixes[0] != instance.Affixes[0] {
		t.Fatalf("restored instance changed: got=%#v want=%#v", got, instance)
	}
}

func TestLegacyArchetypePersistenceRejectsMidTierEquipment(t *testing.T) {
	itemID := withMidTierRestoreCatalog(t)
	legacy, err := characterstate.NewInventoryStateWithEquipment(nil, itemID, "")
	if err != nil { t.Fatal(err) }
	if _, err := restoreCharacterInventory(16, legacy); !errors.Is(err, ErrEquipmentItemNotAllowed) {
		t.Fatalf("legacy mid-tier restore err=%v", err)
	}

	inv := newCharacterInventory(16)
	if err := inv.Add(itemID, 1); err != nil { t.Fatal(err) }
	if err := inv.EquipMainHand(itemID); err != nil { t.Fatal(err) }
	if _, err := durableInventoryState(inv); !errors.Is(err, ErrEquipmentItemNotAllowed) {
		t.Fatalf("legacy mid-tier save err=%v", err)
	}
}
