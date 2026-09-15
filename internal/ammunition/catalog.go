// Package ammunition owns Server-authoritative ammunition gameplay data.
// Client names, icons, meshes and projectile visuals stay outside the Server.
package ammunition

import (
	"strings"

	"github.com/li41/astrahold-server/internal/equipmentcatalog"
)

const (
	ItemLowArrow  = "item_low_arrow"
	ItemMidArrow  = "item_mid_arrow"
	ItemHighArrow = "item_high_arrow"
)

type Definition struct {
	ItemArchetypeID string
	Tier            equipmentcatalog.Tier
	Material        equipmentcatalog.MaterialID
	Damage          equipmentcatalog.DamageRange
	// UnitWeight is authoritative inventory weight per arrow. V1 arrows deliberately use zero
	// because the current carry-weight scale is integer-granularity and would otherwise make
	// one arrow weigh as much as one full generic inventory unit.
	UnitWeight uint32
}

var v1Definitions = []Definition{
	{ItemArchetypeID: ItemLowArrow, Tier: equipmentcatalog.TierLow, Material: equipmentcatalog.MaterialWood, Damage: equipmentcatalog.DamageRange{Min: 6, Max: 8}},
	{ItemArchetypeID: ItemMidArrow, Tier: equipmentcatalog.TierMid, Material: equipmentcatalog.MaterialSteel, Damage: equipmentcatalog.DamageRange{Min: 8, Max: 11}},
	{ItemArchetypeID: ItemHighArrow, Tier: equipmentcatalog.TierHigh, Material: equipmentcatalog.MaterialStarsteel, Damage: equipmentcatalog.DamageRange{Min: 10, Max: 15}},
}

var v1ByItem = mustV1ByItem()

func mustV1ByItem() map[string]Definition {
	byItem := make(map[string]Definition, len(v1Definitions))
	for _, definition := range v1Definitions {
		definition.ItemArchetypeID = strings.TrimSpace(definition.ItemArchetypeID)
		if definition.ItemArchetypeID == "" || definition.Damage.Min == 0 || definition.Damage.Max < definition.Damage.Min || !definition.Material.Valid() {
			panic("ammunition: invalid V1 definition")
		}
		if _, exists := byItem[definition.ItemArchetypeID]; exists {
			panic("ammunition: duplicate V1 item archetype")
		}
		byItem[definition.ItemArchetypeID] = definition
	}
	return byItem
}

// Definitions returns the deterministic V1 arrow priority. The Server consumes lower-tier arrows
// first so carrying ordinary arrows does not silently burn rarer ammunition.
func Definitions() []Definition {
	out := make([]Definition, len(v1Definitions))
	copy(out, v1Definitions)
	return out
}

func Resolve(itemArchetypeID string) (Definition, bool) {
	definition, ok := v1ByItem[strings.TrimSpace(itemArchetypeID)]
	return definition, ok
}
