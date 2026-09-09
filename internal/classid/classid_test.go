package classid

import "testing"

func TestCanonicalClassIDsLocked(t *testing.T) {
	want := []ID{
		Oathguard,
		Breaker,
		Ranger,
		StarfireMage,
		Oathhealer,
		Shadowblade,
	}
	got := All()
	if len(got) != len(want) {
		t.Fatalf("All() len = %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("All()[%d] = %q, want %q", i, got[i], want[i])
		}
		if !IsCanonical(got[i]) {
			t.Fatalf("IsCanonical(%q) = false", got[i])
		}
	}

	if string(Oathguard) != "class_oathguard" ||
		string(Breaker) != "class_breaker" ||
		string(Ranger) != "class_ranger" ||
		string(StarfireMage) != "class_starfire_mage" ||
		string(Oathhealer) != "class_oathhealer" ||
		string(Shadowblade) != "class_shadowblade" {
		t.Fatal("canonical ClassID spelling changed")
	}
}

func TestUnassignedAndUnknownAreNotCanonical(t *testing.T) {
	for _, raw := range []string{
		"",
		"class_unassigned",
		"class_guard",
		"class_knight",
		" class_oathguard",
		"class_oathguard ",
		"CLASS_OATHGUARD",
	} {
		if _, ok := Parse(raw); ok {
			t.Fatalf("Parse(%q) unexpectedly accepted", raw)
		}
	}
}

func TestAllReturnsDefensiveCopy(t *testing.T) {
	first := All()
	first[0] = "corrupted"
	second := All()
	if second[0] != Oathguard {
		t.Fatalf("All() shared mutable backing storage: got %q", second[0])
	}
}
