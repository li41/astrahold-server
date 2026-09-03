package worldruntime

import (
	"errors"
	"strings"

	"github.com/li41/astrahold-server/internal/character"
	"github.com/li41/astrahold-server/internal/inventory"
	"github.com/li41/astrahold-server/internal/protocol"
	"github.com/li41/astrahold-server/internal/session"
	"github.com/li41/astrahold-server/internal/shop"
	"github.com/li41/astrahold-server/internal/world"
)

var (
	ErrInvalidShopIntent = errors.New("worldruntime: invalid shop intent")
	ErrShopUnavailable   = errors.New("worldruntime: shop catalog unavailable")
	ErrShopNotFound      = errors.New("worldruntime: shop not found for NPC")
	ErrShopOfferNotFound = errors.New("worldruntime: shop offer not found")
)

func (r *Runtime) EnqueueShopCommand(id session.ID, sequence uint32, intent protocol.ClientShopCommand) error {
	if id == 0 || sequence == 0 || !validShopIntent(intent) {
		return ErrInvalidShopIntent
	}
	return r.queue.tryPush(shopCommand{sessionID: id, sequence: sequence, intent: intent})
}

func (r *Runtime) EnqueueFencedShopCommand(ownership SessionOwnershipFence, sequence uint32, intent protocol.ClientShopCommand) error {
	if !ownership.Valid() || sequence == 0 || !validShopIntent(intent) {
		return ErrInvalidShopIntent
	}
	return r.queue.tryPush(shopCommand{sessionID: ownership.SessionID, sequence: sequence, intent: intent, ownership: ownership})
}

func validShopIntent(intent protocol.ClientShopCommand) bool {
	if intent.NPCEntityID == 0 {
		return false
	}
	switch intent.Operation {
	case protocol.ShopOperationOpen:
		return strings.TrimSpace(intent.OfferID) == ""
	case protocol.ShopOperationBuy:
		return strings.TrimSpace(intent.OfferID) != ""
	default:
		return false
	}
}

func (r *Runtime) authoritativeShopCatalog() (*shop.Catalog, error) {
	if r.config.ShopCatalog != nil {
		return r.config.ShopCatalog, nil
	}
	catalog, err := shop.Default()
	if err != nil {
		return nil, ErrShopUnavailable
	}
	// This function runs only on the single world owner. Cache the immutable catalog after first use.
	r.config.ShopCatalog = catalog
	return catalog, nil
}

