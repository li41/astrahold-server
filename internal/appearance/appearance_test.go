package appearance

import (
	"testing"

	"github.com/li41/astrahold-server/internal/equipmentcatalog"
	"github.com/li41/astrahold-server/internal/world"
)

func TestFormalSkinWeaponAffinities(t *testing.T) {
	want := map[SkinID]equipmentcatalog.WeaponType{
		KnightDPelegrini: "one_hand_sword",
		Ninja:            "dagger",
		PeasantGirl:      "one_hand_axe",
		CastleGuard01:    "one_hand_spear",
		PeasantMan:       "warhammer",
		Ortiz:            "morning_star",
		Drake:            "mace",
		DreyarByMAure:    "two_hand_sword",
		Brute:            "two_hand_axe",
		CastleGuard02:    "two_hand_spear",
		TheBoss:          "knuckles",
		GanfaulMAure:     "claw",
		KachujinGRosales: "dual_blades",
		ErikaArcher:      "bow",
		Swat:             "crossbow",
		Jackie:           "sling",
		Arissa:           "staff",
	}
	if len(definitions) != len(want) {
		t.Fatalf("skin definition count=%d want=%d", len(definitions), len(want))
	}
	for id, affinity := range want {
		got, ok := WeaponAffinity(id)
		if !ok || got != affinity {
			t.Fatalf("skin=%q affinity=%q ok=%v want=%q", id, got, ok, affinity)
		}
	}
	if ValidSelection("skin_unknown") {
		t.Fatal("unknown skin accepted")
	}
	if !ValidSelection(None) {
		t.Fatal("empty skin must remain a valid no-bonus selection")
	}
}

func TestStoreFailsClosedAndClearsEntity(t *testing.T) {
	var store Store
	entityID := world.EntityID(7)
	if err := store.Set(entityID, PeasantGirl); err != nil {
		t.Fatal(err)
	}
	if got := store.Get(entityID); got != PeasantGirl {
		t.Fatalf("skin=%q want=%q", got, PeasantGirl)
	}
	if err := store.Set(entityID, "skin_unknown"); err == nil {
		t.Fatal("unknown skin accepted")
	}
	if got := store.Get(entityID); got != PeasantGirl {
		t.Fatalf("invalid replacement changed state: %q", got)
	}
	store.ClearEntity(entityID)
	if got := store.Get(entityID); got != None {
		t.Fatalf("skin after clear=%q", got)
	}
}
