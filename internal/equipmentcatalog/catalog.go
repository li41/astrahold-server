// Package equipmentcatalog owns Server-authoritative low-tier equipment gameplay data.
// Client names, meshes, icons, materials and animation mappings stay outside the Server.
package equipmentcatalog

import (
	_ "embed"
	"encoding/json"
	"errors"
	"strings"
)

//go:embed default.json
var defaultCatalogJSON []byte

var ErrInvalidCatalog = errors.New("equipmentcatalog: invalid catalog")

type Kind string
type Slot string
type BodySize string
type ClassPolicy string

const (
	KindWeapon Kind = "weapon"
	KindShield Kind = "shield"

	SlotMainHand Slot = "main_hand"
	SlotOffHand  Slot = "off_hand"

	BodySizeSmall BodySize = "small"
	BodySizeLarge BodySize = "large"
	BodySizeGiant BodySize = "giant"

	ClassPolicyAll       ClassPolicy = "all"
	ClassPolicyAllowList ClassPolicy = "allow_list"
)

type DamageRange struct {
	Min uint32 `json:"min"`
	Max uint32 `json:"max"`
}

type Weapon struct {
	SmallDamage           DamageRange `json:"small_damage"`
	LargeDamage           DamageRange `json:"large_damage"`
	ExtraDamage           uint32      `json:"extra_damage"`
	AccuracyModifier      int32       `json:"accuracy_modifier"`
	BasicAttackIntervalMS uint32      `json:"basic_attack_interval_ms"`
}

type Shield struct {
	PhysicalDefense             uint32 `json:"physical_defense"`
	BlockChancePercent          uint8  `json:"block_chance_percent"`
	BlockDamageReductionPercent uint8  `json:"block_damage_reduction_percent"`
	MagicDamageReductionPercent uint8  `json:"magic_damage_reduction_percent"`
}

type Definition struct {
	ItemArchetypeID string      `json:"item_archetype_id"`
	Kind            Kind        `json:"kind"`
	Slot            Slot        `json:"slot"`
	Weight          uint32      `json:"weight"`
	Material        string      `json:"material"`
	ClassPolicy     ClassPolicy `json:"class_policy,omitempty"`
	AllowedClassIDs []string    `json:"allowed_class_ids,omitempty"`
	Weapon          *Weapon     `json:"weapon,omitempty"`
	Shield          *Shield     `json:"shield,omitempty"`
}

type CatalogDefinition struct {
	Revision string       `json:"revision"`
	Items    []Definition `json:"items"`
}

type Catalog struct {
	revision string
	byItem   map[string]Definition
}

func Default() (*Catalog, error) { return Load(defaultCatalogJSON) }

func Load(data []byte) (*Catalog, error) {
	var def CatalogDefinition
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&def); err != nil {
		return nil, err
	}
	return New(def)
}

func New(def CatalogDefinition) (*Catalog, error) {
	def.Revision = strings.TrimSpace(def.Revision)
	if def.Revision == "" || len(def.Items) == 0 {
		return nil, ErrInvalidCatalog
	}
	catalog := &Catalog{revision: def.Revision, byItem: make(map[string]Definition, len(def.Items))}
	for _, item := range def.Items {
		item.ItemArchetypeID = strings.TrimSpace(item.ItemArchetypeID)
		item.Material = strings.TrimSpace(item.Material)
		item.ClassPolicy = effectiveClassPolicy(item.ClassPolicy)
		if item.ItemArchetypeID == "" || item.Material == "" || item.Weight == 0 || !normalizeClassPolicy(&item) {
			return nil, ErrInvalidCatalog
		}
		if _, exists := catalog.byItem[item.ItemArchetypeID]; exists {
			return nil, ErrInvalidCatalog
		}
		switch item.Kind {
		case KindWeapon:
			if item.Slot != SlotMainHand || item.Weapon == nil || item.Shield != nil || !validWeapon(*item.Weapon) {
				return nil, ErrInvalidCatalog
			}
			weapon := *item.Weapon
			item.Weapon = &weapon
		case KindShield:
			if item.Slot != SlotOffHand || item.Shield == nil || item.Weapon != nil || !validShield(*item.Shield) {
				return nil, ErrInvalidCatalog
			}
			shield := *item.Shield
			item.Shield = &shield
		default:
			return nil, ErrInvalidCatalog
		}
		catalog.byItem[item.ItemArchetypeID] = item
	}
	return catalog, nil
}

func effectiveClassPolicy(policy ClassPolicy) ClassPolicy {
	if policy == "" {
		return ClassPolicyAll
	}
	return policy
}

func normalizeClassPolicy(item *Definition) bool {
	if item == nil {
		return false
	}
	switch item.ClassPolicy {
	case ClassPolicyAll:
		return len(item.AllowedClassIDs) == 0
	case ClassPolicyAllowList:
		if len(item.AllowedClassIDs) == 0 {
			return false
		}
		seen := make(map[string]struct{}, len(item.AllowedClassIDs))
		for i, classID := range item.AllowedClassIDs {
			classID = strings.TrimSpace(classID)
			if classID == "" {
				return false
			}
			if _, exists := seen[classID]; exists {
				return false
			}
			seen[classID] = struct{}{}
			item.AllowedClassIDs[i] = classID
		}
		return true
	default:
		return false
	}
}

func validWeapon(w Weapon) bool {
	return validRange(w.SmallDamage) && validRange(w.LargeDamage) && w.BasicAttackIntervalMS > 0
}

func validRange(r DamageRange) bool { return r.Min > 0 && r.Max >= r.Min }

func validShield(s Shield) bool {
	if s.BlockChancePercent > 100 || s.BlockDamageReductionPercent > 100 || s.MagicDamageReductionPercent > 100 {
		return false
	}
	return (s.BlockChancePercent == 0) == (s.BlockDamageReductionPercent == 0)
}

func (c *Catalog) Revision() string {
	if c == nil {
		return ""
	}
	return c.revision
}

func (c *Catalog) Resolve(itemArchetypeID string) (Definition, bool) {
	if c == nil {
		return Definition{}, false
	}
	item, ok := c.byItem[strings.TrimSpace(itemArchetypeID)]
	if !ok {
		return Definition{}, false
	}
	item.AllowedClassIDs = append([]string(nil), item.AllowedClassIDs...)
	if item.Weapon != nil {
		copy := *item.Weapon
		item.Weapon = &copy
	}
	if item.Shield != nil {
		copy := *item.Shield
		item.Shield = &copy
	}
	return item, true
}

func (d Definition) AllowsClass(classID string) bool {
	switch effectiveClassPolicy(d.ClassPolicy) {
	case ClassPolicyAll:
		return true
	case ClassPolicyAllowList:
		classID = strings.TrimSpace(classID)
		if classID == "" {
			return false
		}
		for _, allowed := range d.AllowedClassIDs {
			if allowed == classID {
				return true
			}
		}
	}
	return false
}

// UnitWeights returns a defensive copy of the authored carry weight for every catalog item.
// Inventory policy consumes this view so equipment has one Server-authoritative weight source.
func (c *Catalog) UnitWeights() map[string]uint32 {
	if c == nil {
		return nil
	}
	weights := make(map[string]uint32, len(c.byItem))
	for itemArchetypeID, item := range c.byItem {
		weights[itemArchetypeID] = item.Weight
	}
	return weights
}

func (d Definition) DamageRangeFor(size BodySize) DamageRange {
	if d.Weapon == nil {
		return DamageRange{}
	}
	switch size {
	case BodySizeLarge, BodySizeGiant:
		return d.Weapon.LargeDamage
	default:
		return d.Weapon.SmallDamage
	}
}
