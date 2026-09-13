// Package equipmentcatalog owns Server-authoritative equipment gameplay data.
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
type Tier string
type BodySize string
type WeaponType string

const (
	KindWeapon Kind = "weapon"
	KindShield Kind = "shield"

	SlotMainHand Slot = "main_hand"
	SlotOffHand  Slot = "off_hand"

	TierLow  Tier = "low"
	TierMid  Tier = "mid"
	TierHigh Tier = "high"

	BodySizeSmall BodySize = "small"
	BodySizeLarge BodySize = "large"
	BodySizeGiant BodySize = "giant"

	WeaponTypeOneHandSword WeaponType = "one_hand_sword"
	WeaponTypeOneHandAxe   WeaponType = "one_hand_axe"
	WeaponTypeMace         WeaponType = "mace"
)

type DamageRange struct {
	Min uint32 `json:"min"`
	Max uint32 `json:"max"`
}

// WeaponTypeDefinition owns shared base cadence for a gameplay weapon type. A nil interval means
// the type is classified but its formal cadence is not authored yet; callers must keep the normal
// action-definition cadence rather than inventing one from item data or presentation assets.
type WeaponTypeDefinition struct {
	WeaponType            WeaponType `json:"weapon_type"`
	BasicAttackIntervalMS *uint32    `json:"basic_attack_interval_ms,omitempty"`
}

type Weapon struct {
	WeaponType       WeaponType  `json:"weapon_type"`
	SmallDamage      DamageRange `json:"small_damage"`
	LargeDamage      DamageRange `json:"large_damage"`
	ExtraDamage      uint32      `json:"extra_damage"`
	AccuracyModifier int32       `json:"accuracy_modifier"`
}

type Shield struct {
	PhysicalDefense             uint32 `json:"physical_defense"`
	BlockChancePercent          uint8  `json:"block_chance_percent"`
	BlockDamageReductionPercent uint8  `json:"block_damage_reduction_percent"`
	MagicDamageReductionPercent uint8  `json:"magic_damage_reduction_percent"`
}

type Definition struct {
	ItemArchetypeID string  `json:"item_archetype_id"`
	Kind            Kind    `json:"kind"`
	Slot            Slot    `json:"slot"`
	Tier            Tier    `json:"tier"`
	Weight          uint32  `json:"weight"`
	Material        string  `json:"material"`
	Weapon          *Weapon `json:"weapon,omitempty"`
	Shield          *Shield `json:"shield,omitempty"`
}

type CatalogDefinition struct {
	Revision    string                 `json:"revision"`
	WeaponTypes []WeaponTypeDefinition `json:"weapon_types,omitempty"`
	Items       []Definition           `json:"items"`
}

type Catalog struct {
	revision    string
	byItem      map[string]Definition
	weaponTypes map[WeaponType]WeaponTypeDefinition
}

func Default() (*Catalog, error) { return Load(defaultCatalogJSON) }

func Load(data []byte) (*Catalog, error) {
	var def CatalogDefinition
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&def); err != nil {
		return nil, err
	}
	// Authored catalog JSON must explicitly declare tier. Programmatic New callers may omit tier
	// only for historical low-tier test fixtures; this keeps production content fail-closed while
	// avoiding unrelated fixture churn.
	for _, item := range def.Items {
		if item.Tier == "" {
			return nil, ErrInvalidCatalog
		}
	}
	return New(def)
}

func New(def CatalogDefinition) (*Catalog, error) {
	def.Revision = strings.TrimSpace(def.Revision)
	if def.Revision == "" || len(def.Items) == 0 {
		return nil, ErrInvalidCatalog
	}
	catalog := &Catalog{
		revision:    def.Revision,
		byItem:      make(map[string]Definition, len(def.Items)),
		weaponTypes: make(map[WeaponType]WeaponTypeDefinition, len(def.WeaponTypes)),
	}
	for _, authored := range def.WeaponTypes {
		authored.WeaponType = WeaponType(strings.TrimSpace(string(authored.WeaponType)))
		if authored.WeaponType == "" {
			return nil, ErrInvalidCatalog
		}
		if _, exists := catalog.weaponTypes[authored.WeaponType]; exists {
			return nil, ErrInvalidCatalog
		}
		if authored.BasicAttackIntervalMS != nil {
			if *authored.BasicAttackIntervalMS == 0 {
				return nil, ErrInvalidCatalog
			}
			interval := *authored.BasicAttackIntervalMS
			authored.BasicAttackIntervalMS = &interval
		}
		catalog.weaponTypes[authored.WeaponType] = authored
	}
	for _, item := range def.Items {
		item.ItemArchetypeID = strings.TrimSpace(item.ItemArchetypeID)
		item.Material = strings.TrimSpace(item.Material)
		if item.Tier == "" {
			item.Tier = TierLow
		}
		if item.ItemArchetypeID == "" || item.Material == "" || item.Weight == 0 || !validTier(item.Tier) {
			return nil, ErrInvalidCatalog
		}
		if _, exists := catalog.byItem[item.ItemArchetypeID]; exists {
			return nil, ErrInvalidCatalog
		}
		switch item.Kind {
		case KindWeapon:
			if item.Slot != SlotMainHand || item.Weapon == nil || item.Shield != nil {
				return nil, ErrInvalidCatalog
			}
			weapon := *item.Weapon
			weapon.WeaponType = WeaponType(strings.TrimSpace(string(weapon.WeaponType)))
			if !validWeapon(weapon) {
				return nil, ErrInvalidCatalog
			}
			if _, ok := catalog.weaponTypes[weapon.WeaponType]; !ok {
				return nil, ErrInvalidCatalog
			}
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

func validTier(tier Tier) bool {
	switch tier {
	case TierLow, TierMid, TierHigh:
		return true
	default:
		return false
	}
}

func validWeapon(w Weapon) bool {
	return w.WeaponType != "" && validRange(w.SmallDamage) && validRange(w.LargeDamage)
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

// BasicAttackIntervalMSForItem resolves shared WeaponType cadence. False means either the item is
// not a catalog weapon or that WeaponType has no formally authored cadence yet.
func (c *Catalog) BasicAttackIntervalMSForItem(itemArchetypeID string) (uint32, bool) {
	if c == nil {
		return 0, false
	}
	item, ok := c.byItem[strings.TrimSpace(itemArchetypeID)]
	if !ok || item.Weapon == nil {
		return 0, false
	}
	typeDefinition, ok := c.weaponTypes[item.Weapon.WeaponType]
	if !ok || typeDefinition.BasicAttackIntervalMS == nil {
		return 0, false
	}
	return *typeDefinition.BasicAttackIntervalMS, true
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