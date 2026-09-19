package gameplayworld

import (
	"errors"
	"math"

	"github.com/li41/astrahold-server/internal/world"
)

const (
	// GMRoomWorldID is the durable world identity for the isolated GM room.
	GMRoomWorldID = "gm-room"
	// GMRoomWorldRevision is pinned with the cross-world transfer contract so a stale source
	// Server cannot silently write a snapshot for a different GM-room gameplay revision.
	GMRoomWorldRevision = "map0-gm-room-v1"
	// GMRoomGameplaySHA256 is the SHA-256 of worlds/gm-room/gameplay.json for the revision above.
	// The authoritative fixture test deliberately fails when that file changes without this
	// cross-world destination contract being reviewed.
	GMRoomGameplaySHA256 = "7deeca7dacc13fdab6952c3060d9c3c5d99ecc8c6d143f3dc13fb31a418a4d5c"

	GMRoomFloorSurfaceID       = "map0-floor"
	GMRoomStockKeeperBlockerID = "stock-keeper"
)

var ErrGMRoomArrivalAuthority = errors.New("gameplayworld: invalid gm-room arrival authority")

// MapTransferTarget is a Server-owned cross-map destination. It carries only gameplay identity
// and transform; Client scenes/assets/endpoints are intentionally outside this contract.
type MapTransferTarget struct {
	MapID          MapID
	WorldID        string
	Revision       string
	GameplaySHA256 string
	Transform      world.Transform
}

// GMRoomTransferTarget is the formal arrival used by Server-authoritative transfers into map0.
// The player lands at the authored floor center and faces the stock-keeper NPC.
func GMRoomTransferTarget() MapTransferTarget {
	return MapTransferTarget{
		MapID:          MapIDGMRoom,
		WorldID:        GMRoomWorldID,
		Revision:       GMRoomWorldRevision,
		GameplaySHA256: GMRoomGameplaySHA256,
		Transform: world.Transform{
			Position: world.Position{X: 0, Y: 0, Z: 0, Layer: 0},
			Yaw:      88.23761,
		},
	}
}

// GMRoomArrivalTransform derives the intended arrival from authored Server gameplay geometry.
// It is used to lock the compiled cross-world target to the room center and stock-keeper facing.
func GMRoomArrivalTransform(d Definition) (world.Transform, error) {
	m, ok := d.Map(MapIDGMRoom)
	if !ok || m.Kind != MapKindIsolated || d.WorldID != GMRoomWorldID {
		return world.Transform{}, ErrGMRoomArrivalAuthority
	}

	var floor *Surface
	for i := range d.Surfaces {
		if d.Surfaces[i].ID == GMRoomFloorSurfaceID {
			floor = &d.Surfaces[i]
			break
		}
	}
	if floor == nil {
		return world.Transform{}, ErrGMRoomArrivalAuthority
	}

	var keeper *Blocker
	for i := range d.Blockers {
		if d.Blockers[i].ID == GMRoomStockKeeperBlockerID {
			keeper = &d.Blockers[i]
			break
		}
	}
	if keeper == nil || !keeper.Enabled || keeper.Layer != floor.Layer {
		return world.Transform{}, ErrGMRoomArrivalAuthority
	}

	x := (floor.Bounds.MinX + floor.Bounds.MaxX) * 0.5
	z := (floor.Bounds.MinZ + floor.Bounds.MaxZ) * 0.5
	y := floor.Plane.HeightAt(x, z)
	keeperX := (keeper.Bounds.MinX + keeper.Bounds.MaxX) * 0.5
	keeperZ := (keeper.Bounds.MinZ + keeper.Bounds.MaxZ) * 0.5
	dx := keeperX - x
	dz := keeperZ - z
	if dx == 0 && dz == 0 {
		return world.Transform{}, ErrGMRoomArrivalAuthority
	}
	yaw := float32(math.Atan2(float64(dz), float64(dx)) * 180 / math.Pi)
	return world.Transform{Position: world.Position{X: x, Y: y, Z: z, Layer: floor.Layer}, Yaw: yaw}, nil
}
