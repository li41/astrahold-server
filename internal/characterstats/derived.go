package characterstats

const (
	physicalHitBaseBasisPoints uint32 = 9000
	physicalHitMinBasisPoints  uint32 = 7500
	physicalHitMaxBasisPoints  uint32 = 9800
	criticalBaseBasisPoints    uint32 = 500
	criticalMaxBasisPoints     uint32 = 3000
)

// MeleePhysicalDamageBonus = floor(max(Strength - 10, 0) / 2).
func MeleePhysicalDamageBonus(strength uint32) uint32 {
	if strength <= 10 { return 0 }
	return (strength - 10) / 2
}

// PhysicalHitModifier is the Agility-sourced physical hit rating: Agility - 10.
func PhysicalHitModifier(agility uint32) int64 {
	return int64(agility) - 10
}

// RangedPhysicalDamageBonus = floor(max(Agility - 9, 0) / 3).
func RangedPhysicalDamageBonus(agility uint32) uint32 {
	if agility <= 9 { return 0 }
	return (agility - 9) / 3
}

// EvasionModifier = floor(max(Agility - 10, 0) / 2).
func EvasionModifier(agility uint32) uint32 {
	if agility <= 10 { return 0 }
	return (agility - 10) / 2
}

// AttributeCriticalBonusPercentPoints gives +1 percentage point per 10 Agility starting at 40.
func AttributeCriticalBonusPercentPoints(agility uint32) uint32 {
	if agility <= 30 { return 0 }
	return (agility - 30) / 10
}

// MagicPowerBonus = floor(max(Intelligence - 10, 0) / 5).
func MagicPowerBonus(intelligence uint32) uint32 {
	if intelligence <= 10 { return 0 }
	return (intelligence - 10) / 5
}

// ConstitutionMaxHPBonus = level * max(Constitution - 15, 0).
func ConstitutionMaxHPBonus(level, constitution uint32) uint64 {
	if constitution <= 15 || level == 0 { return 0 }
	return uint64(level) * uint64(constitution-15)
}

// SpiritGrowthTier implements the formal V1 Spirit table used by maximum MP growth.
func SpiritGrowthTier(spirit uint32) uint32 {
	switch {
	case spirit <= 9:
		return 0
	case spirit <= 14:
		return 1
	case spirit <= 20:
		return 2
	case spirit <= 24:
		return 3
	case spirit <= 28:
		return 4
	case spirit <= 32:
		return 5
	default:
		return 6
	}
}

// SpiritMaxMPBonus = level * SpiritGrowthTier(Spirit).
func SpiritMaxMPBonus(level, spirit uint32) uint64 {
	return uint64(level) * uint64(SpiritGrowthTier(spirit))
}

// LeadershipCount is the shared summon/pet/lord-guard cap. V1 hard caps it at five.
func LeadershipCount(charisma uint32) uint32 {
	if charisma <= 10 { return 1 }
	count := uint32(1) + (charisma-10)/5
	if count > 5 { return 5 }
	return count
}

// PhysicalHitChanceBasisPoints implements:
// clamp(90% + (totalPhysicalHit - totalEvasion) * 0.5 percentage points, 75%, 98%).
// Basis points avoid floating-point gameplay truth: 10000 = 100%.
func PhysicalHitChanceBasisPoints(totalPhysicalHit, totalEvasion int64) uint32 {
	net := totalPhysicalHit - totalEvasion
	if net >= 16 { return physicalHitMaxBasisPoints }
	if net <= -30 { return physicalHitMinBasisPoints }
	return uint32(int64(physicalHitBaseBasisPoints) + net*50)
}

// CriticalChanceBasisPoints implements:
// min(30%, 5% + attributeCriticalBonus + criticalRating * 0.5 percentage points).
func CriticalChanceBasisPoints(agility, criticalRating uint32) uint32 {
	attributePP := AttributeCriticalBonusPercentPoints(agility)
	if attributePP >= 25 || criticalRating >= 50 {
		return criticalMaxBasisPoints
	}
	chance := criticalBaseBasisPoints + attributePP*100 + criticalRating*50
	if chance > criticalMaxBasisPoints { return criticalMaxBasisPoints }
	return chance
}