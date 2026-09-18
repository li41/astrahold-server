package monstercatalog

import (
	"errors"
	"math"
	"sort"
	"strings"

	"github.com/li41/astrahold-server/internal/equipmentcatalog"
)

var ErrInvalidCatalog = errors.New("monstercatalog: invalid catalog")

type AggroMode string

const (
	AggroAggressive AggroMode = "aggressive"
	AggroPassive    AggroMode = "passive"
)

type AssistPolicy string

const (
	AssistNone       AssistPolicy = "none"
	AssistSameFamily AssistPolicy = "same_family"
)

type AIProfile string

const (
	AIProfileMeleeRoamer AIProfile = "melee_roamer"
	AIProfileMeleeGuard  AIProfile = "melee_guard"
	AIProfileMeleeElite  AIProfile = "melee_elite"
)

const (
	BasicMeleeActionID = "monster-melee"

	MonsterGrayWolf           = "monster_gray_wolf"
	MonsterWildBoar           = "monster_wild_boar"
	MonsterWitherwillDeserter = "monster_witherwill_deserter"
	MonsterWitherwillEnforcer = "monster_witherwill_enforcer"
	MonsterWitherwillCaptain  = "monster_witherwill_captain"
	MonsterRedsoilWorkerAnt   = "monster_redsoil_worker_ant"
	MonsterRedsoilSoldierAnt  = "monster_redsoil_soldier_ant"
	MonsterRedsoilGuardAnt    = "monster_redsoil_guard_ant"
	MonsterRedsoilQueen       = "monster_redsoil_queen"
	MonsterShoreCrab          = "monster_shore_crab"
)

type Definition struct {
	ArchetypeID          string
	MonsterLevel         uint16
	MaxHP                uint32
	DamageMin            uint32
	DamageMax            uint32
	PhysicalDefense      uint32
	MagicDefense         uint32
	PhysicalHit          int32
	Evasion              int32
	CriticalRating       uint32
	BaseXP               uint32
	BodySize             equipmentcatalog.BodySize
	MoveSpeed            float32
	AttackRange          float32
	AggroRange           float32
	LeashRange           float32
	AttackIntervalSeconds float32
	CorpseHoldSeconds    uint32
	RespawnDelaySeconds  uint32
	AIProfile            AIProfile
	AggroMode            AggroMode
	AssistFamilyID       string
	AssistPolicy         AssistPolicy
	AssistRadius         float32
	MaxAssist            uint8
	MeleeActionID        string
}

type Catalog struct {
	byID map[string]Definition
	ids  []string
}

func New(definitions []Definition) (*Catalog, error) {
	if len(definitions) == 0 {
		return nil, ErrInvalidCatalog
	}
	catalog := &Catalog{
		byID: make(map[string]Definition, len(definitions)),
		ids:  make([]string, 0, len(definitions)),
	}
	for _, definition := range definitions {
		if err := validateDefinition(definition); err != nil {
			return nil, err
		}
		if _, exists := catalog.byID[definition.ArchetypeID]; exists {
			return nil, ErrInvalidCatalog
		}
		catalog.byID[definition.ArchetypeID] = definition
		catalog.ids = append(catalog.ids, definition.ArchetypeID)
	}
	sort.Strings(catalog.ids)
	return catalog, nil
}

func (c *Catalog) Resolve(archetypeID string) (Definition, bool) {
	if c == nil {
		return Definition{}, false
	}
	definition, ok := c.byID[strings.TrimSpace(archetypeID)]
	return definition, ok
}

func (c *Catalog) IDs() []string {
	if c == nil {
		return nil
	}
	return append([]string(nil), c.ids...)
}

func validateDefinition(definition Definition) error {
	if definition.ArchetypeID == "" || strings.TrimSpace(definition.ArchetypeID) != definition.ArchetypeID {
		return ErrInvalidCatalog
	}
	if definition.MonsterLevel == 0 || definition.MaxHP == 0 || definition.DamageMin == 0 || definition.DamageMax < definition.DamageMin || definition.BaseXP == 0 {
		return ErrInvalidCatalog
	}
	if definition.Evasion < 0 || definition.MeleeActionID == "" || strings.TrimSpace(definition.MeleeActionID) != definition.MeleeActionID {
		return ErrInvalidCatalog
	}
	switch definition.BodySize {
	case equipmentcatalog.BodySizeSmall, equipmentcatalog.BodySizeLarge, equipmentcatalog.BodySizeGiant:
	default:
		return ErrInvalidCatalog
	}
	if !positiveFinite(definition.MoveSpeed) || !positiveFinite(definition.AttackRange) || !positiveFinite(definition.AggroRange) || !positiveFinite(definition.LeashRange) || !positiveFinite(definition.AttackIntervalSeconds) {
		return ErrInvalidCatalog
	}
	if definition.AttackRange > definition.AggroRange || definition.AggroRange > definition.LeashRange || definition.CorpseHoldSeconds == 0 || definition.RespawnDelaySeconds == 0 {
		return ErrInvalidCatalog
	}
	switch definition.AIProfile {
	case AIProfileMeleeRoamer, AIProfileMeleeGuard, AIProfileMeleeElite:
	default:
		return ErrInvalidCatalog
	}
	switch definition.AggroMode {
	case AggroAggressive, AggroPassive:
	default:
		return ErrInvalidCatalog
	}
	switch definition.AssistPolicy {
	case AssistNone:
		if definition.AssistFamilyID != "" || definition.AssistRadius != 0 || definition.MaxAssist != 0 {
			return ErrInvalidCatalog
		}
	case AssistSameFamily:
		if definition.AssistFamilyID == "" || strings.TrimSpace(definition.AssistFamilyID) != definition.AssistFamilyID || !positiveFinite(definition.AssistRadius) || definition.MaxAssist == 0 {
			return ErrInvalidCatalog
		}
	default:
		return ErrInvalidCatalog
	}
	return nil
}

