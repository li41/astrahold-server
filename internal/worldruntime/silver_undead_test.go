package worldruntime

import (
	"math"
	"testing"

	"github.com/li41/astrahold-server/internal/combat"
	"github.com/li41/astrahold-server/internal/equipmentcatalog"
	"github.com/li41/astrahold-server/internal/world"
)

func TestSilverUndeadBasicAttackDamageV1RequiresAllFormalConditions(t *testing.T) {
	const raw = uint32(11)
	cases := []struct {
		name           string
		material       equipmentcatalog.MaterialID
		classification world.EntityClassification
		actionID       string
		damageType     combat.DamageType
		want           uint32
	}{
		{"silver undead basic physical", equipmentcatalog.MaterialSilver, world.EntityClassificationUndead, basicAttackActionID, combat.DamagePhysical, 13},
		{"non-silver", equipmentcatalog.MaterialSteel, world.EntityClassificationUndead, basicAttackActionID, combat.DamagePhysical, raw},
		{"non-undead", equipmentcatalog.MaterialSilver, "", basicAttackActionID, combat.DamagePhysical, raw},
		{"non-basic", equipmentcatalog.MaterialSilver, world.EntityClassificationUndead, "skill-test", combat.DamagePhysical, raw},
		{"magic", equipmentcatalog.MaterialSilver, world.EntityClassificationUndead, basicAttackActionID, combat.DamageMagic, raw},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := silverUndeadBasicAttackDamageV1(tc.material, tc.classification, tc.actionID, tc.damageType, raw); got != tc.want {
				t.Fatalf("damage=%d want=%d", got, tc.want)
			}
		})
	}
}

func TestSilverUndeadRawPhysicalDamageV1FloorsAndSaturates(t *testing.T) {
	if got := silverUndeadRawPhysicalDamageV1(11); got != 13 {
		t.Fatalf("floor damage=%d want=13", got)
	}
	if got := silverUndeadRawPhysicalDamageV1(math.MaxUint32); got != math.MaxUint32 {
		t.Fatalf("saturated damage=%d want=%d", got, uint32(math.MaxUint32))
	}
}

func TestSilverUndeadMultiplierRunsBeforeCriticalAndMitigation(t *testing.T) {
	raw := silverUndeadBasicAttackDamageV1(
		equipmentcatalog.MaterialSilver,
		world.EntityClassificationUndead,
		basicAttackActionID,
		combat.DamagePhysical,
		11,
	)
	if raw != 13 {
		t.Fatalf("silver raw=%d want=13", raw)
	}
	result, err := resolveDamageMitigation(DamageRequest{
		RawDamage:  raw,
		DamageType: combat.DamagePhysical,
		Critical:   true,
	}, nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	// 11 -> floor(13.2)=13 from silver, then critical 13*1.5=19.5, final single round=20.
	if result.FinalDamage != 20 {
		t.Fatalf("critical silver damage=%d want=20", result.FinalDamage)
	}
}
