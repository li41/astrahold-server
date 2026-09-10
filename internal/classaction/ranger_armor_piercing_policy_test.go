package classaction

import (
	"testing"

	"github.com/li41/astrahold-server/internal/classresource"
)

func TestRangerArmorPiercingArrowPolicy(t *testing.T) {
	policy, ok := ForAction(RangerArmorPiercingArrow)
	if !ok {
		t.Fatal("expected armor piercing arrow policy")
	}
	if policy.CostResource != classresource.HuntMomentum || policy.CostAmount != 30 {
		t.Fatalf("cost=%q %d want hunt_momentum 30", policy.CostResource, policy.CostAmount)
	}
	if policy.HitResource != classresource.Empty || policy.HitGain != 0 {
		t.Fatalf("unexpected hit gain=%q +%d", policy.HitResource, policy.HitGain)
	}
}
