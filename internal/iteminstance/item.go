// Package iteminstance owns unique Server-authoritative equipment instance state.
// Low-tier equipment may continue using the existing archetype stack path until inventory migration;
// mid/high-tier equipment must use an Instance so rolled affixes survive lifecycle and persistence.
package iteminstance

import (
	"encoding/json"
	"errors"
	"sort"
	"strings"

	"github.com/li41/astrahold-server/internal/equipmentaffix"
	"github.com/li41/astrahold-server/internal/equipmentcatalog"
)

var (
	ErrInvalidInstance = errors.New("iteminstance: invalid instance")
	ErrUnsupportedItem = errors.New("iteminstance: unsupported item")
)

type ID string

type Instance struct {
	ID               ID                     `json:"item_instance_id"`
	ItemArchetypeID  string                 `json:"item_archetype_id"`
	EnhancementLevel uint16                 `json:"enhancement_level,omitempty"`
	Affixes          []equipmentaffix.Affix `json:"affixes,omitempty"`
}

// Create creates one authoritative equipment instance and rolls its affixes exactly once.
// The caller owns stable instance-ID allocation and persistence transaction boundaries.
func Create(id ID, definition equipmentcatalog.Definition, random equipmentaffix.Roller) (Instance, error) {
	id = ID(strings.TrimSpace(string(id)))
	if id == "" || strings.TrimSpace(definition.ItemArchetypeID) == "" {
		return Instance{}, ErrInvalidInstance
	}
	tier, kind, err := affixDomain(definition)
	if err != nil {
		return Instance{}, err
	}
	affixes, err := equipmentaffix.Generate(tier, kind, random)
	if err != nil {
		return Instance{}, err
	}
	instance := Instance{
		ID:              id,
		ItemArchetypeID: definition.ItemArchetypeID,
		Affixes:         cloneAffixes(affixes),
	}
	if err := Validate(instance, definition); err != nil {
		return Instance{}, err
	}
	return instance, nil
}

// ValidateShape validates durable instance identity, canonical affix ordering, and the intrinsic
// strength-to-value mapping without consulting the current equipment catalog. Persistence can use
// this layer without rerolling or making historical durable state depend on presentation metadata.
func ValidateShape(instance Instance) error {
	if strings.TrimSpace(string(instance.ID)) == "" || string(instance.ID) != strings.TrimSpace(string(instance.ID)) {
		return ErrInvalidInstance
	}
	if strings.TrimSpace(instance.ItemArchetypeID) == "" || instance.ItemArchetypeID != strings.TrimSpace(instance.ItemArchetypeID) {
		return ErrInvalidInstance
	}
	for index, affix := range instance.Affixes {
		value, ok := equipmentaffix.ValueFor(affix.ID, affix.Strength)
		if !ok || value != affix.Value {
			return ErrInvalidInstance
		}
		if index > 0 && instance.Affixes[index-1].ID >= affix.ID {
			return ErrInvalidInstance
		}
	}
	return nil
}

func Validate(instance Instance, definition equipmentcatalog.Definition) error {
	if err := ValidateShape(instance); err != nil {
		return err
	}
	if instance.ItemArchetypeID != definition.ItemArchetypeID {
		return ErrInvalidInstance
	}
	tier, kind, err := affixDomain(definition)
	if err != nil {
		return err
	}
	if err := equipmentaffix.Validate(tier, kind, instance.Affixes); err != nil {
		return ErrInvalidInstance
	}
	return nil
}

func canonicalShape(instance Instance) (Instance, error) {
	canonical := Instance{
		ID:               instance.ID,
		ItemArchetypeID:  instance.ItemArchetypeID,
		EnhancementLevel: instance.EnhancementLevel,
		Affixes:          cloneAffixes(instance.Affixes),
	}
	sort.Slice(canonical.Affixes, func(i, j int) bool { return canonical.Affixes[i].ID < canonical.Affixes[j].ID })
	if len(canonical.Affixes) == 0 {
		canonical.Affixes = nil
	}
	if err := ValidateShape(canonical); err != nil {
		return Instance{}, err
	}
	return canonical, nil
}

// CanonicalShapeJSON is the catalog-independent durable encoding used inside character persistence.
func CanonicalShapeJSON(instance Instance) ([]byte, error) {
	canonical, err := canonicalShape(instance)
	if err != nil {
		return nil, err
	}
	return json.Marshal(canonical)
}

// CanonicalJSON is the durable value encoding for one item instance. It never rerolls affixes.
func CanonicalJSON(instance Instance, definition equipmentcatalog.Definition) ([]byte, error) {
	canonical, err := canonicalShape(instance)
	if err != nil {
		return nil, err
	}
	if err := Validate(canonical, definition); err != nil {
		return nil, err
	}
	return json.Marshal(canonical)
}

// DecodeCanonicalShapeJSON restores persistence-owned instance shape without a catalog lookup.
// Gameplay restore must still call Validate against the current authoritative item definition.
func DecodeCanonicalShapeJSON(data []byte) (Instance, error) {
	var instance Instance
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&instance); err != nil {
		return Instance{}, err
	}
	canonical, err := CanonicalShapeJSON(instance)
	if err != nil {
		return Instance{}, err
	}
	if string(canonical) != string(data) {
		return Instance{}, ErrInvalidInstance
	}
	instance.Affixes = cloneAffixes(instance.Affixes)
	return instance, nil
}

// DecodeCanonicalJSON restores persisted instance state and rejects non-canonical or invalid data.
// No random source is accepted here: reload can never reroll an item.
func DecodeCanonicalJSON(data []byte, definition equipmentcatalog.Definition) (Instance, error) {
	instance, err := DecodeCanonicalShapeJSON(data)
	if err != nil {
		return Instance{}, err
	}
	if err := Validate(instance, definition); err != nil {
		return Instance{}, err
	}
	return instance, nil
}

func affixDomain(definition equipmentcatalog.Definition) (equipmentaffix.Tier, equipmentaffix.EquipmentKind, error) {
	var tier equipmentaffix.Tier
	switch definition.Tier {
	case equipmentcatalog.TierLow:
		tier = equipmentaffix.TierLow
	case equipmentcatalog.TierMid:
		tier = equipmentaffix.TierMid
	case equipmentcatalog.TierHigh:
		tier = equipmentaffix.TierHigh
	default:
		return "", "", ErrUnsupportedItem
	}
	var kind equipmentaffix.EquipmentKind
	switch definition.Kind {
	case equipmentcatalog.KindWeapon:
		kind = equipmentaffix.EquipmentKindWeapon
	case equipmentcatalog.KindShield:
		kind = equipmentaffix.EquipmentKindShield
	case equipmentcatalog.KindArmor:
		kind = equipmentaffix.EquipmentKindArmor
	default:
		return "", "", ErrUnsupportedItem
	}
	return tier, kind, nil
}

func cloneAffixes(in []equipmentaffix.Affix) []equipmentaffix.Affix {
	if len(in) == 0 {
		return nil
	}
	return append([]equipmentaffix.Affix(nil), in...)
}
