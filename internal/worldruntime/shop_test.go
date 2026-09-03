package worldruntime

import (
	"errors"
	"testing"
	"time"

	"github.com/li41/astrahold-server/internal/inventory"
	"github.com/li41/astrahold-server/internal/protocol"
	shopcatalog "github.com/li41/astrahold-server/internal/shop"
	"github.com/li41/astrahold-server/internal/world"
)

func testShopCatalog(t *testing.T) *shopcatalog.Catalog {
	t.Helper()
	catalog, err := shopcatalog.New(shopcatalog.Definition{
		Revision: "shop-vs-001",
		Shops: []shopcatalog.Shop{{
			ID: "shop_emberwatch_warden_supplies",
			NPCArchetypeID: playtestNPCArchetypeID,
			Offers: []shopcatalog.Offer{{
				ID: "trade_gray_pelt_for_healing_potion",
				ItemArchetypeID: "item_minor_healing_potion",
				Quantity: 1,
				CostArchetypeID: grayWolfPeltArchetypeID,
				CostQuantity: 1,
			}},
		}},
	})
	if err != nil { t.Fatal(err) }
	return catalog
}

func TestOpenShopEmitsAuthoritativeSnapshot(t *testing.T) {
	runtime, sim, s, connection := newNPCTestRuntime(t, world.Position{})
	runtime.config.ShopCatalog = testShopCatalog(t)
	npcID := spawnTestNPC(t, sim, world.Position{X: 2})

	if err := runtime.EnqueueShopCommand(s.ID, 1, protocol.ClientShopCommand{Operation: protocol.ShopOperationOpen, NPCEntityID: npcID}); err != nil { t.Fatal(err) }
	report := runtime.Step(2, 50*time.Millisecond)
	if len(report.CommandErrors) != 0 { t.Fatalf("shop errors: %#v", report.CommandErrors) }

	select {
	case envelope := <-connection.Reliable():
		snapshot, ok := envelope.Message.(protocol.ShopSnapshot)
		if !ok { t.Fatalf("message = %T, want ShopSnapshot", envelope.Message) }
		if snapshot.NPCEntityID != npcID || snapshot.Revision != "shop-vs-001" || len(snapshot.Offers) != 1 { t.Fatalf("snapshot = %#v", snapshot) }
		offer := snapshot.Offers[0]
		if offer.OfferID != "trade_gray_pelt_for_healing_potion" || offer.ItemArchetypeID != "item_minor_healing_potion" || offer.CostArchetypeID != grayWolfPeltArchetypeID { t.Fatalf("offer = %#v", offer) }
	default:
		t.Fatal("expected authoritative ShopSnapshot")
	}
}

func TestBuyShopOfferExchangesInventoryAtomically(t *testing.T) {
	runtime, sim, s, connection := newNPCTestRuntime(t, world.Position{})
	runtime.config.ShopCatalog = testShopCatalog(t)
	npcID := spawnTestNPC(t, sim, world.Position{X: 2})
	inv := runtime.inventories[s.CharacterIdentity.ID]
	if inv == nil { t.Fatal("inventory missing") }
	if err := inv.Add(grayWolfPeltArchetypeID, 1); err != nil { t.Fatal(err) }
	if got := inv.Revision(); got != 4 { t.Fatalf("pre-buy revision = %d, want 4", got) }

	if err := runtime.EnqueueShopCommand(s.ID, 1, protocol.ClientShopCommand{Operation: protocol.ShopOperationBuy, NPCEntityID: npcID, OfferID: "trade_gray_pelt_for_healing_potion"}); err != nil { t.Fatal(err) }
	report := runtime.Step(2, 50*time.Millisecond)
	if len(report.CommandErrors) != 0 { t.Fatalf("buy errors: %#v", report.CommandErrors) }
	if got := inv.Quantity(grayWolfPeltArchetypeID); got != 0 { t.Fatalf("pelt quantity = %d, want 0", got) }
	if got := inv.Quantity("item_minor_healing_potion"); got != 6 { t.Fatalf("potion quantity = %d, want 6", got) }
	if got := inv.Revision(); got != 5 { t.Fatalf("post-buy revision = %d, want 5", got) }

	select {
	case envelope := <-connection.Reliable():
		snapshot, ok := envelope.Message.(protocol.InventorySnapshot)
		if !ok { t.Fatalf("message = %T, want InventorySnapshot", envelope.Message) }
		if snapshot.Revision != 5 { t.Fatalf("snapshot revision = %d, want 5", snapshot.Revision) }
	default:
		t.Fatal("expected authoritative InventorySnapshot after buy")
	}
}

func TestBuyShopOfferInsufficientCostLeavesInventoryUnchanged(t *testing.T) {
	runtime, sim, s, connection := newNPCTestRuntime(t, world.Position{})
	runtime.config.ShopCatalog = testShopCatalog(t)
	npcID := spawnTestNPC(t, sim, world.Position{X: 2})
	inv := runtime.inventories[s.CharacterIdentity.ID]
	before := inv.Revision()

	if err := runtime.EnqueueShopCommand(s.ID, 1, protocol.ClientShopCommand{Operation: protocol.ShopOperationBuy, NPCEntityID: npcID, OfferID: "trade_gray_pelt_for_healing_potion"}); err != nil { t.Fatal(err) }
	report := runtime.Step(2, 50*time.Millisecond)
	if len(report.CommandErrors) != 1 || !errors.Is(report.CommandErrors[0].Err, inventory.ErrInsufficient) { t.Fatalf("buy errors = %#v, want insufficient", report.CommandErrors) }
	if inv.Revision() != before || inv.Quantity("item_minor_healing_potion") != 5 { t.Fatalf("inventory mutated after rejected buy: rev=%d potion=%d", inv.Revision(), inv.Quantity("item_minor_healing_potion")) }
	select { case envelope := <-connection.Reliable(): t.Fatalf("unexpected reliable response: %#v", envelope); default: }
}

func TestOpenShopRejectsOutOfRangeWithoutSnapshot(t *testing.T) {
	runtime, sim, s, connection := newNPCTestRuntime(t, world.Position{})
	runtime.config.ShopCatalog = testShopCatalog(t)
	npcID := spawnTestNPC(t, sim, world.Position{X: 10})

	if err := runtime.EnqueueShopCommand(s.ID, 1, protocol.ClientShopCommand{Operation: protocol.ShopOperationOpen, NPCEntityID: npcID}); err != nil { t.Fatal(err) }
	report := runtime.Step(2, 50*time.Millisecond)
	if len(report.CommandErrors) != 1 || !errors.Is(report.CommandErrors[0].Err, ErrNPCOutOfRange) { t.Fatalf("shop errors = %#v, want out of range", report.CommandErrors) }
	select { case envelope := <-connection.Reliable(): t.Fatalf("unexpected ShopSnapshot: %#v", envelope); default: }
}
