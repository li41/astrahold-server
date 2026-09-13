package classaction

import (
	"testing"

	"github.com/li41/astrahold-server/internal/classresource"
)

func TestOathhealerOathlightStrikePolicy(t *testing.T) {
	policy, ok := ForAction(OathhealerOathlightStrike)
	if !ok {
		t.Fatal("expected oathlight strike policy")
	}
	if policy.HitProgressResource != classresource.OathSeal || policy.HitProgressGain != 20 {
		t.Fatalf("hit progress=%q +%d, want oath_seal +20 progress", policy.HitProgressResource, policy.HitProgressGain)
	}
	if policy.AcceptedResource != "" || policy.AcceptedGain != 0 || policy.HitResource != "" || policy.HitGain != 0 {
		t.Fatalf("oathlight strike unexpectedly has direct resource gain: %+v", policy)
	}
}