func (r *Runtime) applyShopCommand(name string, command shopCommand, tick uint64, report *StepReport) {
	if command.ownership.Valid() {
		if err := r.characterIdentities.validateOwnership(command.sessionID, command.ownership); err != nil {
			report.CommandErrors = append(report.CommandErrors, CommandError{Command: name, SessionID: command.sessionID, Err: err})
			return
		}
	}

	s, ok := r.sessions.Get(command.sessionID)
	if !ok {
		report.CommandErrors = append(report.CommandErrors, CommandError{Command: name, SessionID: command.sessionID, Err: session.ErrSessionNotFound})
		return
	}
	if err := s.ValidateActionSequence(command.sequence); err != nil {
		report.CommandErrors = append(report.CommandErrors, CommandError{Command: name, SessionID: command.sessionID, Err: err})
		return
	}
	s.MarkProcessedAction(command.sequence)

	player, ok := r.world.Entity(s.EntityID)
	if !ok || player.Kind != world.EntityPlayer {
		report.CommandErrors = append(report.CommandErrors, CommandError{Command: name, SessionID: command.sessionID, Err: ErrSessionEntityNotFound})
		return
	}
	state, ok := r.characters.State(s.EntityID)
	if !ok {
		report.CommandErrors = append(report.CommandErrors, CommandError{Command: name, SessionID: command.sessionID, Err: character.ErrCharacterNotFound})
		return
	}
	if state.Defeated {
		report.CommandErrors = append(report.CommandErrors, CommandError{Command: name, SessionID: command.sessionID, Err: character.ErrCharacterDefeated})
		return
	}

	npc, ok := r.world.Entity(command.intent.NPCEntityID)
	if !ok || npc.Kind != world.EntityNPC {
		report.CommandErrors = append(report.CommandErrors, CommandError{Command: name, SessionID: command.sessionID, Err: ErrNPCNotFound})
		return
	}
	if player.Transform.Position.Layer != npc.Transform.Position.Layer {
		report.CommandErrors = append(report.CommandErrors, CommandError{Command: name, SessionID: command.sessionID, Err: ErrNPCWrongLayer})
		return
	}
	if player.Transform.Position.DistanceSquared(npc.Transform.Position) > npcInteractionRangeMeters*npcInteractionRangeMeters {
		report.CommandErrors = append(report.CommandErrors, CommandError{Command: name, SessionID: command.sessionID, Err: ErrNPCOutOfRange})
		return
	}
	catalog, err := r.authoritativeShopCatalog()
	if err != nil {
		report.CommandErrors = append(report.CommandErrors, CommandError{Command: name, SessionID: command.sessionID, Err: err})
		return
	}
	catalogShop, ok := catalog.ShopForNPC(npc.ArchetypeID)
	if !ok {
		report.CommandErrors = append(report.CommandErrors, CommandError{Command: name, SessionID: command.sessionID, Err: ErrShopNotFound})
		return
	}

	switch command.intent.Operation {
	case protocol.ShopOperationOpen:
		r.sendShopSnapshot(s, catalog, npc.ID, catalogShop, tick, report)
	case protocol.ShopOperationBuy:
		offer, ok := shop.FindOffer(catalogShop, strings.TrimSpace(command.intent.OfferID))
		if !ok {
			report.CommandErrors = append(report.CommandErrors, CommandError{Command: name, SessionID: command.sessionID, Err: ErrShopOfferNotFound})
			return
		}
		inv := r.inventories[s.CharacterIdentity.ID]
		if inv == nil {
			report.CommandErrors = append(report.CommandErrors, CommandError{Command: name, SessionID: command.sessionID, Err: inventory.ErrInsufficient})
			return
		}
		if err := inv.Exchange(offer.CostArchetypeID, offer.CostQuantity, offer.ItemArchetypeID, offer.Quantity); err != nil {
			report.CommandErrors = append(report.CommandErrors, CommandError{Command: name, SessionID: command.sessionID, Err: err})
			return
		}
		r.sessionInventoryPending[s.ID] = struct{}{}
	default:
		report.CommandErrors = append(report.CommandErrors, CommandError{Command: name, SessionID: command.sessionID, Err: ErrInvalidShopIntent})
	}
}

func (r *Runtime) sendShopSnapshot(s *session.Session, catalog *shop.Catalog, npcEntityID world.EntityID, catalogShop shop.Shop, tick uint64, report *StepReport) {
	offers := make([]protocol.ShopOffer, len(catalogShop.Offers))
	for i, offer := range catalogShop.Offers {
		offers[i] = protocol.ShopOffer{
			OfferID:         offer.ID,
			ItemArchetypeID: offer.ItemArchetypeID,
			Quantity:        offer.Quantity,
			CostArchetypeID: offer.CostArchetypeID,
			CostQuantity:    offer.CostQuantity,
		}
	}
	message := protocol.ShopSnapshot{
		Revision:    catalog.Revision(),
		NPCEntityID: npcEntityID,
		ShopID:      catalogShop.ID,
		Offers:      offers,
	}
	envelope := protocol.Envelope{
		Delivery:   protocol.DeliveryReliableOrdered,
		Sequence:   s.NextOutboundSequence(protocol.DeliveryReliableOrdered),
		ServerTick: tick,
		Message:    message,
	}
	report.Metrics.OutboundMessages++
	if err := s.Connection().TrySend(envelope); err != nil {
		report.DeliveryErrors = append(report.DeliveryErrors, DeliveryError{SessionID: s.ID, Delivery: envelope.Delivery, MessageType: message.Type(), Err: err})
	}
}
