// Package equipmentaffix owns Server-authoritative random equipment affix generation.
// It deliberately contains no Client presentation metadata and no persistence transport shape.
package equipmentaffix

import (
	"errors"
	"sort"
)

var (
	ErrInvalidTier       = errors.New("equipmentaffix: invalid tier")
	ErrInvalidKind       = errors.New("equipmentaffix: invalid equipment kind")
	ErrMissingRandom     = errors.New("equipmentaffix: missing random source")
	ErrInvalidRandomRoll = errors.New("equipmentaffix: invalid random roll")
	ErrInvalidAffixes    = errors.New("equipmentaffix: invalid affixes")
)

type Tier string

type EquipmentKind string

type AffixID string

const (
	TierLow  Tier = "low"
	TierMid  Tier = "mid"
	TierHigh Tier = "high"

	EquipmentKindWeapon EquipmentKind = "weapon"
	EquipmentKindShield EquipmentKind = "shield"
	EquipmentKindArmor  EquipmentKind = "armor"

	AffixStrength        AffixID = "affix_strength"
	AffixDexterity       AffixID = "affix_dexterity"
	AffixIntelligence    AffixID = "affix_intelligence"
	AffixConstitution    AffixID = "affix_constitution"
	AffixSpirit          AffixID = "affix_spirit"
	AffixCharisma        AffixID = "affix_charisma"
	AffixPhysicalHit     AffixID = "affix_physical_hit"
	AffixCriticalRating  AffixID = "affix_critical_rating"
	AffixPhysicalDamage  AffixID = "affix_physical_damage"
	AffixMagicPower      AffixID = "affix_magic_power"
	AffixEvasion         AffixID = "affix_evasion"
	AffixMaxHP           AffixID = "affix_max_hp"
	AffixMaxMP           AffixID = "affix_max_mp"
	AffixPhysicalDefense AffixID = "affix_physical_defense"
	AffixMagicDefense    AffixID = "affix_magic_defense"
)

type Affix struct {
	ID       AffixID `json:"affix_id"`
	Strength uint8   `json:"strength"`
	Value    uint32  `json:"value"`
}

// Roller is intentionally tiny so world-owned callers can supply their authoritative RNG
// without this package creating a process-global random source.
type Roller interface {
	Intn(n int) int
}

var weaponPool = []AffixID{
	AffixStrength,
	AffixDexterity,
	AffixIntelligence,
	AffixConstitution,
	AffixSpirit,
	AffixCharisma,
	AffixPhysicalHit,
	AffixCriticalRating,
	AffixPhysicalDamage,
	AffixMagicPower,
}

var shieldPool = []AffixID{
	AffixStrength,
	AffixDexterity,
	AffixIntelligence,
	AffixConstitution,
	AffixSpirit,
	AffixCharisma,
	AffixEvasion,
	AffixMaxHP,
	AffixMaxMP,
	AffixPhysicalDefense,
	AffixMagicDefense,
}

// armorPool follows the formal armor plan. PhysicalDamage is intentionally excluded so five high
// armor pieces cannot stack direct physical-damage affixes faster than the weapon progression.
var armorPool = []AffixID{
	AffixStrength,
	AffixDexterity,
	AffixIntelligence,
	AffixConstitution,
	AffixSpirit,
	AffixCharisma,
	AffixPhysicalHit,
	AffixCriticalRating,
	AffixEvasion,
	AffixMaxHP,
	AffixMaxMP,
	AffixPhysicalDefense,
	AffixMagicDefense,
	AffixMagicPower,
}

func AffixCount(tier Tier) (int, bool) {
	switch tier {
	case TierLow:
		return 0, true
	case TierMid:
		return 1, true
	case TierHigh:
		return 2, true
	default:
		return 0, false
	}
}

func AllowedPool(kind EquipmentKind) ([]AffixID, bool) {
	var pool []AffixID
	switch kind {
	case EquipmentKindWeapon:
		pool = weaponPool
	case EquipmentKindShield:
		pool = shieldPool
	case EquipmentKindArmor:
		pool = armorPool
	default:
		return nil, false
	}
	return append([]AffixID(nil), pool...), true
}

