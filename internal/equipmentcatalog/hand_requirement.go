package equipmentcatalog

// HandRequirement is Server-authoritative gameplay occupancy for equipment hands. It does not
// prescribe Client meshes, animation rigs or presentation.
type HandRequirement string

const (
	HandRequirementOneHand HandRequirement = "one_hand"
	HandRequirementTwoHand HandRequirement = "two_hand"
)

const (
	WeaponTypeDagger       WeaponType = "dagger"
	WeaponTypeOneHandSpear WeaponType = "one_hand_spear"
	WeaponTypeWarhammer    WeaponType = "warhammer"
	WeaponTypeMorningStar  WeaponType = "morning_star"
	WeaponTypeTwoHandSword WeaponType = "two_hand_sword"
	WeaponTypeTwoHandAxe   WeaponType = "two_hand_axe"
	WeaponTypeTwoHandSpear WeaponType = "two_hand_spear"
	WeaponTypeKnuckles     WeaponType = "knuckles"
	WeaponTypeClaw         WeaponType = "claw"
	WeaponTypeDualBlades   WeaponType = "dual_blades"
	WeaponTypeCrossbow     WeaponType = "crossbow"
	WeaponTypeSling        WeaponType = "sling"
	WeaponTypeStaff        WeaponType = "staff"
)

// HandRequirementForWeaponType keeps hand occupancy at WeaponType level so all tiers and item
// archetypes of the same weapon type share one authoritative rule.
func HandRequirementForWeaponType(weaponType WeaponType) (HandRequirement, bool) {
	switch weaponType {
	case WeaponTypeOneHandSword,
		WeaponTypeDagger,
		WeaponTypeOneHandAxe,
		WeaponTypeOneHandSpear,
		WeaponTypeWarhammer,
		WeaponTypeMorningStar,
		WeaponTypeMace,
		WeaponTypeSling:
		return HandRequirementOneHand, true
	case WeaponTypeTwoHandSword,
		WeaponTypeTwoHandAxe,
		WeaponTypeTwoHandSpear,
		WeaponTypeKnuckles,
		WeaponTypeClaw,
		WeaponTypeDualBlades,
		WeaponTypeBow,
		WeaponTypeCrossbow,
		WeaponTypeStaff:
		return HandRequirementTwoHand, true
	default:
		return "", false
	}
}

func (c *Catalog) HandRequirementForItem(itemArchetypeID string) (HandRequirement, bool) {
	definition, ok := c.Resolve(itemArchetypeID)
	if !ok || definition.Kind != KindWeapon || definition.Weapon == nil {
		return "", false
	}
	return HandRequirementForWeaponType(definition.Weapon.WeaponType)
}
