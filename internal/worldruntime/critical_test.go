package worldruntime

import (
	"testing"

	"github.com/li41/astrahold-server/internal/combat"
	"github.com/li41/astrahold-server/internal/equipmentaffix"
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
