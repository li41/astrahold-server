package worldruntime

import (
	"testing"
	"time"

	"github.com/li41/astrahold-server/internal/combat"
	"github.com/li41/astrahold-server/internal/monstercatalog"
	"github.com/li41/astrahold-server/internal/world"
)

func TestMonsterSpawnFromCatalogRegistersAuthoredCombatStats(t *testing.T) {
	runtime, _ := runtimeWithJoinedPlayerForStaticModifierTest(t)
	definition, ok := monstercatalog.Map1().Resolve(monstercatalog.MonsterGrayWolf)
	if !ok {
		t.Fatal("gray wolf definition missing")
	}
	request, err := NewMonsterSpawnEntityRequest(definition, world.EntityState{
		ID:          9001,
		Kind:        world.EntityMonster,
		ArchetypeID: definition.ArchetypeID,
		Transform:   world.Transform{Position: world.Position{X: 2, Layer: 0}},
	}, 0.35, 0.5)
	if err != nil {
		t.Fatal(err)
	}
	if err := runtime.EnqueueSpawnEntity(request); err != nil {
		t.Fatal(err)
	}
	report := runtime.Step(2, 50*time.Millisecond)
	if len(report.CommandErrors) != 0 {
		t.Fatalf("spawn errors=%#v", report.CommandErrors)
	}

	stats, ok := runtime.monsterStats(9001)
	if !ok {
		t.Fatal("monster combat stats missing")
	}
	if stats.MonsterLevel != 5 || stats.MaxHP != 50 || stats.DamageMin != 5 || stats.DamageMax != 13 ||
		stats.PhysicalDefense != 10 || stats.MagicDefense != 5 || stats.PhysicalHit != 3 ||
		stats.Evasion != 5 || stats.CriticalRating != 2 || stats.BaseXP != 20 ||
		stats.MeleeActionID != monstercatalog.BasicMeleeActionID {
		t.Fatalf("gray wolf stats=%+v", stats)
	}
	state, ok := runtime.combatantState(9001)
	if !ok || state.HP != 50 || state.MaxHP != 50 || state.Defeated {
		t.Fatalf("gray wolf vitals=%+v ok=%v", state, ok)
	}
}

func TestMonsterDamageRangeUsesClosedServerOwnedInterval(t *testing.T) {
	stats := MonsterCombatStats{DamageMin: 5, DamageMax: 13}
	if got := rollMonsterDamage(stats, 0); got != 5 {
		t.Fatalf("min roll=%d want=5", got)
	}
	if got := rollMonsterDamage(stats, 8); got != 13 {
		t.Fatalf("max roll=%d want=13", got)
	}
	if got := rollMonsterDamage(stats, 9); got != 5 {
		t.Fatalf("wrapped roll=%d want=5", got)
	}
}

func TestMonsterPhysicalHitAndCriticalUseAuthoredRatings(t *testing.T) {
	runtime, player := runtimeWithJoinedPlayerForStaticModifierTest(t)
	definition, _ := monstercatalog.Map1().Resolve(monstercatalog.MonsterGrayWolf)
	request, err := NewMonsterSpawnEntityRequest(definition, world.EntityState{
		ID:          9001,
		Kind:        world.EntityMonster,
		ArchetypeID: definition.ArchetypeID,
		Transform:   world.Transform{Position: world.Position{X: 1, Layer: 0}},
	}, 0.35, 0.5)
	if err != nil {
		t.Fatal(err)
	}
	if err := runtime.EnqueueSpawnEntity(request); err != nil {
		t.Fatal(err)
	}
	if report := runtime.Step(2, 50*time.Millisecond); len(report.CommandErrors) != 0 {
		t.Fatalf("spawn errors=%#v", report.CommandErrors)
	}

	prepared := combat.PreparedAction{
		ActorEntityID: 9001,
		Definition: combat.ActionDefinition{
			ID:               monstercatalog.BasicMeleeActionID,
			Effect:           combat.EffectDamage,
			CriticalEligible: true,
		},
		Target: combat.Target{Kind: combat.TargetEntity, ID: "710"},
		Damage: combat.Damage{Type: combat.DamagePhysical},
	}

	originalHit := weaponAccuracyRoll
	originalCritical := criticalRoll
	t.Cleanup(func() {
		weaponAccuracyRoll = originalHit
		criticalRoll = originalCritical
	})

	// Gray wolf PhysicalHit 3 versus default player Evasion 0 = 91.5%.
	weaponAccuracyRoll = func() uint32 { return 9149 }
	if !runtime.resolveEquippedBasicAttackHit(9001, 0, prepared) {
		t.Fatal("roll 9149 should hit at 91.5%")
	}
	weaponAccuracyRoll = func() uint32 { return 9150 }
	if runtime.resolveEquippedBasicAttackHit(9001, 0, prepared) {
		t.Fatal("roll 9150 must miss at 91.5%")
	}

	// Gray wolf CriticalRating 2 = 5% base + 1% = 6%.
	criticalRoll = func() uint32 { return 599 }
	critical, err := runtime.resolveCritical(9001, prepared)
	if err != nil || !critical {
		t.Fatalf("roll 599 critical=%v err=%v", critical, err)
	}
	criticalRoll = func() uint32 { return 600 }
	critical, err = runtime.resolveCritical(9001, prepared)
	if err != nil || critical {
		t.Fatalf("roll 600 critical=%v err=%v", critical, err)
	}

	if player.EntityID != 710 {
		t.Fatalf("unexpected player entity=%d", player.EntityID)
	}
}

func TestMonsterDefenseFeedsCommonIncomingDamageResolver(t *testing.T) {
	runtime, _ := runtimeWithJoinedPlayerForStaticModifierTest(t)
	runtime.monsterCombatStats[9001] = MonsterCombatStats{
		MonsterLevel: 22, MaxHP: 1200, DamageMin: 18, DamageMax: 55,
		PhysicalDefense: 50, MagicDefense: 40, Evasion: 0, BaseXP: 320,
		MeleeActionID: monstercatalog.BasicMeleeActionID,
	}

	physical, err := runtime.resolveIncomingDamage(DamageRequest{
		TargetEntityID: 9001,
		RawDamage:      100,
		DamageType:     combat.DamagePhysical,
	}, 1)
	if err != nil {
		t.Fatal(err)
	}
	if physical.FinalDamage != 67 {
		t.Fatalf("queen physical result=%+v want damage=67", physical)
	}

	magic, err := runtime.resolveIncomingDamage(DamageRequest{
		TargetEntityID: 9001,
		RawDamage:      100,
		DamageType:     combat.DamageMagic,
	}, 1)
	if err != nil {
		t.Fatal(err)
	}
	if magic.FinalDamage != 71 {
		t.Fatalf("queen magic result=%+v want damage=71", magic)
	}
}