// Generate rolls affix type and strength on the Server. High-tier type selection is without
// replacement, so a single item can never carry the same AffixID twice.
func Generate(tier Tier, kind EquipmentKind, random Roller) ([]Affix, error) {
	count, ok := AffixCount(tier)
	if !ok {
		return nil, ErrInvalidTier
	}
	pool, ok := AllowedPool(kind)
	if !ok {
		return nil, ErrInvalidKind
	}
	if count == 0 {
		return nil, nil
	}
	if random == nil {
		return nil, ErrMissingRandom
	}

	affixes := make([]Affix, 0, count)
	available := append([]AffixID(nil), pool...)
	for len(affixes) < count {
		index, err := randomIndex(random, len(available))
		if err != nil {
			return nil, err
		}
		id := available[index]
		available = append(available[:index], available[index+1:]...)

		strength, err := randomStrength(tier, random)
		if err != nil {
			return nil, err
		}
		value, ok := ValueFor(id, strength)
		if !ok {
			return nil, ErrInvalidAffixes
		}
		affixes = append(affixes, Affix{ID: id, Strength: strength, Value: value})
	}

	// Affix order has no gameplay meaning. Canonical ordering prevents persistence from depending
	// on random selection order while preserving each ID's independently rolled strength.
	sort.Slice(affixes, func(i, j int) bool { return affixes[i].ID < affixes[j].ID })
	return affixes, nil
}

func Validate(tier Tier, kind EquipmentKind, affixes []Affix) error {
	count, ok := AffixCount(tier)
	if !ok {
		return ErrInvalidTier
	}
	pool, ok := AllowedPool(kind)
	if !ok {
		return ErrInvalidKind
	}
	if len(affixes) != count {
		return ErrInvalidAffixes
	}

	allowed := make(map[AffixID]struct{}, len(pool))
	for _, id := range pool {
		allowed[id] = struct{}{}
	}
	seen := make(map[AffixID]struct{}, len(affixes))
	for _, affix := range affixes {
		if _, ok := allowed[affix.ID]; !ok {
			return ErrInvalidAffixes
		}
		if _, duplicate := seen[affix.ID]; duplicate {
			return ErrInvalidAffixes
		}
		seen[affix.ID] = struct{}{}
		if !strengthAllowed(tier, affix.Strength) {
			return ErrInvalidAffixes
		}
		value, ok := ValueFor(affix.ID, affix.Strength)
		if !ok || affix.Value != value {
			return ErrInvalidAffixes
		}
	}
	return nil
}

func ValueFor(id AffixID, strength uint8) (uint32, bool) {
	if strength < 1 || strength > 5 {
		return 0, false
	}
	switch id {
	case AffixStrength,
		AffixDexterity,
		AffixIntelligence,
		AffixConstitution,
		AffixSpirit,
		AffixCharisma,
		AffixPhysicalHit,
		AffixCriticalRating,
		AffixPhysicalDamage,
		AffixMagicPower,
		AffixEvasion,
		AffixPhysicalDefense,
		AffixMagicDefense:
		return uint32(strength), true
	case AffixMaxHP:
		return uint32(strength) * 20, true
	case AffixMaxMP:
		return uint32(strength) * 10, true
	default:
		return 0, false
	}
}

func randomIndex(random Roller, n int) (int, error) {
	if n <= 0 {
		return 0, ErrInvalidRandomRoll
	}
	value := random.Intn(n)
	if value < 0 || value >= n {
		return 0, ErrInvalidRandomRoll
	}
	return value, nil
}

func randomStrength(tier Tier, random Roller) (uint8, error) {
	roll := random.Intn(10000)
	if roll < 0 || roll >= 10000 {
		return 0, ErrInvalidRandomRoll
	}
	return strengthForRoll(tier, roll)
}

func strengthForRoll(tier Tier, roll int) (uint8, error) {
	if roll < 0 || roll >= 10000 {
		return 0, ErrInvalidRandomRoll
	}
	switch tier {
	case TierMid:
		if roll < 9000 {
			return 1, nil
		}
		return 2, nil
	case TierHigh:
		switch {
		case roll < 9000:
			return 1, nil
		case roll < 9700:
			return 2, nil
		case roll < 9950:
			return 3, nil
		case roll < 9995:
			return 4, nil
		default:
			return 5, nil
		}
	default:
		return 0, ErrInvalidTier
	}
}

func strengthAllowed(tier Tier, strength uint8) bool {
	switch tier {
	case TierLow:
		return false
	case TierMid:
		return strength >= 1 && strength <= 2
	case TierHigh:
		return strength >= 1 && strength <= 5
	default:
		return false
	}
}
