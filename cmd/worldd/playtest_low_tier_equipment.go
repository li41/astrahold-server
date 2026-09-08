package main

import (
	"errors"
	"flag"
	"fmt"

	"github.com/li41/astrahold-server/internal/inventory"
	"github.com/li41/astrahold-server/internal/worldruntime"
)

var playtestLowTierEquipment = flag.Bool(
	"playtest-low-tier-equipment",
	false,
	"Grant the low-tier equipment v1 kit to fresh browserws-dev playtest characters only",
)

var playtestLowTierEquipmentStarterStacks = []inventory.Stack{
	{ArchetypeID: "item_militia_iron_sword", Quantity: 1},
	{ArchetypeID: "item_light_guard_sword", Quantity: 1},
	{ArchetypeID: "item_gladiator_iron_sword", Quantity: 1},
	{ArchetypeID: "item_militia_battle_axe", Quantity: 1},
	{ArchetypeID: "item_iron_war_mace", Quantity: 1},
	{ArchetypeID: "item_iron_rim_round_shield", Quantity: 1},
	{ArchetypeID: "item_guard_shield", Quantity: 1},
	{ArchetypeID: "item_runed_square_shield", Quantity: 1},
}

// configurePlaytestLowTierEquipment augments only fresh-character bootstrap inventory for the
// loopback BrowserWS development adapter. It is deliberately not a production acquisition path:
// durable initialized inventories still restore their own Server truth, and tcpudp is rejected.
func configurePlaytestLowTierEquipment(config *worldruntime.Config, networkMode string, enabled bool) error {
	if !enabled {
		return nil
	}
	if config == nil {
		return errors.New("playtest low-tier equipment requires runtime config")
	}
	if networkMode != worldNetworkBrowserWSDev {
		return fmt.Errorf("playtest-low-tier-equipment requires network-mode %q", worldNetworkBrowserWSDev)
	}

	starter := append([]inventory.Stack(nil), config.StarterInventory...)
	seen := make(map[string]struct{}, len(starter)+len(playtestLowTierEquipmentStarterStacks))
	for _, stack := range starter {
		seen[stack.ArchetypeID] = struct{}{}
	}
	for _, stack := range playtestLowTierEquipmentStarterStacks {
		if _, exists := seen[stack.ArchetypeID]; exists {
			return fmt.Errorf("playtest low-tier equipment duplicates starter item %q", stack.ArchetypeID)
		}
		seen[stack.ArchetypeID] = struct{}{}
		starter = append(starter, stack)
	}
	config.StarterInventory = starter
	return nil
}
