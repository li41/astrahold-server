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
	if policy.HitResource != "" || policy.AcceptedResource != "" || policy.HitProgressResource != "" { t.Fatalf("unexpected global resource policy=%+v", policy) }
	if err := ValidateClass(ShadowbladeDualBladeStrike, classid.Shadowblade); err != nil { t.Fatal(err) }
	if err := ValidateClass(ShadowbladeDualBladeStrike, classid.Oathguard); !errors.Is(err, ErrWrongClass) { t.Fatalf("wrong class err=%v", err) }
}
