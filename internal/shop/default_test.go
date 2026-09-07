package shop

import "testing"

func TestDefaultCatalogDoesNotPublishManaSiphonStaffAcquisition(t *testing.T) {
	catalog, err := Default()
	if err != nil {
		t.Fatal(err)
	}
	if catalog.Revision() != "shop-vs-001" {
		t.Fatalf("revision = %q, want shop-vs-001", catalog.Revision())
	}
	entry, ok := catalog.ShopForNPC("npc_emberwatch_warden")
	if !ok {
		t.Fatal("emberwatch warden shop not found")
	}
	if _, ok := FindOffer(entry, "trade_gray_pelts_for_mana_siphon_staff"); ok {
		t.Fatal("mana siphon staff acquisition must remain undefined until gameplay-content discussion")
	}
}
