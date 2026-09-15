package equipmentcatalog

import "strings"

var defaultBowBaseDamageByItem = map[string]DamageRange{
	"item_hunter_shortbow": {Min: 2, Max: 3},
	"item_mid_bow":         {Min: 3, Max: 4},
	"item_high_bow":        {Min: 4, Max: 5},
}

// applyDefaultBowAmmunitionBalance moves most bow base damage into ammunition while preserving the
// current total bow+matching-arrow progression. Only the three production bow archetypes are
// rewritten; crossbows, slings and all other weapons keep their authored damage unchanged.
func applyDefaultBowAmmunitionBalance(items []Definition) error {
	seen := make(map[string]struct{}, len(defaultBowBaseDamageByItem))
	for i := range items {
		id := strings.TrimSpace(items[i].ItemArchetypeID)
		damage, ok := defaultBowBaseDamageByItem[id]
		if !ok {
			continue
		}
		if items[i].Kind != KindWeapon || items[i].Weapon == nil || items[i].Weapon.WeaponType != WeaponType("bow") {
			return ErrInvalidCatalog
		}
		items[i].Weapon.SmallDamage = damage
		items[i].Weapon.LargeDamage = damage
		seen[id] = struct{}{}
	}
	if len(seen) != len(defaultBowBaseDamageByItem) {
		return ErrInvalidCatalog
	}
	return nil
}
