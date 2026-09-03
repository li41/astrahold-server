package jsonv1

import (
	"reflect"
	"testing"

	"github.com/li41/astrahold-server/internal/protocol"
	"github.com/li41/astrahold-server/internal/world"
)

func TestClientShopCommandRoundTrip(t *testing.T) {
	codec := Codec{}
	for _, want := range []protocol.ClientShopCommand{
		{Operation: protocol.ShopOperationOpen, NPCEntityID: 7001},
		{Operation: protocol.ShopOperationBuy, NPCEntityID: 7001, OfferID: "trade_gray_pelt_for_healing_potion"},
	} {
		data, err := codec.Marshal(want)
		if err != nil { t.Fatal(err) }
		decoded, err := codec.Unmarshal(protocol.MessageClientShopCommand, data)
		if err != nil { t.Fatal(err) }
		got := decoded.(protocol.ClientShopCommand)
		if !reflect.DeepEqual(got, want) { t.Fatalf("got %#v, want %#v", got, want) }
	}
}

func TestShopSnapshotRoundTrip(t *testing.T) {
	codec := Codec{}
	want := protocol.ShopSnapshot{
		Revision: "shop-vs-001",
		NPCEntityID: world.EntityID(7001),
		ShopID: "shop_emberwatch_warden_supplies",
		Offers: []protocol.ShopOffer{{
			OfferID: "trade_gray_pelt_for_healing_potion",
			ItemArchetypeID: "item_minor_healing_potion",
			Quantity: 1,
			CostArchetypeID: "item_gray_wolf_pelt",
			CostQuantity: 1,
		}},
	}
	data, err := codec.Marshal(want)
	if err != nil { t.Fatal(err) }
	decoded, err := codec.Unmarshal(protocol.MessageShopSnapshot, data)
	if err != nil { t.Fatal(err) }
	got := decoded.(protocol.ShopSnapshot)
	if !reflect.DeepEqual(got, want) { t.Fatalf("got %#v, want %#v", got, want) }
}

func TestClientShopCommandStrictDecodeRejectsUnknownFields(t *testing.T) {
	codec := Codec{}
	_, err := codec.Unmarshal(protocol.MessageClientShopCommand, []byte(`{"operation":"open","npc_entity_id":7001,"client_price":1}`))
	if err == nil { t.Fatal("expected unknown-field rejection") }
}
