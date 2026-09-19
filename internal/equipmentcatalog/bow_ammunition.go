package equipmentcatalog

import "strings"

type rangedAmmunitionBalance struct {
	WeaponType WeaponType
	BaseDamage DamageRange
}

var defaultRangedAmmunitionBalanceByItem = map[string]rangedAmmunitionBalance{
	"item_hunter_shortbow":       {WeaponType: WeaponTypeBow, BaseDamage: DamageRange{Min: 2, Max: 3}},
	"item_mid_bow":               {WeaponType: WeaponTypeBow, BaseDamage: DamageRange{Min: 3, Max: 4}},
	"item_high_bow":              {WeaponType: WeaponTypeBow, BaseDamage: DamageRange{Min: 4, Max: 5}},
	"item_hunter_light_crossbow": {WeaponType: WeaponTypeCrossbow, BaseDamage: DamageRange{Min: 6, Max: 7}},
	"item_mid_crossbow":          {WeaponType: WeaponTypeCrossbow, BaseDamage: DamageRange{Min: 11, Max: 13}},
	"item_high_crossbow":         {WeaponType: WeaponTypeCrossbow, BaseDamage: DamageRange{Min: 16, Max: 19}},
}

// applyDefaultRangedAmmunitionBalance moves the approved arrow's fixed 6-8 contribution out of
// bow/crossbow weapon base damage while preserving each weapon's authored total progression.
// Only the six production bow/crossbow archetypes are rewritten; slings and all other weapons keep
// their authored damage unchanged.
func applyDefaultRangedAmmunitionBalance(items []Definition) error {
	seen := make(map[string]struct{}, len(defaultRangedAmmunitionBalanceByItem))
	for i := range items {
		id := strings.TrimSpace(items[i].ItemArchetypeID)
		balance, ok := defaultRangedAmmunitionBalanceByItem[id]
		if !ok {
			continue
		}
		if items[i].Kind != KindWeapon || items[i].Weapon == nil || items[i].Weapon.WeaponType != balance.WeaponType {
			return ErrInvalidCatalog
		}
		items[i].Weapon.SmallDamage = balance.BaseDamage
		items[i].Weapon.LargeDamage = balance.BaseDamage
		seen[id] = struct{}{}
	}
	if len(seen) != len(defaultRangedAmmunitionBalanceByItem) {
		return ErrInvalidCatalog
	}
	return nil
}
