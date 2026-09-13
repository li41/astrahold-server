package combat

const (
	basePhysicalHitBasisPoints uint32 = 9000
	minPhysicalHitBasisPoints  uint32 = 7500
	maxPhysicalHitBasisPoints  uint32 = 9800
	baseCriticalBasisPoints    uint32 = 500
	maxCriticalBasisPoints     uint32 = 3000
	basisPointsPerRating       int64  = 50
)

// PhysicalHitChanceBasisPoints applies the formal V1 rating rule:
// 90% + (attacker physical hit - target evasion) * 0.5 percentage points,
// clamped to 75%..98%. One basis point is 0.01 percentage point.
func PhysicalHitChanceBasisPoints(attackerPhysicalHit, targetEvasion int32) uint32 {
	chance := int64(basePhysicalHitBasisPoints) + (int64(attackerPhysicalHit)-int64(targetEvasion))*basisPointsPerRating
	if chance < int64(minPhysicalHitBasisPoints) {
		return minPhysicalHitBasisPoints
	}
	if chance > int64(maxPhysicalHitBasisPoints) {
		return maxPhysicalHitBasisPoints
	}
	return uint32(chance)
}

// CriticalChanceBasisPoints applies the formal V1 rule:
// 5% base + attribute critical bonus + critical rating * 0.5 percentage points,
// capped at 30%. attributeBonusBasisPoints is already a percentage-point-derived value.
func CriticalChanceBasisPoints(attributeBonusBasisPoints uint32, criticalRating uint32) uint32 {
	chance := uint64(baseCriticalBasisPoints) + uint64(attributeBonusBasisPoints) + uint64(criticalRating)*uint64(basisPointsPerRating)
	if chance > uint64(maxCriticalBasisPoints) {
		return maxCriticalBasisPoints
	}
	return uint32(chance)
}

// DefenseMitigationBasisPoints applies both formal V1 defense curves:
// defense / (defense + 20). The caller decides whether defense is physical or magic.
func DefenseMitigationBasisPoints(defense uint32) uint32 {
	if defense == 0 {
		return 0
	}
	return uint32((uint64(defense) * 10000) / uint64(defense+20))
}

// ApplyMitigationBasisPoints applies a reduction using integer basis points without allowing
// a percentage value greater than 100%. Rounding remains a caller-level final damage concern.
func ApplyMitigationBasisPoints(amount uint32, mitigationBasisPoints uint32) uint32 {
	if amount == 0 {
		return 0
	}
	if mitigationBasisPoints >= 10000 {
		return 0
	}
	return uint32((uint64(amount) * uint64(10000-mitigationBasisPoints)) / 10000)
}

// ApplyCriticalMultiplierV1 applies the fixed first-version 1.5x critical multiplier.
// Integer arithmetic deliberately truncates; the full damage pipeline must still perform its
// single final rounding policy when the runtime integration replaces legacy paths.
func ApplyCriticalMultiplierV1(amount uint32) uint32 {
	return uint32((uint64(amount) * 3) / 2)
}
