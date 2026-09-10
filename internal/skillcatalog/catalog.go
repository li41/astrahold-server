// Package skillcatalog defines the classless Astrahold skill identities and combat-loadout invariants.
package skillcatalog

import (
	"errors"
	"fmt"
)

type ID string

type Category string

const (
	CategoryUniversal Category = "universal"
	CategoryMelee     Category = "melee"
	CategoryRanged    Category = "ranged"
	CategoryMagic     Category = "magic"
	CategorySupport   Category = "support"
)

type Activation string

const (
	ActivationActive  Activation = "active"
	ActivationPassive Activation = "passive"
)

const MaxCombatLoadoutSkills = 6

const (
	RandomTeleport ID = "random-teleport"
	Sunlight        ID = "sunlight"
	Guard           ID = "guard"
	Heal            ID = "heal"

	HeavyStrike  ID = "heavy-strike"
	Flurry       ID = "flurry"
	Cleave       ID = "cleave"
	Execute      ID = "execute"
	PiercingShot ID = "piercing-shot"
	RapidShot    ID = "rapid-shot"
	Volley       ID = "volley"
	PinningShot  ID = "pinning-shot"
	FireBolt     ID = "fire-bolt"
	FrostBurst   ID = "frost-burst"
	Meteor       ID = "meteor"
	ArcaneLance  ID = "arcane-lance"

	StrongPhysique     ID = "strong-physique"
	ClearMeridians     ID = "clear-meridians"
	SpiritConcentration ID = "spirit-concentration"
	Haste               ID = "haste"
)

type Definition struct {
	ID                    ID
	Category              Category
	Activation            Activation
	CombatLoadoutEligible bool
}

var definitions = []Definition{
	{ID: RandomTeleport, Category: CategoryUniversal, Activation: ActivationActive},
	{ID: Sunlight, Category: CategoryUniversal, Activation: ActivationActive},
	{ID: Guard, Category: CategoryUniversal, Activation: ActivationActive},
	{ID: Heal, Category: CategoryUniversal, Activation: ActivationActive},

	{ID: HeavyStrike, Category: CategoryMelee, Activation: ActivationActive, CombatLoadoutEligible: true},
	{ID: Flurry, Category: CategoryMelee, Activation: ActivationActive, CombatLoadoutEligible: true},
	{ID: Cleave, Category: CategoryMelee, Activation: ActivationActive, CombatLoadoutEligible: true},
	{ID: Execute, Category: CategoryMelee, Activation: ActivationActive, CombatLoadoutEligible: true},
	{ID: PiercingShot, Category: CategoryRanged, Activation: ActivationActive, CombatLoadoutEligible: true},
	{ID: RapidShot, Category: CategoryRanged, Activation: ActivationActive, CombatLoadoutEligible: true},
	{ID: Volley, Category: CategoryRanged, Activation: ActivationActive, CombatLoadoutEligible: true},
	{ID: PinningShot, Category: CategoryRanged, Activation: ActivationActive, CombatLoadoutEligible: true},
	{ID: FireBolt, Category: CategoryMagic, Activation: ActivationActive, CombatLoadoutEligible: true},
	{ID: FrostBurst, Category: CategoryMagic, Activation: ActivationActive, CombatLoadoutEligible: true},
	{ID: Meteor, Category: CategoryMagic, Activation: ActivationActive, CombatLoadoutEligible: true},
	{ID: ArcaneLance, Category: CategoryMagic, Activation: ActivationActive, CombatLoadoutEligible: true},

	{ID: StrongPhysique, Category: CategorySupport, Activation: ActivationPassive},
	{ID: ClearMeridians, Category: CategorySupport, Activation: ActivationPassive},
	{ID: SpiritConcentration, Category: CategorySupport, Activation: ActivationPassive},
	{ID: Haste, Category: CategorySupport, Activation: ActivationActive},
}

var byID = func() map[ID]Definition {
	out := make(map[ID]Definition, len(definitions))
	for _, definition := range definitions {
		out[definition.ID] = definition
	}
	return out
}()

var (
	ErrUnknownSkill           = errors.New("skillcatalog: unknown skill")
	ErrNotCombatLoadoutSkill  = errors.New("skillcatalog: skill is not eligible for combat loadout")
	ErrDuplicateSkill         = errors.New("skillcatalog: duplicate skill")
	ErrCombatLoadoutTooLarge  = errors.New("skillcatalog: combat loadout exceeds six skills")
)

func All() []Definition {
	out := make([]Definition, len(definitions))
	copy(out, definitions)
	return out
}

func Lookup(id ID) (Definition, bool) {
	definition, ok := byID[id]
	return definition, ok
}

func ValidateCombatLoadout(ids []ID) error {
	if len(ids) > MaxCombatLoadoutSkills {
		return ErrCombatLoadoutTooLarge
	}
	seen := make(map[ID]struct{}, len(ids))
	for _, id := range ids {
		definition, ok := Lookup(id)
		if !ok {
			return fmt.Errorf("%w: %q", ErrUnknownSkill, id)
		}
		if !definition.CombatLoadoutEligible {
			return fmt.Errorf("%w: %q", ErrNotCombatLoadoutSkill, id)
		}
		if _, exists := seen[id]; exists {
			return fmt.Errorf("%w: %q", ErrDuplicateSkill, id)
		}
		seen[id] = struct{}{}
	}
	return nil
}
