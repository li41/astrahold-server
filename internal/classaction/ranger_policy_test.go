package classaction

import (
	"testing"

	"github.com/li41/astrahold-server/internal/classresource"
)

func TestRangerHuntingShotPolicy(t *testing.T) {
	policy, ok := ForAction(RangerHuntingShot)
	if !ok {
		t.Fatal("expected hunting shot policy")
	}
	if policy.HitResource != classresource.HuntMomentum || policy.HitGain != 8 {
		t.Fatalf("hit resource = %q +%d, want hunt_momentum +8", policy.HitResource, policy.HitGain)
	}
}
