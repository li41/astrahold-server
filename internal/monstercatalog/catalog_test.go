package monstercatalog

import "testing"

func TestMap1CatalogLocksAuthoredV1Roster(t *testing.T) {
	catalog := Map1()
	if got := len(catalog.IDs()); got != 10 {
		t.Fatalf("map1 monster count=%d want=10", got)
	}

	cases := []struct {
		id                         string
		level                      uint16
		hp, min, max               uint32
		physical, magic            uint32
		hit, evasion               int32
		critical, xp               uint32
		aggro                      AggroMode
		assist                     AssistPolicy
		assistRadius               float32
		maxAssist                  uint8
	}{
		{MonsterGrayWolf, 5, 50, 5, 13, 10, 5, 3, 5, 2, 20, AggroAggressive, AssistSameFamily, 6, 2},
		{MonsterWildBoar, 7, 75, 6, 17, 20, 5, 1, 1, 0, 30, AggroPassive, AssistNone, 0, 0},
		{MonsterWitherwillDeserter, 8, 85, 7, 19, 15, 10, 3, 3, 1, 35, AggroAggressive, AssistSameFamily, 6, 2},
		{MonsterWitherwillEnforcer, 12, 170, 9, 27, 35, 15, 4, 2, 1, 60, AggroAggressive, AssistSameFamily, 8, 3},
		{MonsterWitherwillCaptain, 18, 450, 13, 40, 45, 25, 7, 6, 4, 180, AggroAggressive, AssistSameFamily, 10, 3},
		{MonsterRedsoilWorkerAnt, 7, 55, 5, 14, 5, 5, 1, 4, 0, 20, AggroPassive, AssistSameFamily, 6, 2},
		{MonsterRedsoilSoldierAnt, 11, 135, 8, 24, 20, 10, 3, 3, 1, 50, AggroAggressive, AssistSameFamily, 7, 2},
		{MonsterRedsoilGuardAnt, 15, 220, 11, 33, 40, 20, 5, 2, 2, 90, AggroAggressive, AssistSameFamily, 8, 3},
		{MonsterRedsoilQueen, 22, 1200, 18, 55, 50, 40, 6, 0, 4, 320, AggroAggressive, AssistSameFamily, 10, 4},
		{MonsterShoreCrab, 8, 95, 6, 18, 35, 10, 1, 1, 0, 30, AggroPassive, AssistNone, 0, 0},
	}

	for _, tc := range cases {
		definition, ok := catalog.Resolve(tc.id)
		if !ok {
			t.Fatalf("missing monster %q", tc.id)
		}
		if definition.MonsterLevel != tc.level || definition.MaxHP != tc.hp || definition.DamageMin != tc.min || definition.DamageMax != tc.max {
			t.Fatalf("%s level/hp/damage=%d/%d/%d-%d", tc.id, definition.MonsterLevel, definition.MaxHP, definition.DamageMin, definition.DamageMax)
		}
		if definition.PhysicalDefense != tc.physical || definition.MagicDefense != tc.magic || definition.PhysicalHit != tc.hit || definition.Evasion != tc.evasion || definition.CriticalRating != tc.critical || definition.BaseXP != tc.xp {
			t.Fatalf("%s combat stats=%+v", tc.id, definition)
		}
		if definition.AggroMode != tc.aggro || definition.AssistPolicy != tc.assist || definition.AssistRadius != tc.assistRadius || definition.MaxAssist != tc.maxAssist {
			t.Fatalf("%s aggro/assist=%+v", tc.id, definition)
		}
		if definition.MeleeActionID != BasicMeleeActionID {
			t.Fatalf("%s melee action=%q want=%q", tc.id, definition.MeleeActionID, BasicMeleeActionID)
		}
	}
}

func TestMap1AssistFamiliesStayIntentional(t *testing.T) {
	catalog := Map1()
	wolf, _ := catalog.Resolve(MonsterGrayWolf)
	if wolf.AssistFamilyID != "family_gray_wolf" {
		t.Fatalf("wolf assist family=%q", wolf.AssistFamilyID)
	}
	for _, id := range []string{MonsterWitherwillDeserter, MonsterWitherwillEnforcer, MonsterWitherwillCaptain} {
		definition, _ := catalog.Resolve(id)
		if definition.AssistFamilyID != "family_witherwill" {
			t.Fatalf("%s assist family=%q", id, definition.AssistFamilyID)
		}
	}
	for _, id := range []string{MonsterRedsoilWorkerAnt, MonsterRedsoilSoldierAnt, MonsterRedsoilGuardAnt, MonsterRedsoilQueen} {
		definition, _ := catalog.Resolve(id)
		if definition.AssistFamilyID != "family_redsoil_ant" {
			t.Fatalf("%s assist family=%q", id, definition.AssistFamilyID)
		}
	}
	for _, id := range []string{MonsterWildBoar, MonsterShoreCrab} {
		definition, _ := catalog.Resolve(id)
		if definition.AssistPolicy != AssistNone || definition.AssistFamilyID != "" {
			t.Fatalf("%s should not assist: %+v", id, definition)
		}
	}
}
