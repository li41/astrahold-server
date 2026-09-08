package classaction

import (
	"errors"
	"testing"

	"github.com/li41/astrahold-server/internal/classid"
	"github.com/li41/astrahold-server/internal/classresource"
)

func TestOathguardFortifyPolicy(t *testing.T) {
	policy, ok := ForAction(OathguardFortify)
	if !ok {
		t.Fatal("expected Fortify policy")
	}
	if policy.RequiredClass != classid.Oathguard {
		t.Fatalf("RequiredClass=%q want=%q", policy.RequiredClass, classid.Oathguard)
	}
	if policy.CostResource != classresource.Resolve || policy.CostAmount != 30 {
		t.Fatalf("cost=%q %d want resolve 30", policy.CostResource, policy.CostAmount)
	}
	if policy.AcceptedResource != classresource.Empty || policy.HitResource != classresource.Empty || policy.HitGain != 0 {
		t.Fatalf("unexpected resource gain policy=%+v", policy)
	}
	if err := ValidateClass(OathguardFortify, classid.Oathguard); err != nil {
		t.Fatal(err)
	}
	if err := ValidateClass(OathguardFortify, classid.Ranger); !errors.Is(err, ErrWrongClass) {
		t.Fatalf("wrong class err=%v", err)
	}
}
