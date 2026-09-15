package equipmentcatalog

import (
	"sort"
	"strings"

	"github.com/li41/astrahold-server/internal/characterstats"
)

// SetID is a stable Server gameplay identity for an equipment set. It carries no Client asset path.
type SetID string

// BaseStatRequirement is evaluated against durable character base attributes only. Equipment,
// passive skill, status, and other derived bonuses must never satisfy this requirement.
type BaseStatRequirement struct {
	Stat    characterstats.ID `json:"stat"`
	Minimum uint32            `json:"minimum"`
}

// SetBonusDefinition is derived from the number of currently equipped pieces of one SetID.
// Set activation is never persisted as mutable state.
type SetBonusDefinition struct {
	Pieces          uint8            `json:"pieces"`
	StaticModifiers []StaticModifier `json:"static_modifiers"`
}

type SetDefinition struct {
	ID      SetID                `json:"set_id"`
	Bonuses []SetBonusDefinition `json:"bonuses"`
}

func canonicalBaseRequirements(in []BaseStatRequirement) ([]BaseStatRequirement, error) {
	if len(in) == 0 {
		return nil, nil
	}
	out := append([]BaseStatRequirement(nil), in...)
	for i := range out {
		out[i].Stat = characterstats.ID(strings.TrimSpace(string(out[i].Stat)))
		if !validPrimaryRequirementStat(out[i].Stat) || out[i].Minimum == 0 {
			return nil, ErrInvalidCatalog
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Stat < out[j].Stat })
	for i := 1; i < len(out); i++ {
		if out[i-1].Stat == out[i].Stat {
			return nil, ErrInvalidCatalog
		}
	}
	return out, nil
}

func validPrimaryRequirementStat(id characterstats.ID) bool {
	switch id {
	case characterstats.Strength,
		characterstats.Agility,
		characterstats.Constitution,
		characterstats.Intelligence,
		characterstats.Spirit,
		characterstats.Charisma:
		return true
	default:
		return false
	}
}

func primaryValue(primary characterstats.Primary, id characterstats.ID) uint32 {
	switch id {
	case characterstats.Strength:
		return primary.Strength
	case characterstats.Agility:
		return primary.Agility
	case characterstats.Constitution:
		return primary.Constitution
	case characterstats.Intelligence:
		return primary.Intelligence
	case characterstats.Spirit:
		return primary.Spirit
	case characterstats.Charisma:
		return primary.Charisma
	default:
		return 0
	}
}

// BaseRequirementsMet intentionally consumes only durable base Primary values.
func (d Definition) BaseRequirementsMet(primary characterstats.Primary) bool {
	for _, requirement := range d.BaseRequirements {
		if primaryValue(primary, requirement.Stat) < requirement.Minimum {
			return false
		}
	}
	return true
}

func canonicalSetDefinitions(in []SetDefinition) (map[SetID]SetDefinition, error) {
	sets := make(map[SetID]SetDefinition, len(in))
	for _, authored := range in {
		authored.ID = SetID(strings.TrimSpace(string(authored.ID)))
		if authored.ID == "" || len(authored.Bonuses) == 0 {
			return nil, ErrInvalidCatalog
		}
		if _, exists := sets[authored.ID]; exists {
			return nil, ErrInvalidCatalog
		}
		bonuses := append([]SetBonusDefinition(nil), authored.Bonuses...)
		sort.Slice(bonuses, func(i, j int) bool { return bonuses[i].Pieces < bonuses[j].Pieces })
		for i := range bonuses {
			if bonuses[i].Pieces == 0 || len(bonuses[i].StaticModifiers) == 0 {
				return nil, ErrInvalidCatalog
			}
			canonical, err := canonicalStaticModifiers(bonuses[i].StaticModifiers)
			if err != nil {
				return nil, err
			}
			bonuses[i].StaticModifiers = canonical
			if i > 0 && bonuses[i-1].Pieces == bonuses[i].Pieces {
				return nil, ErrInvalidCatalog
			}
		}
		authored.Bonuses = bonuses
		sets[authored.ID] = authored
	}
	return sets, nil
}

