package worldruntime

import (
	"errors"
	"testing"

	"github.com/li41/astrahold-server/internal/equipmentaffix"
	"github.com/li41/astrahold-server/internal/equipmentcatalog"
	"github.com/li41/astrahold-server/internal/iteminstance"
	"github.com/li41/astrahold-server/internal/protocol"
)

const (
	testLowTwoHandWeapon = "item_test_low_two_hand_sword"
	testLowOneHandWeapon = "item_test_low_one_hand_sword"
	testLowShield        = "item_test_low_shield"
	testMidTwoHandWeapon = "item_test_mid_two_hand_sword"
	testMidShield        = "item_test_mid_shield"
)

func withEquipmentHandTestCatalog(t *testing.T) {
	t.Helper()
	catalog, err := equipmentcatalog.New(equipmentcatalog.CatalogDefinition{
		Revision: "equipment-hand-test",
		WeaponTypes: []equipmentcatalog.WeaponTypeDefinition{
			{WeaponType: equipmentcatalog.WeaponTypeOneHandSword},
			{WeaponType: equipmentcatalog.WeaponTypeTwoHandSword},
		},
		Items: []equipmentcatalog.Definition{
			{ItemArchetypeID: testLowTwoHandWeapon, Kind: equipmentcatalog.KindWeapon, Slot: equipmentcatalog.SlotMainHand, Tier: equipmentcatalog.TierLow, Weight: 1, Material: "iron", Weapon: &equipmentcatalog.Weapon{WeaponType: equipmentcatalog.WeaponTypeTwoHandSword, SmallDamage: equipmentcatalog.DamageRange{Min: 1, Max: 1}, LargeDamage: equipmentcatalog.DamageRange{Min: 1, Max: 1}}},
			{ItemArchetypeID: testLowOneHandWeapon, Kind: equipmentcatalog.KindWeapon, Slot: equipmentcatalog.SlotMainHand, Tier: equipmentcatalog.TierLow, Weight: 1, Material: "iron", Weapon: &equipmentcatalog.Weapon{WeaponType: equipmentcatalog.WeaponTypeOneHandSword, SmallDamage: equipmentcatalog.DamageRange{Min: 1, Max: 1}, LargeDamage: equipmentcatalog.DamageRange{Min: 1, Max: 1}}},
			{ItemArchetypeID: testLowShield, Kind: equipmentcatalog.KindShield, Slot: equipmentcatalog.SlotOffHand, Tier: equipmentcatalog.TierLow, Weight: 1, Material: "wood", Shield: &equipmentcatalog.Shield{PhysicalDefense: 1}},
			{ItemArchetypeID: testMidTwoHandWeapon, Kind: equipmentcatalog.KindWeapon, Slot: equipmentcatalog.SlotMainHand, Tier: equipmentcatalog.TierMid, Weight: 1, Material: "steel", Weapon: &equipmentcatalog.Weapon{WeaponType: equipmentcatalog.WeaponTypeTwoHandSword, SmallDamage: equipmentcatalog.DamageRange{Min: 2, Max: 2}, LargeDamage: equipmentcatalog.DamageRange{Min: 2, Max: 2}}},
			{ItemArchetypeID: testMidShield, Kind: equipmentcatalog.KindShield, Slot: equipmentcatalog.SlotOffHand, Tier: equipmentcatalog.TierMid, Weight: 1, Material: "steel", Shield: &equipmentcatalog.Shield{PhysicalDefense: 2}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	oldCatalog := defaultEquipmentCatalog
	defaultEquipmentCatalog = catalog
	t.Cleanup(func() { defaultEquipmentCatalog = oldCatalog })
}

func testHandInstance(id iteminstance.ID, itemArchetypeID string) iteminstance.Instance {
	return iteminstance.Instance{
		ID: id,
		ItemArchetypeID: itemArchetypeID,
		Affixes: []equipmentaffix.Affix{{ID: equipmentaffix.AffixStrength, Strength: 1, Value: 1}},
	}
}

func TestArchetypeEquipRejectsTwoHandShieldConflictWithoutAutoUnequip(t *testing.T) {
	withEquipmentHandTestCatalog(t)
	inv := newCharacterInventory(16)
	if err := inv.Add(testLowShield, 1); err != nil { t.Fatal(err) }
	if err := applyEquipArchetype(inv, protocol.EquipmentSlotOffHand, testLowShield); err != nil { t.Fatal(err) }
	if err := inv.Add(testLowTwoHandWeapon, 1); err != nil { t.Fatal(err) }
	if err := applyEquipArchetype(inv, protocol.EquipmentSlotMainHand, testLowTwoHandWeapon); !errors.Is(err, ErrEquipmentHandConflict) {
		t.Fatalf("two-hand with shield err=%v", err)
	}
	if inv.MainHand() != "" || inv.OffHand() != testLowShield {
		t.Fatalf("conflict mutated equipment: main=%q off=%q", inv.MainHand(), inv.OffHand())
	}
	if _, err := inv.UnequipOffHand(); err != nil { t.Fatal(err) }
	if err := applyEquipArchetype(inv, protocol.EquipmentSlotMainHand, testLowTwoHandWeapon); err != nil { t.Fatal(err) }
	if err := applyEquipArchetype(inv, protocol.EquipmentSlotOffHand, testLowShield); !errors.Is(err, ErrEquipmentHandConflict) {
		t.Fatalf("shield with two-hand err=%v", err)
	}
	if inv.MainHand() != testLowTwoHandWeapon || inv.OffHand() != "" {
		t.Fatalf("reverse conflict mutated equipment: main=%q off=%q", inv.MainHand(), inv.OffHand())
	}
}

func TestOneHandWeaponStillAllowsShield(t *testing.T) {
	withEquipmentHandTestCatalog(t)
	inv := newCharacterInventory(16)
	if err := inv.Add(testLowOneHandWeapon, 1); err != nil { t.Fatal(err) }
	if err := inv.Add(testLowShield, 1); err != nil { t.Fatal(err) }
	if err := applyEquipArchetype(inv, protocol.EquipmentSlotMainHand, testLowOneHandWeapon); err != nil { t.Fatal(err) }
	if err := applyEquipArchetype(inv, protocol.EquipmentSlotOffHand, testLowShield); err != nil { t.Fatal(err) }
	if inv.MainHand() != testLowOneHandWeapon || inv.OffHand() != testLowShield {
		t.Fatalf("one-hand + shield not preserved: main=%q off=%q", inv.MainHand(), inv.OffHand())
	}
}

func TestUniqueInstanceEquipRejectsTwoHandShieldConflict(t *testing.T) {
	withEquipmentHandTestCatalog(t)
	runtime := &Runtime{}

	inv := newCharacterInventory(16)
	if err := inv.Add(testLowShield, 1); err != nil { t.Fatal(err) }
	if err := applyEquipArchetype(inv, protocol.EquipmentSlotOffHand, testLowShield); err != nil { t.Fatal(err) }
	weapon := testHandInstance("item-instance:test-two-hand", testMidTwoHandWeapon)
	if err := inv.AddInstance(weapon); err != nil { t.Fatal(err) }
	if err := runtime.applyEquipInstance(inv, protocol.EquipmentSlotMainHand, weapon.ID); !errors.Is(err, ErrEquipmentHandConflict) {
		t.Fatalf("unique two-hand with shield err=%v", err)
	}
	if _, ok := inv.MainHandInstance(); ok || inv.OffHand() != testLowShield {
		t.Fatalf("unique conflict mutated equipment")
	}

	inv = newCharacterInventory(16)
	if err := inv.Add(testLowTwoHandWeapon, 1); err != nil { t.Fatal(err) }
	if err := applyEquipArchetype(inv, protocol.EquipmentSlotMainHand, testLowTwoHandWeapon); err != nil { t.Fatal(err) }
	shield := testHandInstance("item-instance:test-shield", testMidShield)
	if err := inv.AddInstance(shield); err != nil { t.Fatal(err) }
	if err := runtime.applyEquipInstance(inv, protocol.EquipmentSlotOffHand, shield.ID); !errors.Is(err, ErrEquipmentHandConflict) {
		t.Fatalf("unique shield with two-hand err=%v", err)
	}
	if _, ok := inv.OffHandInstance(); ok || inv.MainHand() != testLowTwoHandWeapon {
		t.Fatalf("unique reverse conflict mutated equipment")
	}
}
