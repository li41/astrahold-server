package main

import (
	"flag"
	"fmt"
	"log"

	"github.com/li41/astrahold-server/internal/siege"
	"github.com/li41/astrahold-server/internal/siegeownership"
)

var siegeOwnershipDir = flag.String("siege-ownership-dir", "data/siege-ownership", "Durable single-writer siege castle ownership directory")

type siegeOwnershipPersistence struct {
	worldID string
	store   *siegeownership.Store
}

func openSiegeOwnershipPersistence(root, worldID, initialOwnerID string) (*siegeOwnershipPersistence, siege.CastleOwnershipState, bool, error) {
	store, err := siegeownership.Open(root)
	if err != nil {
		return nil, siege.CastleOwnershipState{}, false, err
	}
	state, created, err := store.LoadOrCreate(worldID, initialOwnerID)
	if err != nil {
		return nil, siege.CastleOwnershipState{}, false, err
	}
	return &siegeOwnershipPersistence{worldID: worldID, store: store}, state, created, nil
}

func (p *siegeOwnershipPersistence) Commit(transfer siege.CastleOwnershipTransfer) (siege.CastleOwnershipState, error) {
	if p == nil || p.store == nil || p.worldID == "" {
		return siege.CastleOwnershipState{}, fmt.Errorf("siege ownership persistence unavailable")
	}
	state, err := p.store.Commit(p.worldID, transfer)
	if err != nil {
		log.Printf("siege ownership durable commit failed: world=%s expected_revision=%d previous_owner=%s next_owner=%s match=%s err=%v", p.worldID, transfer.ExpectedRevision, transfer.PreviousOwnerID, transfer.OwnerID, transfer.MatchID, err)
		return siege.CastleOwnershipState{}, err
	}
	return state, nil
}

func (p *siegeOwnershipPersistence) Path() string {
	if p == nil || p.store == nil {
		return ""
	}
	return p.store.Path()
}
