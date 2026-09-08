package classaction

import (
	"errors"
	"testing"

	"github.com/li41/astrahold-server/internal/classid"
	"github.com/li41/astrahold-server/internal/classresource"
)

func TestStarfireFireBoltPolicy(t *testing.T) {
	policy, ok := ForAction(StarfireFireBolt)
	if !ok {
		t.Fatal("expected fire bolt policy")
	}
	if policy.RequiredClass != classid.StarfireMage {
		t.Fatalf("RequiredClass = %q, want %q", policy.RequiredClass, classid.StarfireMage)
	}
	if policy.AcceptedResource != classresource.StarHeat || policy.AcceptedGain != 8 {
		t.Fatalf("accepted resource = %q +%d, want star_heat +8", policy.AcceptedResource, policy.AcceptedGain)
	}
	if policy.HitResource != "" || policy.HitGain != 0 {
		t.Fatalf("fire bolt unexpectedly has hit-conditioned resource = %q +%d", policy.HitResource, policy.HitGain)
	}
}

func TestStarfireFireBoltValidatesClass(t *testing.T) {
	if err := ValidateClass(StarfireFireBolt, classid.StarfireMage); err != nil {
		t.Fatalf("starfire mage rejected: %v", err)
	}
	if err := ValidateClass(StarfireFireBolt, classid.Ranger); !errors.Is(err, ErrWrongClass) {
		t.Fatalf("ranger fire-bolt error = %v, want ErrWrongClass", err)
	}
}
