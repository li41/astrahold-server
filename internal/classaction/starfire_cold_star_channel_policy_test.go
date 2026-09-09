package classaction

import (
	"errors"
	"testing"

	"github.com/li41/astrahold-server/internal/classid"
	"github.com/li41/astrahold-server/internal/classresource"
)

func TestStarfireColdStarChannelPolicyReducesHeatWithoutMinimumCost(t *testing.T) {
	policy, ok := ForAction(StarfireColdStarChannel)
	if !ok {
		t.Fatal("expected cold star channel policy")
	}
	if policy.RequiredClass != classid.StarfireMage {
		t.Fatalf("RequiredClass=%q want=%q", policy.RequiredClass, classid.StarfireMage)
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
	if err := ValidateClass(StarfireColdStarChannel, classid.StarfireMage); err != nil {
		t.Fatal(err)
	}
	if err := ValidateClass(StarfireColdStarChannel, classid.Oathguard); !errors.Is(err, ErrWrongClass) {
		t.Fatalf("wrong class err=%v", err)
	}
}
