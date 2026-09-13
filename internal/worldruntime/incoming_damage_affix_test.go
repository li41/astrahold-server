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
	if result.FinalDamage != 80 {
		t.Fatalf("final damage=%d want=80", result.FinalDamage)
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
	// 100 * 0.8 * 0.92 = 73.6, then the single final rounding step produces 74.
	if result.FinalDamage != 74 {
		t.Fatalf("final damage=%d want=74", result.FinalDamage)
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
	if result.FinalDamage != 50 {
		t.Fatalf("final damage=%d want=50", result.FinalDamage)
	}
}
