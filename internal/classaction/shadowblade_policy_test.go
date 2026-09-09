package classaction

import (
	"errors"
	"testing"

	"github.com/li41/astrahold-server/internal/classid"
	"github.com/li41/astrahold-server/internal/targetresource"
)

func TestShadowbladeDualBladePolicy(t *testing.T) {
	policy, ok := ForAction(ShadowbladeDualBladeStrike)
	if !ok { t.Fatal("missing Shadowblade dual blade policy") }
	if policy.RequiredClass != classid.Shadowblade || policy.HitTargetResource != targetresource.Flaw || policy.HitTargetGain != 1 || policy.HitTargetMax != 3 || policy.HitTargetICDSeconds != 2.5 || !policy.RequireSideOrBack {
		t.Fatalf("policy=%+v", policy)
	}
	if policy.HitResource != "" || policy.AcceptedResource != "" || policy.HitProgressResource != "" || policy.TargetSpend.ResourceID != "" { t.Fatalf("unexpected global/spend resource policy=%+v", policy) }
	if err := ValidateClass(ShadowbladeDualBladeStrike, classid.Shadowblade); err != nil { t.Fatal(err) }
	if err := ValidateClass(ShadowbladeDualBladeStrike, classid.Oathguard); !errors.Is(err, ErrWrongClass) { t.Fatalf("wrong class err=%v", err) }
}

func TestShadowbladeFlawExecutePolicyUsesHighestAvailableCanonicalTier(t *testing.T) {
	policy, ok := ForAction(ShadowbladeFlawExecute)
	if !ok { t.Fatal("missing Shadowblade Flaw Execute policy") }
	if policy.RequiredClass != classid.Shadowblade || policy.TargetSpend.ResourceID != targetresource.Flaw { t.Fatalf("policy=%+v", policy) }
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
		if amount != tc.amount || damage != tc.damage || resolved != tc.ok { t.Fatalf("current=%d amount=%d damage=%d ok=%v", tc.current, amount, damage, resolved) }
	}
	if err := ValidateClass(ShadowbladeFlawExecute, classid.Shadowblade); err != nil { t.Fatal(err) }
	if err := ValidateClass(ShadowbladeFlawExecute, classid.Oathguard); !errors.Is(err, ErrWrongClass) { t.Fatalf("wrong class err=%v", err) }
}
