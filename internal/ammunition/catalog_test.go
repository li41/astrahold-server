package ammunition

import (
	"testing"

	"github.com/li41/astrahold-server/internal/equipmentcatalog"
)

func TestV1ArrowDefinitionsAreWoodAndSilverOnly(t *testing.T) {
	definitions := Definitions()
	if len(definitions) != 2 {
		t.Fatalf("definitions=%d want 2", len(definitions))
	}
	want := []Definition{
		{ItemArchetypeID: ItemWoodArrow, Material: equipmentcatalog.MaterialWood, Damage: equipmentcatalog.DamageRange{Min: 6, Max: 8}},
		{ItemArchetypeID: ItemSilverArrow, Material: equipmentcatalog.MaterialSilver, Damage: equipmentcatalog.DamageRange{Min: 6, Max: 8}},
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
	for _, removed := range []string{"item_mid_arrow", "item_high_arrow", "item_starsteel_arrow"} {
		if _, ok := Resolve(removed); ok {
			t.Fatalf("removed arrow %q still resolves", removed)
		}
	}
}