func positiveFinite(value float32) bool {
	v := float64(value)
	return value > 0 && !math.IsNaN(v) && !math.IsInf(v, 0)
}

var map1Catalog = mustCatalog(map1Definitions())

func Map1() *Catalog {
	return map1Catalog
}

func mustCatalog(definitions []Definition) *Catalog {
	catalog, err := New(definitions)
	if err != nil {
		panic(err)
	}
	return catalog
}

func map1Definitions() []Definition {
	return []Definition{
		{
			ArchetypeID: MonsterGrayWolf, MonsterLevel: 5, MaxHP: 50, DamageMin: 5, DamageMax: 13,
			PhysicalDefense: 10, MagicDefense: 5, PhysicalHit: 3, Evasion: 5, CriticalRating: 2, BaseXP: 20,
			BodySize: equipmentcatalog.BodySizeSmall, MoveSpeed: 4.5, AttackRange: 1.75, AggroRange: 9, LeashRange: 18,
			AttackIntervalSeconds: 1.35, CorpseHoldSeconds: 2, RespawnDelaySeconds: 25,
			AIProfile: AIProfileMeleeRoamer, AggroMode: AggroAggressive,
			AssistFamilyID: "family_gray_wolf", AssistPolicy: AssistSameFamily, AssistRadius: 6, MaxAssist: 2,
			MeleeActionID: BasicMeleeActionID,
		},
		{
			ArchetypeID: MonsterWildBoar, MonsterLevel: 7, MaxHP: 75, DamageMin: 6, DamageMax: 17,
			PhysicalDefense: 20, MagicDefense: 5, PhysicalHit: 1, Evasion: 1, CriticalRating: 0, BaseXP: 30,
			BodySize: equipmentcatalog.BodySizeLarge, MoveSpeed: 3.6, AttackRange: 1.9, AggroRange: 7, LeashRange: 16,
			AttackIntervalSeconds: 1.55, CorpseHoldSeconds: 3, RespawnDelaySeconds: 30,
			AIProfile: AIProfileMeleeRoamer, AggroMode: AggroPassive, AssistPolicy: AssistNone,
			MeleeActionID: BasicMeleeActionID,
		},
		{
			ArchetypeID: MonsterWitherwillDeserter, MonsterLevel: 8, MaxHP: 85, DamageMin: 7, DamageMax: 19,
			PhysicalDefense: 15, MagicDefense: 10, PhysicalHit: 3, Evasion: 3, CriticalRating: 1, BaseXP: 35,
			BodySize: equipmentcatalog.BodySizeSmall, MoveSpeed: 4, AttackRange: 1.8, AggroRange: 10, LeashRange: 20,
			AttackIntervalSeconds: 1.3, CorpseHoldSeconds: 2, RespawnDelaySeconds: 35,
			AIProfile: AIProfileMeleeRoamer, AggroMode: AggroAggressive,
			AssistFamilyID: "family_witherwill", AssistPolicy: AssistSameFamily, AssistRadius: 6, MaxAssist: 2,
			MeleeActionID: BasicMeleeActionID,
		},
		{
			ArchetypeID: MonsterWitherwillEnforcer, MonsterLevel: 12, MaxHP: 170, DamageMin: 9, DamageMax: 27,
			PhysicalDefense: 35, MagicDefense: 15, PhysicalHit: 4, Evasion: 2, CriticalRating: 1, BaseXP: 60,
			BodySize: equipmentcatalog.BodySizeSmall, MoveSpeed: 3.5, AttackRange: 1.9, AggroRange: 9, LeashRange: 18,
			AttackIntervalSeconds: 1.45, CorpseHoldSeconds: 3, RespawnDelaySeconds: 45,
			AIProfile: AIProfileMeleeGuard, AggroMode: AggroAggressive,
			AssistFamilyID: "family_witherwill", AssistPolicy: AssistSameFamily, AssistRadius: 8, MaxAssist: 3,
			MeleeActionID: BasicMeleeActionID,
		},
		{
			ArchetypeID: MonsterWitherwillCaptain, MonsterLevel: 18, MaxHP: 450, DamageMin: 13, DamageMax: 40,
			PhysicalDefense: 45, MagicDefense: 25, PhysicalHit: 7, Evasion: 6, CriticalRating: 4, BaseXP: 180,
			BodySize: equipmentcatalog.BodySizeSmall, MoveSpeed: 3.8, AttackRange: 2, AggroRange: 11, LeashRange: 24,
			AttackIntervalSeconds: 1.3, CorpseHoldSeconds: 5, RespawnDelaySeconds: 120,
			AIProfile: AIProfileMeleeElite, AggroMode: AggroAggressive,
			AssistFamilyID: "family_witherwill", AssistPolicy: AssistSameFamily, AssistRadius: 10, MaxAssist: 3,
			MeleeActionID: BasicMeleeActionID,
		},
		{
			ArchetypeID: MonsterRedsoilWorkerAnt, MonsterLevel: 7, MaxHP: 55, DamageMin: 5, DamageMax: 14,
			PhysicalDefense: 5, MagicDefense: 5, PhysicalHit: 1, Evasion: 4, CriticalRating: 0, BaseXP: 20,
			BodySize: equipmentcatalog.BodySizeSmall, MoveSpeed: 4, AttackRange: 1.35, AggroRange: 7, LeashRange: 14,
			AttackIntervalSeconds: 1.1, CorpseHoldSeconds: 2, RespawnDelaySeconds: 25,
			AIProfile: AIProfileMeleeRoamer, AggroMode: AggroPassive,
			AssistFamilyID: "family_redsoil_ant", AssistPolicy: AssistSameFamily, AssistRadius: 6, MaxAssist: 2,
			MeleeActionID: BasicMeleeActionID,
		},
		{
			ArchetypeID: MonsterRedsoilSoldierAnt, MonsterLevel: 11, MaxHP: 135, DamageMin: 8, DamageMax: 24,
			PhysicalDefense: 20, MagicDefense: 10, PhysicalHit: 3, Evasion: 3, CriticalRating: 1, BaseXP: 50,
			BodySize: equipmentcatalog.BodySizeLarge, MoveSpeed: 3.8, AttackRange: 1.7, AggroRange: 8, LeashRange: 16,
			AttackIntervalSeconds: 1.3, CorpseHoldSeconds: 2, RespawnDelaySeconds: 30,
			AIProfile: AIProfileMeleeGuard, AggroMode: AggroAggressive,
			AssistFamilyID: "family_redsoil_ant", AssistPolicy: AssistSameFamily, AssistRadius: 7, MaxAssist: 2,
			MeleeActionID: BasicMeleeActionID,
		},
		{
			ArchetypeID: MonsterRedsoilGuardAnt, MonsterLevel: 15, MaxHP: 220, DamageMin: 11, DamageMax: 33,
			PhysicalDefense: 40, MagicDefense: 20, PhysicalHit: 5, Evasion: 2, CriticalRating: 2, BaseXP: 90,
			BodySize: equipmentcatalog.BodySizeLarge, MoveSpeed: 3.4, AttackRange: 1.8, AggroRange: 8, LeashRange: 16,
			AttackIntervalSeconds: 1.4, CorpseHoldSeconds: 3, RespawnDelaySeconds: 45,
			AIProfile: AIProfileMeleeGuard, AggroMode: AggroAggressive,
			AssistFamilyID: "family_redsoil_ant", AssistPolicy: AssistSameFamily, AssistRadius: 8, MaxAssist: 3,
			MeleeActionID: BasicMeleeActionID,
		},
		{
			ArchetypeID: MonsterRedsoilQueen, MonsterLevel: 22, MaxHP: 1200, DamageMin: 18, DamageMax: 55,
			PhysicalDefense: 50, MagicDefense: 40, PhysicalHit: 6, Evasion: 0, CriticalRating: 4, BaseXP: 320,
			BodySize: equipmentcatalog.BodySizeGiant, MoveSpeed: 2.6, AttackRange: 2.4, AggroRange: 10, LeashRange: 22,
			AttackIntervalSeconds: 1.6, CorpseHoldSeconds: 5, RespawnDelaySeconds: 180,
			AIProfile: AIProfileMeleeElite, AggroMode: AggroAggressive,
			AssistFamilyID: "family_redsoil_ant", AssistPolicy: AssistSameFamily, AssistRadius: 10, MaxAssist: 4,
			MeleeActionID: BasicMeleeActionID,
		},
		{
			ArchetypeID: MonsterShoreCrab, MonsterLevel: 8, MaxHP: 95, DamageMin: 6, DamageMax: 18,
			PhysicalDefense: 35, MagicDefense: 10, PhysicalHit: 1, Evasion: 1, CriticalRating: 0, BaseXP: 30,
			BodySize: equipmentcatalog.BodySizeLarge, MoveSpeed: 2.8, AttackRange: 1.8, AggroRange: 6, LeashRange: 13,
			AttackIntervalSeconds: 1.65, CorpseHoldSeconds: 3, RespawnDelaySeconds: 35,
			AIProfile: AIProfileMeleeRoamer, AggroMode: AggroPassive, AssistPolicy: AssistNone,
			MeleeActionID: BasicMeleeActionID,
		},
	}
}
