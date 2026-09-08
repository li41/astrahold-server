package classaction

import (
	"errors"
	"testing"

	"github.com/li41/astrahold-server/internal/classid"
	"github.com/li41/astrahold-server/internal/classresource"
)

func TestRangerArmorPiercingArrowPolicy(t *testing.T) {
	policy, ok := ForAction(RangerArmorPiercingArrow)
	if !ok {
		t.Fatal("expected armor piercing arrow policy")
	}
	if policy.RequiredClass != classid.Ranger {
		t.Fatalf("RequiredClass=%q want=%q", policy.RequiredClass, classid.Ranger)
	}
	if policy.CostResource != classresource.HuntMomentum || policy.CostAmount != 30 {
		t.Fatalf("cost=%q %d want hunt_momentum 30", policy.CostResource, policy.CostAmount)
	}
	if policy.HitResource != classresource.Empty || policy.HitGain != 0 {
		t.Fatalf("unexpected hit gain=%q +%d", policy.HitResource, policy.HitGain)
	}
	if err := ValidateClass(RangerArmorPiercingArrow, classid.Ranger); err != nil {
		t.Fatal(err)
	}
	if err := ValidateClass(RangerArmorPiercingArrow, classid.Breaker); !errors.Is(err, ErrWrongClass) {
		t.Fatalf("wrong class err=%v", err)
	}
}
