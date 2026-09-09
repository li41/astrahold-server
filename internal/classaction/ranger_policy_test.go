package classaction

import (
	"errors"
	"testing"

	"github.com/li41/astrahold-server/internal/classid"
	"github.com/li41/astrahold-server/internal/classresource"
)

func TestRangerHuntingShotPolicy(t *testing.T) {
	policy, ok := ForAction(RangerHuntingShot)
	if !ok {
		t.Fatal("expected hunting shot policy")
	}
	if policy.RequiredClass != classid.Ranger {
		t.Fatalf("RequiredClass = %q, want %q", policy.RequiredClass, classid.Ranger)
	}
	if policy.HitResource != classresource.HuntMomentum || policy.HitGain != 8 {
		t.Fatalf("hit resource = %q +%d, want hunt_momentum +8", policy.HitResource, policy.HitGain)
	}
}

func TestRangerHuntingShotValidatesClass(t *testing.T) {
	if err := ValidateClass(RangerHuntingShot, classid.Ranger); err != nil {
		t.Fatalf("ranger rejected: %v", err)
	}
	if err := ValidateClass(RangerHuntingShot, classid.Oathguard); !errors.Is(err, ErrWrongClass) {
		t.Fatalf("oathguard hunting-shot error = %v, want ErrWrongClass", err)
	}
}
