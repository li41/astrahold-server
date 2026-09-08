package worldruntime

import (
	"errors"
	"math"
	"testing"

	"github.com/li41/astrahold-server/internal/combat"
	"github.com/li41/astrahold-server/internal/equipmentcatalog"
)

func TestPhysicalDefenseFormulaLocksShieldV1(t *testing.T) {
	cases := []struct {
		defense uint32
		want    float64
	}{
		{2, 2.0 / 22.0},
		{3, 3.0 / 23.0},
		{4, 4.0 / 24.0},
	}
	for _, tc := range cases {
		if got := physicalMitigationRate(tc.defense); math.Abs(got-tc.want) > 1e-12 {
			t.Fatalf("defense=%d got=%v want=%v", tc.defense, got, tc.want)
		}
	}
}

func TestPhysicalShieldMitigationAndBlockUseOneFinalRound(t *testing.T) {
	guard := &equipmentcatalog.Shield{PhysicalDefense: 4, BlockChancePercent: 10, BlockDamageReductionPercent: 30}
	request := DamageRequest{RawDamage: 100, DamageType: combat.DamagePhysical, Blockable: true}

	blocked, err := resolveDamageMitigation(request, guard, 9)
	if err != nil {
		t.Fatal(err)
	}
	if !blocked.Blocked || blocked.FinalDamage != 58 {
		t.Fatalf("blocked=%+v want damage=58 blocked=true", blocked)
	}

	notBlocked, err := resolveDamageMitigation(request, guard, 10)
	if err != nil {
		t.Fatal(err)
	}
	if notBlocked.Blocked || notBlocked.FinalDamage != 83 {
		t.Fatalf("notBlocked=%+v want damage=83 blocked=false", notBlocked)
	}
}

func TestPhysicalDamageCanBeExplicitlyNonBlockable(t *testing.T) {
	guard := &equipmentcatalog.Shield{PhysicalDefense: 4, BlockChancePercent: 10, BlockDamageReductionPercent: 30}
	got, err := resolveDamageMitigation(DamageRequest{RawDamage: 100, DamageType: combat.DamagePhysical, Blockable: false}, guard, 0)
	if err != nil {
		t.Fatal(err)
	}
	if got.Blocked || got.FinalDamage != 83 {
		t.Fatalf("got=%+v", got)
	}
}

func TestMagicReductionIsIndependentFromBlock(t *testing.T) {
	runed := &equipmentcatalog.Shield{PhysicalDefense: 2, BlockChancePercent: 100, BlockDamageReductionPercent: 100, MagicDamageReductionPercent: 8}
	got, err := resolveDamageMitigation(DamageRequest{RawDamage: 100, DamageType: combat.DamageMagic, Blockable: true}, runed, 0)
	if err != nil {
		t.Fatal(err)
	}
	if got.Blocked || got.FinalDamage != 92 {
		t.Fatalf("got=%+v want damage=92 blocked=false", got)
	}
}

func TestDamageMitigationRoundsOnceHalfUpAndKeepsMinimumOne(t *testing.T) {
	magicHalf := &equipmentcatalog.Shield{MagicDamageReductionPercent: 50}
	got, err := resolveDamageMitigation(DamageRequest{RawDamage: 5, DamageType: combat.DamageMagic}, magicHalf, 0)
	if err != nil {
		t.Fatal(err)
	}
	if got.FinalDamage != 3 {
		t.Fatalf("half-up damage=%d want=3", got.FinalDamage)
	}

	guard := &equipmentcatalog.Shield{PhysicalDefense: 4, BlockChancePercent: 100, BlockDamageReductionPercent: 99}
	got, err = resolveDamageMitigation(DamageRequest{RawDamage: 1, DamageType: combat.DamagePhysical, Blockable: true}, guard, 0)
	if err != nil {
		t.Fatal(err)
	}
	if got.FinalDamage != 1 || !got.Blocked {
		t.Fatalf("minimum damage result=%+v", got)
	}
}

func TestZeroAndUnsupportedDamage(t *testing.T) {
	zero, err := resolveDamageMitigation(DamageRequest{RawDamage: 0, DamageType: combat.DamageType("future")}, nil, 0)
	if err != nil || zero != (DamageResult{}) {
		t.Fatalf("zero=%+v err=%v", zero, err)
	}
	_, err = resolveDamageMitigation(DamageRequest{RawDamage: 1, DamageType: combat.DamageType("future")}, nil, 0)
	if !errors.Is(err, ErrUnsupportedIncomingDamageType) {
		t.Fatalf("err=%v", err)
	}
}
