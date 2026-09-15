package ammunition

import (
	"testing"

	"github.com/li41/astrahold-server/internal/equipmentcatalog"
)

func TestV1ArrowDefinitions(t *testing.T) {
	definitions := Definitions()
	if len(definitions) != 3 {
		t.Fatalf("definitions=%d want 3", len(definitions))
	}
	want := []Definition{
		{ItemArchetypeID: ItemLowArrow, Tier: equipmentcatalog.TierLow, Material: equipmentcatalog.MaterialWood, Damage: equipmentcatalog.DamageRange{Min: 6, Max: 8}},
		{ItemArchetypeID: ItemMidArrow, Tier: equipmentcatalog.TierMid, Material: equipmentcatalog.MaterialSteel, Damage: equipmentcatalog.DamageRange{Min: 8, Max: 11}},
		{ItemArchetypeID: ItemHighArrow, Tier: equipmentcatalog.TierHigh, Material: equipmentcatalog.MaterialStarsteel, Damage: equipmentcatalog.DamageRange{Min: 10, Max: 15}},
	}
	for i, expected := range want {
		got := definitions[i]
		if got != expected {
			t.Fatalf("definition[%d]=%+v want %+v", i, got, expected)
		}
		resolved, ok := Resolve(expected.ItemArchetypeID)
		if !ok || resolved != expected {
			t.Fatalf("resolve %q=%+v ok=%v want %+v", expected.ItemArchetypeID, resolved, ok, expected)
		}
		if got.UnitWeight != 0 {
			t.Fatalf("arrow %q unit weight=%d want 0", got.ItemArchetypeID, got.UnitWeight)
		}
	}
}
