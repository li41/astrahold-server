package protocol

import "github.com/li41/astrahold-server/internal/world"

const (
	MessageClientShopCommand MessageType = 6
	MessageShopSnapshot      MessageType = 113
)

type ShopOperation string

const (
	ShopOperationOpen ShopOperation = "open"
	ShopOperationBuy  ShopOperation = "buy"
)

// ClientShopCommand is intent only. Item identities, quantities and costs are resolved from the Server catalog.
type ClientShopCommand struct {
	Operation   ShopOperation
	NPCEntityID world.EntityID
	OfferID     string
}

func (ClientShopCommand) Type() MessageType { return MessageClientShopCommand }

type ShopOffer struct {
	OfferID         string
	ItemArchetypeID string
	Quantity        uint32
	CostArchetypeID string
	CostQuantity    uint32
}

// ShopSnapshot is source-session-only authoritative vendor presentation.
type ShopSnapshot struct {
	Revision    string
	NPCEntityID world.EntityID
	ShopID      string
	Offers      []ShopOffer
}

func (ShopSnapshot) Type() MessageType { return MessageShopSnapshot }
