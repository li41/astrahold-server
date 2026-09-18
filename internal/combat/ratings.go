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
// defense / (defense + 100). The caller decides whether defense is physical or magic.
func DefenseMitigationBasisPoints(defense uint32) uint32 {
	if defense == 0 {
		return 0
	}
	return uint32((uint64(defense) * 10000) / (uint64(defense) + 100))
}

// RemainingBasisPointsAfterMitigation converts one mitigation layer into its remaining-damage
// multiplier. 8000 means 80% of the incoming amount remains.
func RemainingBasisPointsAfterMitigation(mitigationBasisPoints uint32) uint32 {
	if mitigationBasisPoints >= 10000 {
		return 0
	}
	return 10000 - mitigationBasisPoints
}

// CombineRemainingBasisPoints multiplies independent remaining-damage layers while retaining
// basis-point precision for the caller. This supports the formal rule that defense and shield
// percentage reduction are separate multiplicative layers, without rounding the damage between
// those layers.
func CombineRemainingBasisPoints(first, second uint32) uint32 {
	if first > 10000 {
		first = 10000
	}
	if second > 10000 {
		second = 10000
	}
	return uint32((uint64(first) * uint64(second)) / 10000)
}

const (
	CriticalMultiplierV1Numerator   uint32 = 3
	CriticalMultiplierV1Denominator uint32 = 2
)
