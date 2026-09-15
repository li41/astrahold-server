// Package appearance owns Server-authoritative character skin identity and gameplay affinity.
// Asset paths, model filenames and third-party character UUIDs are presentation concerns and must
// never become gameplay identity.
package appearance

import (
	"errors"

	"github.com/li41/astrahold-server/internal/equipmentcatalog"
	"github.com/li41/astrahold-server/internal/world"
)

// SkinID is a stable Server-owned character appearance identifier.
type SkinID string

const (
	None SkinID = ""

	KnightDPelegrini SkinID = "skin_knight_d_pelegrini"
	Ninja            SkinID = "skin_ninja"
	PeasantGirl      SkinID = "skin_peasant_girl"
	CastleGuard01    SkinID = "skin_castle_guard_01"
	PeasantMan       SkinID = "skin_peasant_man"
	Ortiz            SkinID = "skin_ortiz"
	Drake            SkinID = "skin_drake"
	HerakliosByADizon SkinID = "skin_heraklios_by_a_dizon"
	Brute            SkinID = "skin_brute"
	CastleGuard02    SkinID = "skin_castle_guard_02"
	TheBoss          SkinID = "skin_the_boss"
	GanfaulMAure     SkinID = "skin_ganfaul_m_aure"
	KachujinGRosales SkinID = "skin_kachujin_g_rosales"
	ErikaArcher      SkinID = "skin_erika_archer"
	Swat             SkinID = "skin_swat"
	Jackie           SkinID = "skin_jackie"
	Arissa           SkinID = "skin_arissa"
)

var ErrInvalidSkin = errors.New("appearance: invalid skin")

// Definition contains gameplay data only. WeaponAffinity grants the V1 flat +1 basic-attack raw
// damage bonus when the authoritative equipped main-hand weapon has the same WeaponType.
type Definition struct {
	ID             SkinID
	WeaponAffinity equipmentcatalog.WeaponType
}

var definitions = map[SkinID]Definition{
	KnightDPelegrini: {ID: KnightDPelegrini, WeaponAffinity: equipmentcatalog.WeaponType("one_hand_sword")},
	Ninja:            {ID: Ninja, WeaponAffinity: equipmentcatalog.WeaponType("dagger")},
	PeasantGirl:      {ID: PeasantGirl, WeaponAffinity: equipmentcatalog.WeaponType("one_hand_axe")},
	CastleGuard01:    {ID: CastleGuard01, WeaponAffinity: equipmentcatalog.WeaponType("one_hand_spear")},
	PeasantMan:       {ID: PeasantMan, WeaponAffinity: equipmentcatalog.WeaponType("warhammer")},
	Ortiz:            {ID: Ortiz, WeaponAffinity: equipmentcatalog.WeaponType("morning_star")},
	Drake:            {ID: Drake, WeaponAffinity: equipmentcatalog.WeaponType("mace")},
	HerakliosByADizon: {ID: HerakliosByADizon, WeaponAffinity: equipmentcatalog.WeaponType("two_hand_sword")},
	Brute:            {ID: Brute, WeaponAffinity: equipmentcatalog.WeaponType("two_hand_axe")},
	CastleGuard02:    {ID: CastleGuard02, WeaponAffinity: equipmentcatalog.WeaponType("two_hand_spear")},
	TheBoss:          {ID: TheBoss, WeaponAffinity: equipmentcatalog.WeaponType("knuckles")},
	GanfaulMAure:     {ID: GanfaulMAure, WeaponAffinity: equipmentcatalog.WeaponType("claw")},
	KachujinGRosales: {ID: KachujinGRosales, WeaponAffinity: equipmentcatalog.WeaponType("dual_blades")},
	ErikaArcher:      {ID: ErikaArcher, WeaponAffinity: equipmentcatalog.WeaponType("bow")},
	Swat:             {ID: Swat, WeaponAffinity: equipmentcatalog.WeaponType("crossbow")},
	Jackie:           {ID: Jackie, WeaponAffinity: equipmentcatalog.WeaponType("sling")},
	Arissa:           {ID: Arissa, WeaponAffinity: equipmentcatalog.WeaponType("staff")},
}

func Resolve(id SkinID) (Definition, bool) {
	definition, ok := definitions[id]
	return definition, ok
}

// ValidSelection accepts None because old characters and characters that have not selected a skin
// intentionally receive no affinity bonus.
func ValidSelection(id SkinID) bool {
	if id == None {
		return true
	}
	_, ok := Resolve(id)
	return ok
}

func WeaponAffinity(id SkinID) (equipmentcatalog.WeaponType, bool) {
	definition, ok := Resolve(id)
	if !ok {
		return "", false
	}
	return definition.WeaponAffinity, true
}

// Store is world-owner mutable runtime truth. It intentionally has no independent lock or network
// mutation path; callers must mutate it through the authoritative Runtime owner.
type Store struct {
	byEntity map[world.EntityID]SkinID
}

func (s *Store) Set(entityID world.EntityID, id SkinID) error {
	if entityID == 0 || !ValidSelection(id) {
		return ErrInvalidSkin
	}
	if s.byEntity == nil {
		s.byEntity = make(map[world.EntityID]SkinID)
	}
	if id == None {
		delete(s.byEntity, entityID)
		return nil
	}
	s.byEntity[entityID] = id
	return nil
}

func (s *Store) Get(entityID world.EntityID) SkinID {
	if s == nil || entityID == 0 {
		return None
	}
	return s.byEntity[entityID]
}

func (s *Store) ClearEntity(entityID world.EntityID) {
	if s == nil || entityID == 0 {
		return
	}
	delete(s.byEntity, entityID)
}
