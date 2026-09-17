// Package ammunition owns Server-authoritative ammunition gameplay data.
// Client names, icons, meshes and projectile visuals stay outside the Server.
package ammunition

import (
	"strings"

	"github.com/li41/astrahold-server/internal/equipmentcatalog"
)

const (
	// ItemWoodArrow keeps the existing stable item identity used by the first playable bow loop.
	ItemWoodArrow = "item_low_arrow"
	ItemSilverArrow = "item_silver_arrow"

	// ItemLowArrow is retained as a source-compatible alias for the existing wooden-arrow ID.
	// New Server code should use ItemWoodArrow so ammunition is not modeled as a tier ladder.
	ItemLowArrow = ItemWoodArrow
)

type Definition struct {
	ItemArchetypeID string
	Material        equipmentcatalog.MaterialID
	Damage          equipmentcatalog.DamageRange
	// UnitWeight is authoritative inventory weight per arrow. V1 arrows deliberately use zero
	// because the current carry-weight scale is integer-granularity and would otherwise make
	// one arrow weigh as much as one full generic inventory unit.
	UnitWeight uint32
}

// V1 has exactly two production arrow identities. Wood is consumed before silver when both are
// present so ordinary ammunition is never silently replaced by the silver stack.
//
// Both arrows currently contribute the same base arrow damage. The existing silver-main-hand
// versus undead rule is not inferred from ammunition material and remains a separate contract.
var v1Definitions = []Definition{
	{ItemArchetypeID: ItemWoodArrow, Material: equipmentcatalog.MaterialWood, Damage: equipmentcatalog.DamageRange{Min: 6, Max: 8}},
	{ItemArchetypeID: ItemSilverArrow, Material: equipmentcatalog.MaterialSilver, Damage: equipmentcatalog.DamageRange{Min: 6, Max: 8}},
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

// Definitions returns the deterministic V1 consumption priority: wood, then silver.
func Definitions() []Definition {
	out := make([]Definition, len(v1Definitions))
	copy(out, v1Definitions)
	return out
}

func Resolve(itemArchetypeID string) (Definition, bool) {
	definition, ok := v1ByItem[strings.TrimSpace(itemArchetypeID)]
	return definition, ok
}
