package worldruntime

import (
	"errors"
	"testing"

	"github.com/li41/astrahold-server/internal/combat"
	"github.com/li41/astrahold-server/internal/equipmentcatalog"
)

func TestPhysicalDefenseIgnoreAffectsOnlyDefenseTerm(t *testing.T) {
	guard := &equipmentcatalog.Shield{PhysicalDefense: 4}
	base := DamageRequest{RawDamage: 210, DamageType: combat.DamagePhysical, Blockable: true}

	normal, err := resolveDamageMitigation(base, guard, 99)
	if err != nil {
		t.Fatal(err)
	}
	if normal.Blocked || normal.FinalDamage != 175 {
		t.Fatalf("normal=%+v want 175 unblocked", normal)
	}

	base.PhysicalDefenseIgnorePercent = 25
	piercing, err := resolveDamageMitigation(base, guard, 99)
	if err != nil {
		t.Fatal(err)
	}
	if piercing.Blocked || piercing.FinalDamage != 183 {
		t.Fatalf("piercing=%+v want 183 unblocked", piercing)
	}

	again, err := resolveDamageMitigation(DamageRequest{RawDamage: 210, DamageType: combat.DamagePhysical}, guard, 99)
	if err != nil || again.FinalDamage != 175 {
		t.Fatalf("later normal=%+v err=%v; ignore must not mutate shield", again, err)
	}
}

func TestPhysicalDefenseIgnoreDoesNotBypassBlockReduction(t *testing.T) {
	guard := &equipmentcatalog.Shield{PhysicalDefense: 4, BlockChancePercent: 100, BlockDamageReductionPercent: 30}
	got, err := resolveDamageMitigation(DamageRequest{
		RawDamage:                    210,
		DamageType:                   combat.DamagePhysical,
		Blockable:                    true,
		PhysicalDefenseIgnorePercent: 25,
	}, guard, 0)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Blocked || got.FinalDamage != 128 {
		t.Fatalf("got=%+v want damage=128 blocked=true", got)
	}
}

func TestPhysicalDefenseIgnoreRejectsInvalidOrMagicUse(t *testing.T) {
	_, err := resolveDamageMitigation(DamageRequest{RawDamage: 10, DamageType: combat.DamagePhysical, PhysicalDefenseIgnorePercent: 101}, nil, 0)
	if !errors.Is(err, ErrInvalidPhysicalDefenseIgnore) {
		t.Fatalf("ignore>100 err=%v", err)
	}
	_, err = resolveDamageMitigation(DamageRequest{RawDamage: 10, DamageType: combat.DamageMagic, PhysicalDefenseIgnorePercent: 25}, nil, 0)
	if !errors.Is(err, ErrInvalidPhysicalDefenseIgnore) {
		t.Fatalf("magic ignore err=%v", err)
	}
}
