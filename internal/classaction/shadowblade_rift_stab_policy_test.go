package classaction

import (
	"errors"
	"testing"

	"github.com/li41/astrahold-server/internal/classid"
	"github.com/li41/astrahold-server/internal/targetresource"
)

func TestShadowbladeRiftStabPolicy(t *testing.T) {
	policy, ok := ForAction(ShadowbladeRiftStab)
	if !ok {
		t.Fatal("expected Rift Stab policy")
	}
	if policy.RequiredClass != classid.Shadowblade {
		t.Fatalf("RequiredClass=%q want %q", policy.RequiredClass, classid.Shadowblade)
	}
	if policy.HitTargetResource != targetresource.Flaw || policy.HitTargetGain != 1 || policy.HitTargetMax != 3 {
		t.Fatalf("target resource policy=%+v", policy)
	}
	if !policy.RequireSideOrBack || !policy.HitTargetPreserveReadyTick {
		t.Fatalf("Rift Stab must require side/back and preserve existing build ReadyTick: %+v", policy)
	}
	if policy.HitTargetICDSeconds != 0 {
		t.Fatalf("Rift Stab must not install a target-resource build ICD: %v", policy.HitTargetICDSeconds)
	}
	if err := ValidateClass(ShadowbladeRiftStab, classid.Shadowblade); err != nil {
		t.Fatalf("Shadowblade rejected: %v", err)
	}
	if err := ValidateClass(ShadowbladeRiftStab, classid.Oathguard); !errors.Is(err, ErrWrongClass) {
		t.Fatalf("wrong-class error=%v want ErrWrongClass", err)
	}
}
