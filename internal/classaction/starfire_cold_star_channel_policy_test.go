package classaction

import (
	"testing"

	"github.com/li41/astrahold-server/internal/classresource"
)

func TestStarfireColdStarChannelPolicyReducesHeatWithoutMinimumCost(t *testing.T) {
	policy, ok := ForAction(StarfireColdStarChannel)
	if !ok {
		t.Fatal("expected cold star channel policy")
	}
	if policy.AcceptedReductionResource != classresource.StarHeat || policy.AcceptedReductionAmount != 45 {
		t.Fatalf("accepted reduction=%q %d want star_heat 45", policy.AcceptedReductionResource, policy.AcceptedReductionAmount)
	}
	if policy.CostResource != classresource.Empty || policy.CostAmount != 0 {
		t.Fatalf("cold star channel must not require minimum heat as a cost: %+v", policy)
	}
	if policy.AcceptedResource != classresource.Empty || policy.AcceptedGain != 0 {
		t.Fatalf("unexpected accepted gain=%q +%d", policy.AcceptedResource, policy.AcceptedGain)
	}
}
