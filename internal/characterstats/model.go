// Package characterstats defines the authoritative classless character-attribute model.
//
// This package intentionally contains only attributes whose existence is already
// part of the current product contract. It does not invent base values, point
// budgets, level gains, caps, respec rules, or combat scaling formulas.
package characterstats

import (
	"errors"
	"math"
)

// ID is a stable character-attribute identifier.
type ID string

const (
	Strength ID = "strength"
	Agility  ID = "agility"
)

var ErrOverflow = errors.New("characterstats: attribute overflow")

// Primary contains the current authoritative primary attributes that have been
// formally established for the classless character model.
type Primary struct {
	Strength uint32
	Agility  uint32
}

// AdditiveBonus contains Server-owned additive bonuses applied on top of the
// authoritative primary values. The model is deliberately unsigned: current
// product rules establish positive bonuses (+5 Strength and +5 Agility) but do
// not yet establish negative primary-attribute modifiers.
type AdditiveBonus struct {
	Strength uint32
	Agility  uint32
}

// Effective applies additive bonuses without permitting integer wraparound.
// It does not apply any unapproved caps or scaling formulas.
func Effective(base Primary, bonus AdditiveBonus) (Primary, error) {
	strength, ok := add(base.Strength, bonus.Strength)
	if !ok {
		return Primary{}, ErrOverflow
	}
	agility, ok := add(base.Agility, bonus.Agility)
	if !ok {
		return Primary{}, ErrOverflow
	}
	return Primary{Strength: strength, Agility: agility}, nil
}

func add(left, right uint32) (uint32, bool) {
	if right > math.MaxUint32-left {
		return 0, false
	}
	return left + right, true
}
