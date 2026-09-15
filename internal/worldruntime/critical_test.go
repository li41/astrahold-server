package worldruntime

import (
	"testing"

	"github.com/li41/astrahold-server/internal/characterstats"
	"github.com/li41/astrahold-server/internal/combat"
	"github.com/li41/astrahold-server/internal/equipmentaffix"
	"github.com/li41/astrahold-server/internal/learnedskills"
	"github.com/li41/astrahold-server/internal/skillcatalog"
	"github.com/li41/astrahold-server/internal/skillloadout"
)

func TestResolveCriticalUsesBaseChancePlusEquippedRating(t *testing.T) {
	runtime, s := runtimeWithEquippedMidTierDamageAffix(t, equipmentaffix.Affix{
		ID: equipmentaffix.AffixCriticalRating,
		Strength: 2,
		Value: 2,
	})
	prepared := combat.PreparedAction{Definition: combat.ActionDefinition{Effect: combat.EffectDamage, CriticalEligible: true}}

	original := criticalRoll
	t.Cleanup(func() { criticalRoll = original })
	criticalRoll = func() uint32 { return 599 }
	critical, err := runtime.resolveCritical(s.EntityID, prepared)
	if err != nil { t.Fatal(err) }
	if !critical { t.Fatal("roll 599 should crit at 6%") }

	criticalRoll = func() uint32 { return 600 }
	critical, err = runtime.resolveCritical(s.EntityID, prepared)
	if err != nil { t.Fatal(err) }
	if critical { t.Fatal("roll 600 must not crit at 6%") }
}

func TestResolveCriticalUsesEffectiveAgilityAndEquippedRating(t *testing.T) {
	runtime, s := runtimeWithEquippedMidTierDamageAffix(t, equipmentaffix.Affix{
		ID: equipmentaffix.AffixCriticalRating,
		Strength: 2,
		Value: 2,
	})
	state, ok := runtime.characters.State(s.EntityID)
	if !ok {
		t.Fatal("character state missing")
	}
	runtime.characters.Remove(s.EntityID)
	state.PrimaryStats = characterstats.DefaultPrimary()
	state.PrimaryStats.Agility = 35
	if err := runtime.characters.RegisterState(state); err != nil {
		t.Fatal(err)
	}
	learned, err := learnedskills.NewSet([]skillcatalog.ID{skillcatalog.ClearMeridians})
	if err != nil {
		t.Fatal(err)
	}
	if err := runtime.characterSkills.restore(s.EntityID, learned, skillloadout.Slots{}); err != nil {
		t.Fatal(err)
	}

	prepared := combat.PreparedAction{Definition: combat.ActionDefinition{Effect: combat.EffectDamage, CriticalEligible: true}}
	original := criticalRoll
	t.Cleanup(func() { criticalRoll = original })

	// Effective AGI 40 contributes +1 percentage point; CriticalRating 2 contributes another +1.
	// With the 5% base chance the exact authoritative boundary is therefore 7% (700 bp).
	criticalRoll = func() uint32 { return 699 }
	critical, err := runtime.resolveCritical(s.EntityID, prepared)
	if err != nil { t.Fatal(err) }
	if !critical { t.Fatal("roll 699 should crit at 7%") }

	criticalRoll = func() uint32 { return 700 }
	critical, err = runtime.resolveCritical(s.EntityID, prepared)
	if err != nil { t.Fatal(err) }
	if critical { t.Fatal("roll 700 must not crit at 7%") }
}

func TestResolveCriticalSkipsIneligibleDamageWithoutRolling(t *testing.T) {
	original := criticalRoll
	t.Cleanup(func() { criticalRoll = original })
	criticalRoll = func() uint32 { t.Fatal("ineligible action rolled critical"); return 0 }

	critical, err := (&Runtime{}).resolveCritical(1, combat.PreparedAction{Definition: combat.ActionDefinition{Effect: combat.EffectDamage}})
	if err != nil { t.Fatal(err) }
	if critical { t.Fatal("ineligible damage crit") }
}

func TestCriticalMultiplierRunsBeforeDefenseAndFinalRounding(t *testing.T) {
	result, err := resolveDamageMitigation(DamageRequest{
		RawDamage: 101,
		DamageType: combat.DamagePhysical,
		Critical: true,
		AdditionalPhysicalDefense: 5,
	}, nil, 0)
	if err != nil { t.Fatal(err) }
	// 101 * 1.5 = 151.5; defense 5 => 20% mitigation => 121.2; final round => 121.
	if result.FinalDamage != 121 { t.Fatalf("critical mitigated damage=%d want=121", result.FinalDamage) }
}

func TestMagicCriticalUsesSameMultiplierBeforeMagicDefense(t *testing.T) {
	result, err := resolveDamageMitigation(DamageRequest{
		RawDamage: 100,
		DamageType: combat.DamageMagic,
		Critical: true,
		MagicDefense: 20,
	}, nil, 0)
	if err != nil { t.Fatal(err) }
	if result.FinalDamage != 75 { t.Fatalf("magic critical mitigated damage=%d want=75", result.FinalDamage) }
}
