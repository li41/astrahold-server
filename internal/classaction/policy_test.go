package classaction

import (
	"errors"
	"testing"

	"github.com/li41/astrahold-server/internal/classid"
	"github.com/li41/astrahold-server/internal/classresource"
)

func TestOathguardSwordStrikePolicy(t *testing.T) {
	policy, ok := ForAction(OathguardSwordStrike)
	if !ok {
		t.Fatal("expected sword strike policy")
	}
	if policy.RequiredClass != classid.Oathguard {
		t.Fatalf("RequiredClass = %q, want %q", policy.RequiredClass, classid.Oathguard)
	}
	if policy.HitResource != classresource.Resolve || policy.HitGain != 8 {
		t.Fatalf("hit resource = %q +%d, want resolve +8", policy.HitResource, policy.HitGain)
	}
}

func TestValidateClass(t *testing.T) {
	if err := ValidateClass(OathguardSwordStrike, classid.Oathguard); err != nil {
		t.Fatalf("oathguard rejected: %v", err)
	}
	if err := ValidateClass(OathguardSwordStrike, classid.Ranger); !errors.Is(err, ErrWrongClass) {
		t.Fatalf("ranger error = %v, want ErrWrongClass", err)
	}
	if err := ValidateClass("basic-attack", ""); err != nil {
		t.Fatalf("unscoped action rejected: %v", err)
	}
}
