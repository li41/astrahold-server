package shop

import "testing"

func TestDefaultCatalogOffersManaSiphonStaffForGrayPelts(t *testing.T) {
	catalog, err := Default()
	if err != nil {
		t.Fatal(err)
	}
	if catalog.Revision() != "shop-vs-002" {
		t.Fatalf("revision = %q, want shop-vs-002", catalog.Revision())
	}
	entry, ok := catalog.ShopForNPC("npc_emberwatch_warden")
	if !ok {
		t.Fatal("emberwatch warden shop not found")
	}
	offer, ok := FindOffer(entry, "trade_gray_pelts_for_mana_siphon_staff")
	if !ok {
		t.Fatal("mana siphon staff offer not found")
	}
	if offer.ItemArchetypeID != "item_mana_siphon_staff" || offer.Quantity != 1 || offer.CostArchetypeID != "item_gray_wolf_pelt" || offer.CostQuantity != 3 {
		t.Fatalf("unexpected staff offer: %#v", offer)
	}
}
