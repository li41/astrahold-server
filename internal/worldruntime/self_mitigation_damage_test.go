package worldruntime

import (
	"errors"
	"testing"

	"github.com/li41/astrahold-server/internal/combat"
	"github.com/li41/astrahold-server/internal/equipmentcatalog"
)

func TestSelfMitigationReducesPhysicalAndMagicBeforeOneFinalRound(t *testing.T) {
	physical, err := resolveDamageMitigation(DamageRequest{
		RawDamage:                  100,
		DamageType:                 combat.DamagePhysical,
		SelfDamageReductionPercent: 45,
	}, nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	if physical.FinalDamage != 55 || physical.Blocked {
		t.Fatalf("physical=%+v want damage=55", physical)
	}

	magic, err := resolveDamageMitigation(DamageRequest{
		RawDamage:                  100,
		DamageType:                 combat.DamageMagic,
		SelfDamageReductionPercent: 45,
	}, nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	if magic.FinalDamage != 55 || magic.Blocked {
		t.Fatalf("magic=%+v want damage=55", magic)
	}
}

func TestSelfMitigationComposesWithShieldWithoutChangingBlock(t *testing.T) {
	guard := &equipmentcatalog.Shield{
		PhysicalDefense:             4,
		BlockChancePercent:          10,
		BlockDamageReductionPercent: 30,
	}
	unblocked, err := resolveDamageMitigation(DamageRequest{
		RawDamage:                  100,
		DamageType:                 combat.DamagePhysical,
		Blockable:                  true,
		SelfDamageReductionPercent: 45,
	}, guard, 10)
	if err != nil {
		t.Fatal(err)
	}
	if unblocked.Blocked || unblocked.FinalDamage != 46 {
		t.Fatalf("unblocked=%+v want damage=46", unblocked)
	}

	blocked, err := resolveDamageMitigation(DamageRequest{
		RawDamage:                  100,
		DamageType:                 combat.DamagePhysical,
		Blockable:                  true,
		SelfDamageReductionPercent: 45,
	}, guard, 9)
	if err != nil {
		t.Fatal(err)
	}
	if !blocked.Blocked || blocked.FinalDamage != 32 {
		t.Fatalf("blocked=%+v want damage=32 blocked=true", blocked)
	}
}

func TestSelfMitigationRejectsImpossibleFullReduction(t *testing.T) {
	_, err := resolveDamageMitigation(DamageRequest{
		RawDamage:                  100,
		DamageType:                 combat.DamagePhysical,
		SelfDamageReductionPercent: 100,
	}, nil, 0)
	if !errors.Is(err, ErrInvalidSelfDamageReduction) {
		t.Fatalf("err=%v want ErrInvalidSelfDamageReduction", err)
	}
}
