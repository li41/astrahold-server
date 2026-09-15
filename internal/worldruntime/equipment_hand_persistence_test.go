package worldruntime

import (
	"errors"
	"testing"

	"github.com/li41/astrahold-server/internal/characterstate"
)

func TestDurableInventoryRejectsArchetypeTwoHandShieldConflict(t *testing.T) {
	withEquipmentHandTestCatalog(t)
	inv := newCharacterInventory(16)
	if err := inv.Add(testLowTwoHandWeapon, 1); err != nil { t.Fatal(err) }
	if err := inv.EquipMainHand(testLowTwoHandWeapon); err != nil { t.Fatal(err) }
	if err := inv.Add(testLowShield, 1); err != nil { t.Fatal(err) }
	if err := inv.EquipOffHand(testLowShield); err != nil { t.Fatal(err) }
	if _, err := durableInventoryState(inv); !errors.Is(err, ErrEquipmentHandConflict) {
		t.Fatalf("illegal archetype save err=%v", err)
	}

	state, err := characterstate.NewInventoryStateWithEquipment(nil, testLowTwoHandWeapon, testLowShield)
	if err != nil { t.Fatal(err) }
	if _, err := restoreCharacterInventory(16, state); !errors.Is(err, ErrEquipmentHandConflict) {
		t.Fatalf("illegal archetype restore err=%v", err)
	}
}

func TestDurableInventoryRejectsUniqueTwoHandShieldConflict(t *testing.T) {
	withEquipmentHandTestCatalog(t)
	weapon := testHandInstance("item-instance:persist-two-hand", testMidTwoHandWeapon)

	inv := newCharacterInventory(16)
	if err := inv.AddInstance(weapon); err != nil { t.Fatal(err) }
	if err := inv.EquipMainHandInstance(weapon.ID); err != nil { t.Fatal(err) }
	if err := inv.Add(testLowShield, 1); err != nil { t.Fatal(err) }
	if err := inv.EquipOffHand(testLowShield); err != nil { t.Fatal(err) }
	if _, err := durableInventoryState(inv); !errors.Is(err, ErrEquipmentHandConflict) {
		t.Fatalf("illegal unique save err=%v", err)
	}

	state, err := characterstate.NewInventoryStateWithInstances(nil, nil, "", testLowShield, &weapon, nil)
	if err != nil { t.Fatal(err) }
	if _, err := restoreCharacterInventory(16, state); !errors.Is(err, ErrEquipmentHandConflict) {
		t.Fatalf("illegal unique restore err=%v", err)
	}
}
