package actionpolicy

import (
	"testing"

	"github.com/li41/astrahold-server/internal/actionresource"
	"github.com/li41/astrahold-server/internal/targetresource"
)

func TestOathguardSwordStrikePolicy(t *testing.T) {
	policy, ok := ForAction(OathguardSwordStrike)
	if !ok { t.Fatal("expected sword strike policy") }
	if policy.HitResource != actionresource.Resolve || policy.HitGain != 8 {
		t.Fatalf("hit resource = %q +%d, want resolve +8", policy.HitResource, policy.HitGain)
	}
}

func TestOathguardFortifyPolicy(t *testing.T) {
	policy, ok := ForAction(OathguardFortify)
	if !ok { t.Fatal("expected Fortify policy") }
	if policy.CostResource != actionresource.Resolve || policy.CostAmount != 30 {
		t.Fatalf("cost=%q %d want resolve 30", policy.CostResource, policy.CostAmount)
	}
	if policy.AcceptedResource != actionresource.Empty || policy.HitResource != actionresource.Empty || policy.HitGain != 0 {
		t.Fatalf("unexpected resource gain policy=%+v", policy)
	}
}

func TestBreakerHeavySlashPolicy(t *testing.T) {
	policy, ok := ForAction(BreakerHeavySlash)
	if !ok { t.Fatal("expected heavy slash policy") }
	if policy.HitResource != actionresource.Momentum || policy.HitGain != 10 {
		t.Fatalf("hit resource = %q +%d, want momentum +10", policy.HitResource, policy.HitGain)
	}
	if policy.CostResource != actionresource.Empty || policy.CostAmount != 0 {
		t.Fatalf("unexpected heavy slash cost = %q %d", policy.CostResource, policy.CostAmount)
	}
}

func TestBreakerStaggerStrikePolicy(t *testing.T) {
	policy, ok := ForAction(BreakerStaggerStrike)
	if !ok { t.Fatal("expected stagger strike policy") }
	if policy.CostResource != actionresource.Momentum || policy.CostAmount != 20 {
		t.Fatalf("cost = %q %d, want momentum 20", policy.CostResource, policy.CostAmount)
	}
	if policy.AcceptedResource != actionresource.Empty || policy.HitResource != actionresource.Empty || policy.HitProgressResource != actionresource.Empty {
		t.Fatalf("unexpected stagger strike resource gain policy = %+v", policy)
	}
}

func TestRangerHuntingShotPolicy(t *testing.T) {
	policy, ok := ForAction(RangerHuntingShot)
	if !ok { t.Fatal("expected hunting shot policy") }
	if policy.HitResource != actionresource.HuntMomentum || policy.HitGain != 8 {
		t.Fatalf("hit resource = %q +%d, want hunt_momentum +8", policy.HitResource, policy.HitGain)
	}
}

func TestRangerArmorPiercingArrowPolicy(t *testing.T) {
	policy, ok := ForAction(RangerArmorPiercingArrow)
	if !ok { t.Fatal("expected armor piercing arrow policy") }
	if policy.CostResource != actionresource.HuntMomentum || policy.CostAmount != 30 {
		t.Fatalf("cost=%q %d want hunt_momentum 30", policy.CostResource, policy.CostAmount)
	}
	if policy.HitResource != actionresource.Empty || policy.HitGain != 0 {
		t.Fatalf("unexpected hit gain=%q +%d", policy.HitResource, policy.HitGain)
	}
}

func TestStarfireFireBoltPolicy(t *testing.T) {
	policy, ok := ForAction(StarfireFireBolt)
	if !ok { t.Fatal("expected fire bolt policy") }
	if policy.AcceptedResource != actionresource.StarHeat || policy.AcceptedGain != 8 {
		t.Fatalf("accepted resource = %q +%d, want star_heat +8", policy.AcceptedResource, policy.AcceptedGain)
	}
	if policy.HitResource != actionresource.Empty || policy.HitGain != 0 {
		t.Fatalf("fire bolt unexpectedly has hit-conditioned resource = %q +%d", policy.HitResource, policy.HitGain)
	}
}

