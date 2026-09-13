package classaction

import (
	"testing"

	"github.com/li41/astrahold-server/internal/classresource"
)

func TestStarfireFireBoltPolicy(t *testing.T) {
	policy, ok := ForAction(StarfireFireBolt)
	if !ok {
		t.Fatal("expected fire bolt policy")
	}
	if policy.AcceptedResource != classresource.StarHeat || policy.AcceptedGain != 8 {
		t.Fatalf("accepted resource = %q +%d, want star_heat +8", policy.AcceptedResource, policy.AcceptedGain)
	}
	if policy.HitResource != "" || policy.HitGain != 0 {
		t.Fatalf("fire bolt unexpectedly has hit-conditioned resource = %q +%d", policy.HitResource, policy.HitGain)
	}
}