func validateSetPopulation(c *Catalog) error {
	if c == nil {
		return ErrInvalidCatalog
	}
	pieceCounts := make(map[SetID]int, len(c.sets))
	for _, item := range c.byItem {
		if item.SetID != "" {
			pieceCounts[item.SetID]++
		}
	}
	for _, item := range c.armorByItem {
		if item.SetID != "" {
			pieceCounts[item.SetID]++
		}
	}
	for id, set := range c.sets {
		pieces := pieceCounts[id]
		if pieces == 0 {
			return ErrInvalidCatalog
		}
		for _, bonus := range set.Bonuses {
			if int(bonus.Pieces) > pieces {
				return ErrInvalidCatalog
			}
		}
	}
	return nil
}

func cloneBaseRequirements(in []BaseStatRequirement) []BaseStatRequirement {
	if len(in) == 0 {
		return nil
	}
	return append([]BaseStatRequirement(nil), in...)
}

func cloneSetDefinition(in SetDefinition) SetDefinition {
	out := SetDefinition{ID: in.ID, Bonuses: make([]SetBonusDefinition, len(in.Bonuses))}
	for i, bonus := range in.Bonuses {
		out.Bonuses[i] = SetBonusDefinition{Pieces: bonus.Pieces, StaticModifiers: cloneStaticModifiers(bonus.StaticModifiers)}
	}
	return out
}

func (c *Catalog) SetDefinition(id SetID) (SetDefinition, bool) {
	if c == nil {
		return SetDefinition{}, false
	}
	set, ok := c.sets[SetID(strings.TrimSpace(string(id)))]
	if !ok {
		return SetDefinition{}, false
	}
	return cloneSetDefinition(set), true
}

// SetStaticModifiersForEquipped derives cumulative 2/5-piece-style bonuses from current equipment.
// Duplicate archetype IDs are ignored defensively; activation is never stored in mutable state.
func (c *Catalog) SetStaticModifiersForEquipped(itemArchetypeIDs []string) ([]StaticModifier, error) {
	if c == nil || len(itemArchetypeIDs) == 0 {
		return nil, nil
	}
	counts := make(map[SetID]int)
	seenItems := make(map[string]struct{}, len(itemArchetypeIDs))
	for _, rawID := range itemArchetypeIDs {
		id := strings.TrimSpace(rawID)
		if id == "" {
			continue
		}
		if _, duplicate := seenItems[id]; duplicate {
			continue
		}
		seenItems[id] = struct{}{}
		item, ok := c.Resolve(id)
		if !ok || item.SetID == "" {
			continue
		}
		counts[item.SetID]++
	}
	setIDs := make([]SetID, 0, len(counts))
	for id := range counts {
		setIDs = append(setIDs, id)
	}
	sort.Slice(setIDs, func(i, j int) bool { return setIDs[i] < setIDs[j] })
	out := make([]StaticModifier, 0, len(setIDs)*3)
	for _, id := range setIDs {
		set, ok := c.sets[id]
		if !ok {
			return nil, ErrInvalidCatalog
		}
		for _, bonus := range set.Bonuses {
			if counts[id] >= int(bonus.Pieces) {
				out = append(out, cloneStaticModifiers(bonus.StaticModifiers)...)
			}
		}
	}
	return out, nil
}

