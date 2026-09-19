// Package characterstats defines the authoritative classless character-attribute model.
package characterstats

import (
	"errors"
	"math"
)

// ID is a stable character-attribute identifier.
type ID string

const (
	Strength     ID = "strength"
	Agility      ID = "agility"
	Constitution ID = "constitution"
	Intelligence ID = "intelligence"
	Spirit       ID = "spirit"
	Charisma     ID = "charisma"

	BasePrimaryValue uint32 = 10
)

var (
	ErrOverflow     = errors.New("characterstats: attribute overflow")
	ErrInvalidBase  = errors.New("characterstats: invalid base attributes")
	ErrInvalidLevel = errors.New("characterstats: invalid character level")
)

// Primary contains the six authoritative classless primary attributes.
// Durable/base values start at 10 and can only increase through formally owned allocation paths.
// Effective values may additionally include Server-owned equipment/passive/temporary modifiers.
type Primary struct {
	Strength     uint32
	Agility      uint32
	Constitution uint32
	Intelligence uint32
	Spirit       uint32
	Charisma     uint32
}

// AdditiveBonus contains positive Server-owned additive modifiers. Temporary negative modifiers
// are a separate future status concern; this type must not be used to encode them by underflow.
type AdditiveBonus struct {
	Strength     uint32
	Agility      uint32
	Constitution uint32
	Intelligence uint32
	Spirit       uint32
	Charisma     uint32
}

func DefaultPrimary() Primary {
	return Primary{
		Strength: BasePrimaryValue,
		Agility: BasePrimaryValue,
		Constitution: BasePrimaryValue,
		Intelligence: BasePrimaryValue,
		Spirit: BasePrimaryValue,
		Charisma: BasePrimaryValue,
	}
}

// ValidateBase validates durable allocation truth. The formal V1 base cannot fall below 10;
// no lifetime upper cap is applied here because maximum level is not yet fixed.
func ValidateBase(primary Primary) error {
	if primary.Strength < BasePrimaryValue ||
		primary.Agility < BasePrimaryValue ||
		primary.Constitution < BasePrimaryValue ||
		primary.Intelligence < BasePrimaryValue ||
		primary.Spirit < BasePrimaryValue ||
		primary.Charisma < BasePrimaryValue {
		return ErrInvalidBase
	}
	return nil
}

// Effective applies positive additive modifiers without permitting integer wraparound.
func Effective(base Primary, bonus AdditiveBonus) (Primary, error) {
	strength, ok := add(base.Strength, bonus.Strength)
	if !ok { return Primary{}, ErrOverflow }
	agility, ok := add(base.Agility, bonus.Agility)
	if !ok { return Primary{}, ErrOverflow }
	constitution, ok := add(base.Constitution, bonus.Constitution)
	if !ok { return Primary{}, ErrOverflow }
	intelligence, ok := add(base.Intelligence, bonus.Intelligence)
	if !ok { return Primary{}, ErrOverflow }
	spirit, ok := add(base.Spirit, bonus.Spirit)
	if !ok { return Primary{}, ErrOverflow }
	charisma, ok := add(base.Charisma, bonus.Charisma)
	if !ok { return Primary{}, ErrOverflow }
	return Primary{
		Strength: strength,
		Agility: agility,
		Constitution: constitution,
		Intelligence: intelligence,
		Spirit: spirit,
		Charisma: charisma,
	}, nil
}

func add(left, right uint32) (uint32, bool) {
	if right > math.MaxUint32-left {
		return 0, false
	}
	return left + right, true
}