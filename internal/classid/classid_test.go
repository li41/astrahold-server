package classid

import "testing"

func TestCanonicalClassIDsLocked(t *testing.T) {
	want := []struct {
		id  ID
		raw string
	}{
		{Oathguard, "class_oathguard"},
		{Breaker, "class_breaker"},
		{Ranger, "class_ranger"},
		{StarfireMage, "class_starfire_mage"},
		{Oathhealer, "class_oathhealer"},
		{Shadowblade, "class_shadowblade"},
	}
	for _, tc := range want {
		if string(tc.id) != tc.raw {
			t.Fatalf("ClassID = %q, want %q", tc.id, tc.raw)
		}
		if !IsCanonical(tc.id) {
			t.Fatalf("IsCanonical(%q) = false", tc.id)
		}
		if got, ok := Parse(tc.raw); !ok || got != tc.id {
			t.Fatalf("Parse(%q) = (%q, %v), want (%q, true)", tc.raw, got, ok, tc.id)
		}
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