func defaultArmorSets() []SetDefinition {
	return []SetDefinition{
		{ID: "set_garrison_steel", Bonuses: []SetBonusDefinition{
			{Pieces: 2, StaticModifiers: []StaticModifier{{ID: StaticMaxHP, Value: 40}}},
			{Pieces: 5, StaticModifiers: []StaticModifier{{ID: StaticStrength, Value: 2}, {ID: StaticPhysicalDefense, Value: 2}}},
		}},
		{ID: "set_windchaser_huntgear", Bonuses: []SetBonusDefinition{
			{Pieces: 2, StaticModifiers: []StaticModifier{{ID: StaticPhysicalHit, Value: 2}}},
			{Pieces: 5, StaticModifiers: []StaticModifier{{ID: StaticDexterity, Value: 2}, {ID: StaticCriticalRating, Value: 1}}},
		}},
		{ID: "set_arcane_rune_robes", Bonuses: []SetBonusDefinition{
			{Pieces: 2, StaticModifiers: []StaticModifier{{ID: StaticMaxMP, Value: 20}}},
			{Pieces: 5, StaticModifiers: []StaticModifier{{ID: StaticIntelligence, Value: 2}, {ID: StaticMagicPower, Value: 1}}},
		}},
		{ID: "set_starforged_bastion", Bonuses: []SetBonusDefinition{
			{Pieces: 2, StaticModifiers: []StaticModifier{{ID: StaticMaxHP, Value: 80}, {ID: StaticPhysicalDefense, Value: 1}}},
			{Pieces: 5, StaticModifiers: []StaticModifier{{ID: StaticStrength, Value: 3}, {ID: StaticPhysicalDefense, Value: 2}}},
		}},
		{ID: "set_starshadow_huntgear", Bonuses: []SetBonusDefinition{
			{Pieces: 2, StaticModifiers: []StaticModifier{{ID: StaticPhysicalHit, Value: 4}, {ID: StaticEvasion, Value: 2}}},
			{Pieces: 5, StaticModifiers: []StaticModifier{{ID: StaticDexterity, Value: 3}, {ID: StaticCriticalRating, Value: 3}}},
		}},
		{ID: "set_starlight_robes", Bonuses: []SetBonusDefinition{
			{Pieces: 2, StaticModifiers: []StaticModifier{{ID: StaticMaxMP, Value: 40}, {ID: StaticMagicDefense, Value: 2}}},
			{Pieces: 5, StaticModifiers: []StaticModifier{{ID: StaticIntelligence, Value: 3}, {ID: StaticMagicPower, Value: 3}}},
		}},
	}
}

