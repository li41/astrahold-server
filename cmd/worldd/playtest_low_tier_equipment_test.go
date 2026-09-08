package main

import (
	"reflect"
	"strings"
	"testing"

	"github.com/li41/astrahold-server/internal/inventory"
	"github.com/li41/astrahold-server/internal/worldruntime"
)

func TestConfigurePlaytestLowTierEquipmentDisabledLeavesStarterInventoryUnchanged(t *testing.T) {
	config := worldruntime.DefaultConfig()
	before := append([]inventory.Stack(nil), config.StarterInventory...)

	if err := configurePlaytestLowTierEquipment(&config, worldNetworkTCPUDP, false); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(config.StarterInventory, before) {
		t.Fatalf("starter inventory changed while disabled: got=%#v want=%#v", config.StarterInventory, before)
	}
}

func TestConfigurePlaytestLowTierEquipmentRejectsTCPUDP(t *testing.T) {
	config := worldruntime.DefaultConfig()
	before := append([]inventory.Stack(nil), config.StarterInventory...)

	err := configurePlaytestLowTierEquipment(&config, worldNetworkTCPUDP, true)
	if err == nil || !strings.Contains(err.Error(), worldNetworkBrowserWSDev) {
		t.Fatalf("error=%v, want browserws-dev rejection", err)
	}
	if !reflect.DeepEqual(config.StarterInventory, before) {
		t.Fatalf("rejected configuration mutated starter inventory: got=%#v want=%#v", config.StarterInventory, before)
	}
}

func TestConfigurePlaytestLowTierEquipmentAddsEightUniqueBrowserWSStacks(t *testing.T) {
	config := worldruntime.DefaultConfig()
	baseCount := len(config.StarterInventory)

	if err := configurePlaytestLowTierEquipment(&config, worldNetworkBrowserWSDev, true); err != nil {
		t.Fatal(err)
	}
	if got, want := len(config.StarterInventory), baseCount+8; got != want {
		t.Fatalf("starter stack count=%d want=%d", got, want)
	}

	quantities := make(map[string]uint32, len(config.StarterInventory))
	for _, stack := range config.StarterInventory {
		if _, duplicate := quantities[stack.ArchetypeID]; duplicate {
			t.Fatalf("duplicate starter item %q", stack.ArchetypeID)
		}
		quantities[stack.ArchetypeID] = stack.Quantity
	}
	for _, stack := range playtestLowTierEquipmentStarterStacks {
		if got := quantities[stack.ArchetypeID]; got != 1 {
			t.Fatalf("playtest item %q quantity=%d want=1", stack.ArchetypeID, got)
		}
	}
}

func TestConfigurePlaytestLowTierEquipmentRejectsStarterCollision(t *testing.T) {
	config := worldruntime.DefaultConfig()
	config.StarterInventory = append(config.StarterInventory, inventory.Stack{
		ArchetypeID: playtestLowTierEquipmentStarterStacks[0].ArchetypeID,
		Quantity:    1,
	})
	before := append([]inventory.Stack(nil), config.StarterInventory...)

	if err := configurePlaytestLowTierEquipment(&config, worldNetworkBrowserWSDev, true); err == nil {
		t.Fatal("expected starter collision rejection")
	}
	if !reflect.DeepEqual(config.StarterInventory, before) {
		t.Fatalf("collision rejection mutated starter inventory: got=%#v want=%#v", config.StarterInventory, before)
	}
}
