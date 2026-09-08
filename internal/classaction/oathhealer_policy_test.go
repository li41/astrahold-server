package classaction

import (
	"errors"
	"testing"

	"github.com/li41/astrahold-server/internal/classid"
	"github.com/li41/astrahold-server/internal/classresource"
)

func TestOathhealerOathlightStrikePolicy(t *testing.T) {
	policy, ok := ForAction(OathhealerOathlightStrike)
	if !ok {
		t.Fatal("expected oathlight strike policy")
	}
	if policy.RequiredClass != classid.Oathhealer {
		t.Fatalf("RequiredClass=%q, want %q", policy.RequiredClass, classid.Oathhealer)
	}
	if policy.HitProgressResource != classresource.OathSeal || policy.HitProgressGain != 20 {
		t.Fatalf("hit progress=%q +%d, want oath_seal +20 progress", policy.HitProgressResource, policy.HitProgressGain)
	}
	if policy.AcceptedResource != "" || policy.AcceptedGain != 0 || policy.HitResource != "" || policy.HitGain != 0 {
		t.Fatalf("oathlight strike unexpectedly has direct resource gain: %+v", policy)
	}
}

func TestOathhealerOathlightStrikeValidatesClass(t *testing.T) {
	if err := ValidateClass(OathhealerOathlightStrike, classid.Oathhealer); err != nil {
		t.Fatalf("oathhealer rejected: %v", err)
	}
	if err := ValidateClass(OathhealerOathlightStrike, classid.StarfireMage); !errors.Is(err, ErrWrongClass) {
		t.Fatalf("starfire oathlight error=%v, want ErrWrongClass", err)
	}
}
