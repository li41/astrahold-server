package characterstats

// CombineAdditive combines independently owned positive additive stat bonuses without allowing
// uint32 wraparound to become authoritative gameplay truth.
func CombineAdditive(left, right AdditiveBonus) (AdditiveBonus, error) {
	strength, ok := add(left.Strength, right.Strength)
	if !ok { return AdditiveBonus{}, ErrOverflow }
	agility, ok := add(left.Agility, right.Agility)
	if !ok { return AdditiveBonus{}, ErrOverflow }
	constitution, ok := add(left.Constitution, right.Constitution)
	if !ok { return AdditiveBonus{}, ErrOverflow }
	intelligence, ok := add(left.Intelligence, right.Intelligence)
	if !ok { return AdditiveBonus{}, ErrOverflow }
	spirit, ok := add(left.Spirit, right.Spirit)
	if !ok { return AdditiveBonus{}, ErrOverflow }
	charisma, ok := add(left.Charisma, right.Charisma)
	if !ok { return AdditiveBonus{}, ErrOverflow }
	return AdditiveBonus{
		Strength: strength,
		Agility: agility,
		Constitution: constitution,
		Intelligence: intelligence,
		Spirit: spirit,
		Charisma: charisma,
	}, nil
}