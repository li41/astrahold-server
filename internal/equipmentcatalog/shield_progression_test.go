package equipmentcatalog

import "testing"

func TestDefaultCatalogLocksThreeTierShieldProgression(t *testing.T) {
	catalog, err := Default()
	if err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		id                                    string
		tier                                  Tier
		weight                                uint32
		material                              MaterialID
		physical                              uint32
		block, blockReduction, magicReduction uint8
	}{
		{"item_iron_rim_round_shield", TierLow, 6, MaterialWood, 3, 8, 25, 0},
		{"item_mid_iron_rim_round_shield", TierMid, 6, MaterialSteel, 5, 10, 30, 0},
		{"item_high_iron_rim_round_shield", TierHigh, 6, MaterialStarsteel, 7, 12, 35, 0},
		{"item_guard_shield", TierLow, 8, MaterialIron, 4, 10, 30, 0},
		{"item_mid_guard_shield", TierMid, 8, MaterialSteel, 6, 13, 35, 0},
		{"item_high_guard_shield", TierHigh, 8, MaterialStarsteel, 8, 16, 40, 0},
		{"item_runed_square_shield", TierLow, 6, MaterialRunewood, 2, 6, 20, 8},
		{"item_mid_runed_square_shield", TierMid, 6, MaterialSilver, 3, 8, 25, 12},
		{"item_high_runed_square_shield", TierHigh, 6, MaterialStarsteel, 4, 10, 30, 16},
	}

	if got := len(catalog.byItem); got != 62 {
		t.Fatalf("catalog item count=%d want=62", got)
	}
	shieldCount := 0
	for _, item := range catalog.byItem {
		if item.Kind == KindShield {
			shieldCount++
		}
	}
	if shieldCount != 9 {
		t.Fatalf("shield count=%d want=9", shieldCount)
	}

	for _, tc := range cases {
		item, ok := catalog.Resolve(tc.id)
		if !ok {
			t.Fatalf("missing shield %s", tc.id)
		}
		if item.Kind != KindShield || item.Slot != SlotOffHand || item.Tier != tc.tier || item.Shield == nil {
			t.Fatalf("%s invalid shield definition: %#v", tc.id, item)
		}
		if item.Weight != tc.weight || item.Material != tc.material {
			t.Fatalf("%s weight/material=%d/%q want=%d/%q", tc.id, item.Weight, item.Material, tc.weight, tc.material)
		}
		if item.Shield.PhysicalDefense != tc.physical ||
			item.Shield.BlockChancePercent != tc.block ||
			item.Shield.BlockDamageReductionPercent != tc.blockReduction ||
			item.Shield.MagicDamageReductionPercent != tc.magicReduction {
			t.Fatalf("%s shield values=%#v", tc.id, item.Shield)
		}
	}
}
