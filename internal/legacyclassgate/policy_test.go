package legacyclassgate

import (
	"errors"
	"testing"

	"github.com/li41/astrahold-server/internal/classaction"
	"github.com/li41/astrahold-server/internal/classid"
)

func TestLegacyV27ClassGateMatchesLegacyActionCatalog(t *testing.T) {
	tests := []struct {
		action string
		want   classid.ID
		wrong  classid.ID
	}{
		{classaction.OathguardSwordStrike, classid.Oathguard, classid.Ranger},
		{classaction.OathguardFortify, classid.Oathguard, classid.Ranger},
		{classaction.BreakerHeavySlash, classid.Breaker, classid.Oathguard},
		{classaction.BreakerStaggerStrike, classid.Breaker, classid.Ranger},
		{classaction.RangerHuntingShot, classid.Ranger, classid.Oathguard},
		{classaction.RangerArmorPiercingArrow, classid.Ranger, classid.Breaker},
		{classaction.StarfireFireBolt, classid.StarfireMage, classid.Ranger},
		{classaction.StarfireColdStarChannel, classid.StarfireMage, classid.Oathguard},
		{classaction.OathhealerOathlightStrike, classid.Oathhealer, classid.StarfireMage},
		{classaction.ShadowbladeDualBladeStrike, classid.Shadowblade, classid.Oathguard},
		{classaction.ShadowbladeRiftStab, classid.Shadowblade, classid.Oathguard},
		{classaction.ShadowbladeFlawExecute, classid.Shadowblade, classid.Oathguard},
	}
	for _, tc := range tests {
		t.Run(tc.action, func(t *testing.T) {
			got, ok := requiredClass(tc.action)
			if !ok || got != tc.want {
				t.Fatalf("requiredClass(%q) = %q, %v; want %q, true", tc.action, got, ok, tc.want)
			}
			if err := Validate(tc.action, tc.want); err != nil {
				t.Fatalf("matching class rejected: %v", err)
			}
			if err := Validate(tc.action, tc.wrong); !errors.Is(err, ErrWrongClass) {
				t.Fatalf("wrong class error = %v, want ErrWrongClass", err)
			}
		})
	}
}

func TestLegacyV27ClassGateLeavesUnscopedActionsToTheirLegalityOwner(t *testing.T) {
	if _, ok := requiredClass("basic-attack"); ok {
		t.Fatal("basic-attack unexpectedly entered legacy class gate")
	}
	if err := Validate("basic-attack", ""); err != nil {
		t.Fatalf("unscoped action rejected: %v", err)
	}
}