func defaultRemainingArmor() []Definition {
	const requirement = uint32(18)
	constitution := []BaseStatRequirement{{Stat: characterstats.Constitution, Minimum: requirement}}
	agility := []BaseStatRequirement{{Stat: characterstats.Agility, Minimum: requirement}}
	intelligence := []BaseStatRequirement{{Stat: characterstats.Intelligence, Minimum: requirement}}

	return []Definition{
		progressionArmor("item_garrison_steel_helm", SlotHelmet, TierMid, 5, "steel", ArmorClassHeavy, "set_garrison_steel", 2, 1, nil),
		progressionArmor("item_garrison_steel_cuirass", SlotChest, TierMid, 10, "steel", ArmorClassHeavy, "set_garrison_steel", 4, 1, nil),
		progressionArmor("item_garrison_steel_gauntlets", SlotGloves, TierMid, 4, "steel", ArmorClassHeavy, "set_garrison_steel", 1, 0, nil),
		progressionArmor("item_garrison_steel_greaves", SlotLegs, TierMid, 8, "steel", ArmorClassHeavy, "set_garrison_steel", 3, 1, nil),
		progressionArmor("item_garrison_steel_boots", SlotBoots, TierMid, 4, "steel", ArmorClassHeavy, "set_garrison_steel", 2, 1, nil),
		progressionArmor("item_windchaser_cap", SlotHelmet, TierMid, 2, "leather", ArmorClassLeather, "set_windchaser_huntgear", 1, 1, nil),
		progressionArmor("item_windchaser_armor", SlotChest, TierMid, 5, "leather", ArmorClassLeather, "set_windchaser_huntgear", 3, 2, nil),
		progressionArmor("item_windchaser_gloves", SlotGloves, TierMid, 2, "leather", ArmorClassLeather, "set_windchaser_huntgear", 1, 1, nil),
		progressionArmor("item_windchaser_leggings", SlotLegs, TierMid, 4, "leather", ArmorClassLeather, "set_windchaser_huntgear", 2, 1, nil),
		progressionArmor("item_windchaser_boots", SlotBoots, TierMid, 2, "leather", ArmorClassLeather, "set_windchaser_huntgear", 1, 1, nil),
		progressionArmor("item_arcane_rune_hood", SlotHelmet, TierMid, 1, "cloth", ArmorClassCloth, "set_arcane_rune_robes", 1, 2, nil),
		progressionArmor("item_arcane_rune_robe", SlotChest, TierMid, 3, "cloth", ArmorClassCloth, "set_arcane_rune_robes", 1, 4, nil),
		progressionArmor("item_arcane_rune_gloves", SlotGloves, TierMid, 1, "cloth", ArmorClassCloth, "set_arcane_rune_robes", 0, 1, nil),
		progressionArmor("item_arcane_rune_trousers", SlotLegs, TierMid, 2, "cloth", ArmorClassCloth, "set_arcane_rune_robes", 1, 3, nil),
		progressionArmor("item_arcane_rune_boots", SlotBoots, TierMid, 1, "cloth", ArmorClassCloth, "set_arcane_rune_robes", 1, 2, nil),
		progressionArmor("item_starforged_bastion_helm", SlotHelmet, TierHigh, 6, "steel", ArmorClassHeavy, "set_starforged_bastion", 3, 1, constitution),
		progressionArmor("item_starforged_bastion_cuirass", SlotChest, TierHigh, 12, "steel", ArmorClassHeavy, "set_starforged_bastion", 5, 2, constitution),
		progressionArmor("item_starforged_bastion_gauntlets", SlotGloves, TierHigh, 5, "steel", ArmorClassHeavy, "set_starforged_bastion", 2, 1, constitution),
		progressionArmor("item_starforged_bastion_greaves", SlotLegs, TierHigh, 10, "steel", ArmorClassHeavy, "set_starforged_bastion", 4, 1, constitution),
		progressionArmor("item_starforged_bastion_boots", SlotBoots, TierHigh, 5, "steel", ArmorClassHeavy, "set_starforged_bastion", 2, 1, constitution),
		progressionArmor("item_starshadow_hunter_cap", SlotHelmet, TierHigh, 2, "leather", ArmorClassLeather, "set_starshadow_huntgear", 2, 1, agility),
		progressionArmor("item_starshadow_hunter_armor", SlotChest, TierHigh, 6, "leather", ArmorClassLeather, "set_starshadow_huntgear", 3, 3, agility),
		progressionArmor("item_starshadow_hunter_gloves", SlotGloves, TierHigh, 2, "leather", ArmorClassLeather, "set_starshadow_huntgear", 1, 1, agility),
		progressionArmor("item_starshadow_hunter_leggings", SlotLegs, TierHigh, 5, "leather", ArmorClassLeather, "set_starshadow_huntgear", 2, 2, agility),
		progressionArmor("item_starshadow_hunter_boots", SlotBoots, TierHigh, 2, "leather", ArmorClassLeather, "set_starshadow_huntgear", 2, 1, agility),
		progressionArmor("item_starlight_hood", SlotHelmet, TierHigh, 1, "cloth", ArmorClassCloth, "set_starlight_robes", 1, 3, intelligence),
		progressionArmor("item_starlight_robe", SlotChest, TierHigh, 4, "cloth", ArmorClassCloth, "set_starlight_robes", 2, 5, intelligence),
		progressionArmor("item_starlight_gloves", SlotGloves, TierHigh, 1, "cloth", ArmorClassCloth, "set_starlight_robes", 1, 2, intelligence),
		progressionArmor("item_starlight_trousers", SlotLegs, TierHigh, 2, "cloth", ArmorClassCloth, "set_starlight_robes", 1, 4, intelligence),
		progressionArmor("item_starlight_boots", SlotBoots, TierHigh, 1, "cloth", ArmorClassCloth, "set_starlight_robes", 1, 2, intelligence),
	}
}

func progressionArmor(id string, slot Slot, tier Tier, weight uint32, material MaterialID, class ArmorClass, setID SetID, physicalDefense, magicDefense uint32, requirements []BaseStatRequirement) Definition {
	modifiers := make([]StaticModifier, 0, 2)
	if physicalDefense > 0 {
		modifiers = append(modifiers, StaticModifier{ID: StaticPhysicalDefense, Value: physicalDefense})
	}
	if magicDefense > 0 {
		modifiers = append(modifiers, StaticModifier{ID: StaticMagicDefense, Value: magicDefense})
	}
	return Definition{
		ItemArchetypeID:  id,
		Kind:             KindArmor,
		Slot:             slot,
		Tier:             tier,
		Weight:           weight,
		Material:         material,
		ArmorClass:       class,
		SetID:            setID,
		BaseRequirements: cloneBaseRequirements(requirements),
		StaticModifiers:  modifiers,
	}
}