func TestStarfireColdStarChannelPolicyReducesHeatWithoutMinimumCost(t *testing.T) {
	policy, ok := ForAction(StarfireColdStarChannel)
	if !ok { t.Fatal("expected cold star channel policy") }
	if policy.AcceptedReductionResource != actionresource.StarHeat || policy.AcceptedReductionAmount != 45 {
		t.Fatalf("accepted reduction=%q %d want star_heat 45", policy.AcceptedReductionResource, policy.AcceptedReductionAmount)
	}
	if policy.CostResource != actionresource.Empty || policy.CostAmount != 0 {
		t.Fatalf("cold star channel must not require minimum heat as a cost: %+v", policy)
	}
	if policy.AcceptedResource != actionresource.Empty || policy.AcceptedGain != 0 {
		t.Fatalf("unexpected accepted gain=%q +%d", policy.AcceptedResource, policy.AcceptedGain)
	}
}

func TestOathhealerOathlightStrikePolicy(t *testing.T) {
	policy, ok := ForAction(OathhealerOathlightStrike)
	if !ok { t.Fatal("expected oathlight strike policy") }
	if policy.HitProgressResource != actionresource.OathSeal || policy.HitProgressGain != 20 {
		t.Fatalf("hit progress=%q +%d, want oath_seal +20 progress", policy.HitProgressResource, policy.HitProgressGain)
	}
	if policy.AcceptedResource != actionresource.Empty || policy.AcceptedGain != 0 || policy.HitResource != actionresource.Empty || policy.HitGain != 0 {
		t.Fatalf("oathlight strike unexpectedly has direct resource gain: %+v", policy)
	}
}

func TestShadowbladeDualBladePolicy(t *testing.T) {
	policy, ok := ForAction(ShadowbladeDualBladeStrike)
	if !ok { t.Fatal("missing Shadowblade dual blade policy") }
	if policy.HitTargetResource != targetresource.Flaw || policy.HitTargetGain != 1 || policy.HitTargetMax != 3 || policy.HitTargetICDSeconds != 2.5 || !policy.RequireSideOrBack {
		t.Fatalf("policy=%+v", policy)
	}
	if policy.HitResource != actionresource.Empty || policy.AcceptedResource != actionresource.Empty || policy.HitProgressResource != actionresource.Empty || policy.TargetSpend.ResourceID != "" {
		t.Fatalf("unexpected global/spend resource policy=%+v", policy)
	}
}

func TestShadowbladeRiftStabPolicy(t *testing.T) {
	policy, ok := ForAction(ShadowbladeRiftStab)
	if !ok { t.Fatal("expected Rift Stab policy") }
	if policy.HitTargetResource != targetresource.Flaw || policy.HitTargetGain != 1 || policy.HitTargetMax != 3 {
		t.Fatalf("target resource policy=%+v", policy)
	}
	if !policy.RequireSideOrBack || !policy.HitTargetPreserveReadyTick {
		t.Fatalf("Rift Stab must require side/back and preserve existing build ReadyTick: %+v", policy)
	}
	if policy.HitTargetICDSeconds != 0 {
		t.Fatalf("Rift Stab must not install a target-resource build ICD: %v", policy.HitTargetICDSeconds)
	}
}

func TestShadowbladeFlawExecutePolicyUsesHighestAvailableCanonicalTier(t *testing.T) {
	policy, ok := ForAction(ShadowbladeFlawExecute)
	if !ok { t.Fatal("missing Shadowblade Flaw Execute policy") }
	if policy.TargetSpend.ResourceID != targetresource.Flaw { t.Fatalf("policy=%+v", policy) }
	for _, tc := range []struct {
		current uint32
		amount  uint32
		damage  uint32
		ok      bool
	}{
		{current: 0, ok: false},
		{current: 1, amount: 1, damage: 140, ok: true},
		{current: 2, amount: 2, damage: 220, ok: true},
		{current: 3, amount: 3, damage: 300, ok: true},
	} {
		amount, damage, resolved := policy.TargetSpend.Resolve(tc.current)
		if amount != tc.amount || damage != tc.damage || resolved != tc.ok {
			t.Fatalf("current=%d amount=%d damage=%d ok=%v", tc.current, amount, damage, resolved)
		}
	}
}
