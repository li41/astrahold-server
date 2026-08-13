// Package protocol 定義 Astrahold 自有即時協定的語意 DTO。
//
// 這裡暫時不綁定 JSON、Protobuf、FlatBuffers 或自訂 binary encoding。
// 先把 gameplay message 語意定清楚，再用壓測決定 wire format。
package protocol

import "github.com/li41/astrahold-server/internal/world"

const Version uint16 = 1

// ClientMoveInput 是 client 上送的移動意圖。
// Client 不提供權威 delta time 或最終座標。
type ClientMoveInput struct {
	Sequence   uint32
	DirectionX float32
	DirectionZ float32
}

// EntityTransform 是 server 對 client 發布的權威空間狀態。
type EntityTransform struct {
	EntityID world.EntityID
	Tick     uint64
	Position world.Position
	Yaw      float32
}

// EntitySpawn 是 AOI 進入時的最小 spawn message。
type EntitySpawn struct {
	EntityID  world.EntityID
	Kind      world.EntityKind
	Transform EntityTransform
}

// EntityDespawn 是 AOI 離開時的 message。
type EntityDespawn struct {
	EntityID world.EntityID
}

// WorldSnapshot 是 correction / snapshot replication 的基礎容器。
type WorldSnapshot struct {
	Tick     uint64
	Entities []EntityTransform
}
