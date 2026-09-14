package characterstats

// CombineAdditive combines independently owned additive stat bonuses without allowing uint32
// wraparound to become authoritative gameplay truth.
func CombineAdditive(left, right AdditiveBonus) (AdditiveBonus, error) {
	strength, ok := add(left.Strength, right.Strength)
	if !ok {
		return AdditiveBonus{}, ErrOverflow
	}
	agility, ok := add(left.Agility, right.Agility)
	if !ok {
		return AdditiveBonus{}, ErrOverflow
	}
	return AdditiveBonus{Strength: strength, Agility: agility}, nil
}