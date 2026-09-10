package classaction

import (
	"testing"

	"github.com/li41/astrahold-server/internal/classresource"
)

func TestOathguardSwordStrikePolicy(t *testing.T) {
	policy, ok := ForAction(OathguardSwordStrike)
	if !ok {
		t.Fatal("expected sword strike policy")
	}
	if policy.HitResource != classresource.Resolve || policy.HitGain != 8 {
		t.Fatalf("hit resource = %q +%d, want resolve +8", policy.HitResource, policy.HitGain)
	}
}

func TestBreakerHeavySlashPolicy(t *testing.T) {
	policy, ok := ForAction(BreakerHeavySlash)
	if !ok {
		t.Fatal("expected heavy slash policy")
	}
	if policy.HitResource != classresource.Momentum || policy.HitGain != 10 {
		t.Fatalf("hit resource = %q +%d, want momentum +10", policy.HitResource, policy.HitGain)
	}
	if policy.CostResource != classresource.Empty || policy.CostAmount != 0 {
		t.Fatalf("unexpected heavy slash cost = %q %d", policy.CostResource, policy.CostAmount)
	}
}

func TestBreakerStaggerStrikePolicy(t *testing.T) {
	policy, ok := ForAction(BreakerStaggerStrike)
	if !ok {
		t.Fatal("expected stagger strike policy")
	}
	if policy.CostResource != classresource.Momentum || policy.CostAmount != 20 {
		t.Fatalf("cost = %q %d, want momentum 20", policy.CostResource, policy.CostAmount)
	}
	if policy.AcceptedResource != classresource.Empty || policy.HitResource != classresource.Empty || policy.HitProgressResource != classresource.Empty {
		t.Fatalf("unexpected stagger strike resource gain policy = %+v", policy)
	}
}
