package worldruntime

import (
	"testing"

	"github.com/li41/astrahold-server/internal/combat"
	"github.com/li41/astrahold-server/internal/equipmentcatalog"
)

func TestPhysicalDefenseAffixAddsToShieldDefenseBeforeMitigation(t *testing.T) {
	shield := &equipmentcatalog.Shield{PhysicalDefense: 3}
	result, err := resolveDamageMitigation(DamageRequest{
		RawDamage:                 100,
		DamageType:                combat.DamagePhysical,
		AdditionalPhysicalDefense: 2,
	}, shield, 0)
	if err != nil {
		t.Fatalf("resolveDamageMitigation: %v", err)
	}
	if result.FinalDamage != 95 {
		t.Fatalf("final damage=%d want=95", result.FinalDamage)
	}
}

func TestMagicDefenseAffixPrecedesShieldMagicReductionWithoutIntermediateRounding(t *testing.T) {
	shield := &equipmentcatalog.Shield{MagicDamageReductionPercent: 8}
	result, err := resolveDamageMitigation(DamageRequest{
		RawDamage:    100,
		DamageType:   combat.DamageMagic,
		MagicDefense: 5,
	}, shield, 0)
	if err != nil {
		t.Fatalf("resolveDamageMitigation: %v", err)
	}
	// 100 * (100/105) * 0.92 = 87.619..., then the single final rounding step produces 88.
	if result.FinalDamage != 88 {
		t.Fatalf("final damage=%d want=88", result.FinalDamage)
	}
}

func TestMagicDefenseWithoutShieldUsesFormalDefenseCurve(t *testing.T) {
	result, err := resolveDamageMitigation(DamageRequest{
		RawDamage:    100,
		DamageType:   combat.DamageMagic,
		MagicDefense: 20,
	}, nil, 0)
	if err != nil {
		t.Fatalf("resolveDamageMitigation: %v", err)
	}
	if result.FinalDamage != 83 {
		t.Fatalf("final damage=%d want=83", result.FinalDamage)
	}
}
