package shop

import "testing"

func TestCatalogResolvesVendorOffer(t *testing.T) {
	catalog, err := New(Definition{
		Revision: "shop-vs-001",
		Shops: []Shop{{
			ID: "shop_emberwatch_warden_supplies",
			NPCArchetypeID: "npc_emberwatch_warden",
			Offers: []Offer{{
				ID: "trade_gray_pelt_for_healing_potion",
				ItemArchetypeID: "item_minor_healing_potion",
				Quantity: 1,
				CostArchetypeID: "item_gray_wolf_pelt",
				CostQuantity: 1,
			}},
		}},
	})
	if err != nil { t.Fatal(err) }
	entry, ok := catalog.ShopForNPC("npc_emberwatch_warden")
	if !ok { t.Fatal("shop not found") }
	offer, ok := FindOffer(entry, "trade_gray_pelt_for_healing_potion")
	if !ok { t.Fatal("offer not found") }
	if offer.ItemArchetypeID != "item_minor_healing_potion" || offer.CostArchetypeID != "item_gray_wolf_pelt" {
		t.Fatalf("unexpected offer: %#v", offer)
	}
}

func TestCatalogRejectsDuplicateNPCShop(t *testing.T) {
	_, err := New(Definition{
		Revision: "r1",
		Shops: []Shop{
			{ID: "a", NPCArchetypeID: "npc", Offers: []Offer{{ID: "a1", ItemArchetypeID: "item_a", Quantity: 1, CostArchetypeID: "item_b", CostQuantity: 1}}},
			{ID: "b", NPCArchetypeID: "npc", Offers: []Offer{{ID: "b1", ItemArchetypeID: "item_a", Quantity: 1, CostArchetypeID: "item_b", CostQuantity: 1}}},
		},
	})
	if err == nil { t.Fatal("expected duplicate NPC shop rejection") }
}
