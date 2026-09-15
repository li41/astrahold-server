// Package equipmentcatalog owns Server-authoritative equipment gameplay data.
// Client names, meshes, icons, materials and animation mappings stay outside the Server.
package equipmentcatalog

import (
	_ "embed"
	"encoding/json"
	"errors"
	"math"
	"strings"

	"github.com/li41/astrahold-server/internal/characterstats"
)

//go:embed default.json
var defaultCatalogJSON []byte

var ErrInvalidCatalog = errors.New("equipmentcatalog: invalid catalog")

type Kind string
type Slot string
type Tier string
type BodySize string
type WeaponType string
type ArmorClass string

const (
	KindWeapon Kind = "weapon"
	KindShield Kind = "shield"
	KindArmor  Kind = "armor"

	SlotMainHand Slot = "main_hand"
	SlotOffHand  Slot = "off_hand"
	SlotHelmet   Slot = "helmet"
	SlotChest    Slot = "chest"
	SlotGloves   Slot = "gloves"
	SlotLegs     Slot = "legs"
	SlotBoots    Slot = "boots"

	ArmorClassCloth   ArmorClass = "cloth"
	ArmorClassLeather ArmorClass = "leather"
	ArmorClassHeavy   ArmorClass = "heavy"

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

type WeaponTypeDefinition struct {
	WeaponType                 WeaponType        `json:"weapon_type"`
	BasicAttackIntervalMS      *uint32           `json:"basic_attack_interval_ms,omitempty"`
	BasicAttackDamageAttribute characterstats.ID `json:"basic_attack_damage_attribute,omitempty"`
	BasicAttackRange           *float32          `json:"basic_attack_range,omitempty"`
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
	ItemArchetypeID  string                `json:"item_archetype_id"`
	Kind             Kind                  `json:"kind"`
	Slot             Slot                  `json:"slot"`
	Tier             Tier                  `json:"tier"`
	Weight           uint32                `json:"weight"`
	Material         string                `json:"material"`
	ArmorClass       ArmorClass            `json:"armor_class,omitempty"`
	SetID            SetID                 `json:"set_id,omitempty"`
	BaseRequirements []BaseStatRequirement `json:"base_requirements,omitempty"`
	StaticModifiers  []StaticModifier      `json:"static_modifiers,omitempty"`
	Weapon           *Weapon               `json:"weapon,omitempty"`
	Shield           *Shield               `json:"shield,omitempty"`
}

type CatalogDefinition struct {
	Revision    string                 `json:"revision"`
	WeaponTypes []WeaponTypeDefinition `json:"weapon_types,omitempty"`
	Sets        []SetDefinition        `json:"sets,omitempty"`
	Items       []Definition           `json:"items"`
}

type Catalog struct {
	revision    string
	byItem      map[string]Definition
	armorByItem map[string]Definition
	weaponTypes map[WeaponType]WeaponTypeDefinition
	sets        map[SetID]SetDefinition
}

func Default() (*Catalog, error) {
	var def CatalogDefinition
	decoder := json.NewDecoder(strings.NewReader(string(defaultCatalogJSON)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&def); err != nil {
		return nil, err
	}
	// The historical revision identifies the authored weapon progression in default.json. Protocol
	// v29 is the compatibility fence for the seven-slot model, so adding armor does not rewrite that
	// weapon-data revision string.
	def.Items = append(def.Items, defaultLowTierArmor()...)
	def.Items = append(def.Items, defaultRemainingArmor()...)
	def.Sets = append(def.Sets, defaultArmorSets()...)
	return New(def)
}

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
	sets, err := canonicalSetDefinitions(def.Sets)
	if err != nil {
		return nil, ErrInvalidCatalog
	}
	catalog := &Catalog{
		revision:    def.Revision,
		byItem:      make(map[string]Definition, len(def.Items)),
		armorByItem: make(map[string]Definition),
		weaponTypes: make(map[WeaponType]WeaponTypeDefinition, len(def.WeaponTypes)),
		sets:        sets,
	}
	for _, authored := range def.WeaponTypes {
		authored.WeaponType = WeaponType(strings.TrimSpace(string(authored.WeaponType)))
		authored.BasicAttackDamageAttribute = characterstats.ID(strings.TrimSpace(string(authored.BasicAttackDamageAttribute)))
		if authored.WeaponType == "" || !validBasicAttackDamageAttribute(authored.BasicAttackDamageAttribute) {
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
		if authored.BasicAttackRange != nil {
			attackRange := *authored.BasicAttackRange
			if attackRange <= 0 || math.IsNaN(float64(attackRange)) || math.IsInf(float64(attackRange), 0) {
				return nil, ErrInvalidCatalog
			}
			authored.BasicAttackRange = &attackRange
		}
		catalog.weaponTypes[authored.WeaponType] = authored
	}
	for _, item := range def.Items {
		item.ItemArchetypeID = strings.TrimSpace(item.ItemArchetypeID)
		item.Material = strings.TrimSpace(item.Material)
		item.ArmorClass = ArmorClass(strings.TrimSpace(string(item.ArmorClass)))
		item.SetID = SetID(strings.TrimSpace(string(item.SetID)))
		if item.ItemArchetypeID == "" || item.Material == "" || item.Weight == 0 || !validTier(item.Tier) {
			return nil, ErrInvalidCatalog
		}
		if _, exists := catalog.byItem[item.ItemArchetypeID]; exists {
			return nil, ErrInvalidCatalog
		}
		if _, exists := catalog.armorByItem[item.ItemArchetypeID]; exists {
			return nil, ErrInvalidCatalog
		}
		staticModifiers, err := canonicalStaticModifiers(item.StaticModifiers)
		if err != nil {
			return nil, ErrInvalidCatalog
		}
		item.StaticModifiers = staticModifiers
		baseRequirements, err := canonicalBaseRequirements(item.BaseRequirements)
		if err != nil {
			return nil, ErrInvalidCatalog
		}
		item.BaseRequirements = baseRequirements
		if item.SetID != "" {
			if _, ok := catalog.sets[item.SetID]; !ok {
				return nil, ErrInvalidCatalog
			}
		}
		switch item.Kind {
		case KindWeapon:
			if item.Slot != SlotMainHand || item.Weapon == nil || item.Shield != nil || item.ArmorClass != "" {
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
			catalog.byItem[item.ItemArchetypeID] = item
		case KindShield:
			if item.Slot != SlotOffHand || item.Shield == nil || item.Weapon != nil || item.ArmorClass != "" || !validShield(*item.Shield) {
				return nil, ErrInvalidCatalog
			}
			shield := *item.Shield
			item.Shield = &shield
			catalog.byItem[item.ItemArchetypeID] = item
		case KindArmor:
			if !validArmorSlot(item.Slot) || !validArmorClass(item.ArmorClass) || item.Weapon != nil || item.Shield != nil {
				return nil, ErrInvalidCatalog
			}
			catalog.armorByItem[item.ItemArchetypeID] = item
		default:
			return nil, ErrInvalidCatalog
		}
	}
	if err := validateSetPopulation(catalog); err != nil {
		return nil, ErrInvalidCatalog
	}
	return catalog, nil
}

func validArmorSlot(slot Slot) bool {
	switch slot {
	case SlotHelmet, SlotChest, SlotGloves, SlotLegs, SlotBoots:
		return true
	default:
		return false
	}
}

func validArmorClass(class ArmorClass) bool {
	switch class {
	case ArmorClassCloth, ArmorClassLeather, ArmorClassHeavy:
		return true
	default:
		return false
	}
}

func validTier(tier Tier) bool {
	switch tier {
	case TierLow, TierMid, TierHigh:
		return true
	default:
		return false
	}
}

func validBasicAttackDamageAttribute(attribute characterstats.ID) bool {
	switch attribute {
	case "", characterstats.Strength, characterstats.Agility:
		return true
	default:
		return false
	}
}

func validWeapon(w Weapon) bool {
	return w.WeaponType != "" && validRange(w.SmallDamage) && validRange(w.LargeDamage)
}

func validRange(r DamageRange) bool {
	return r.Min > 0 && r.Max >= r.Min
}

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
	id := strings.TrimSpace(itemArchetypeID)
	item, ok := c.byItem[id]
	if !ok {
		item, ok = c.armorByItem[id]
	}
	if !ok {
		return Definition{}, false
	}
	item.StaticModifiers = cloneStaticModifiers(item.StaticModifiers)
	item.BaseRequirements = cloneBaseRequirements(item.BaseRequirements)
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

func (c *Catalog) BasicAttackDamageAttributeForItem(itemArchetypeID string) (characterstats.ID, bool) {
	if c == nil {
		return "", false
	}
	item, ok := c.byItem[strings.TrimSpace(itemArchetypeID)]
	if !ok || item.Weapon == nil {
		return "", false
	}
	typeDefinition, ok := c.weaponTypes[item.Weapon.WeaponType]
	if !ok || typeDefinition.BasicAttackDamageAttribute == "" {
		return "", false
	}
	return typeDefinition.BasicAttackDamageAttribute, true
}

func (c *Catalog) BasicAttackRangeForItem(itemArchetypeID string) (float32, bool) {
	if c == nil {
		return 0, false
	}
	item, ok := c.byItem[strings.TrimSpace(itemArchetypeID)]
	if !ok || item.Weapon == nil {
		return 0, false
	}
	typeDefinition, ok := c.weaponTypes[item.Weapon.WeaponType]
	if !ok || typeDefinition.BasicAttackRange == nil {
		return 0, false
	}
	return *typeDefinition.BasicAttackRange, true
}

func (c *Catalog) UnitWeights() map[string]uint32 {
	if c == nil {
		return nil
	}
	weights := make(map[string]uint32, len(c.byItem)+len(c.armorByItem))
	for id, item := range c.byItem {
		weights[id] = item.Weight
	}
	for id, item := range c.armorByItem {
		weights[id] = item.Weight
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
