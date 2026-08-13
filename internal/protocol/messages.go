// Package protocol 定義 Astrahold 自有協定的語意層；不綁定特定 wire codec 或 transport。
package protocol

import (
	"encoding/hex"

	"github.com/li41/astrahold-server/internal/world"
)

// Version 在 wire-incompatible contract 變更時必須遞增。
// v5 將 S3-D Gate-specific intent 收斂為 Reliable ClientUseAction；
// damage/range/cooldown 仍完全由 Server Combat Action Catalog 決定。
const Version uint16 = 5

// MaxSnapshotEntitiesPerChunk 延續 Protocol v3 的 Realtime snapshot 單一 chunk 上限。
// compact payload 每個 transform 26 bytes；43 筆加上 14-byte snapshot header、28-byte ASTR frame
// 與 24-byte ASTU datagram header後共 1184 bytes，保留在 1200-byte UDP guard 內。
const MaxSnapshotEntitiesPerChunk = 43

type MessageType uint16

const (
	MessageUnknown         MessageType = 0
	MessageClientMoveInput MessageType = 1
	MessageClientUseAction MessageType = 2

	MessageSessionWelcome MessageType = 10

	MessageEntitySpawn        MessageType = 100
	MessageEntityDespawn      MessageType = 101
	MessageWorldSnapshot      MessageType = 102
	MessagePositionCorrection MessageType = 103
	MessageWorldDynamicState  MessageType = 104
)

type Delivery uint8

const (
	DeliveryUnknown Delivery = iota
	DeliveryReliableOrdered
	DeliveryRealtimeSequenced
)

type Message interface{ Type() MessageType }

type Envelope struct {
	Delivery   Delivery
	Sequence   uint32
	ServerTick uint64
	Message    Message
}

func (e Envelope) MessageType() MessageType {
	if e.Message == nil {
		return MessageUnknown
	}
	return e.Message.Type()
}

// WorldIdentity 確保 Client 與 Server 使用完全相同的 Gameplay Proxy。
type WorldIdentity struct {
	WorldID        string
	Revision       string
	GameplaySHA256 string
}

func (w WorldIdentity) Valid() bool {
	if w.WorldID == "" || w.Revision == "" || len(w.GameplaySHA256) != 64 {
		return false
	}
	decoded, err := hex.DecodeString(w.GameplaySHA256)
	return err == nil && len(decoded) == 32
}

// ClientMoveInput 只描述移動意圖；input sequence 由外層 Envelope/Frame 提供，避免雙重來源。
type ClientMoveInput struct {
	DirectionX float32
	DirectionZ float32
}

func (ClientMoveInput) Type() MessageType { return MessageClientMoveInput }

type ActionTargetKind string

const (
	ActionTargetGate ActionTargetKind = "gate"
)

// ClientUseAction 只描述玩家的 action intent 與目標識別。
// Client 不可提供 damage、range、cooldown、命中結果或 destroyed 判定。
// Action sequence 仍由外層 Envelope.Sequence 提供，與 movement input sequence 分流。
type ClientUseAction struct {
	ActionID   string
	TargetKind ActionTargetKind
	TargetID   string
}

func (ClientUseAction) Type() MessageType { return MessageClientUseAction }

// SessionWelcome 先建立 Reliable session。Client 必須驗證 WorldIdentity 後才啟用 realtime UDP。
type SessionWelcome struct {
	SessionID      uint64
	EntityID       world.EntityID
	RealtimePort   uint16
	RealtimeToken  string
	TickRateHz     uint16
	SnapshotRateHz uint16
	World          WorldIdentity
}

func (SessionWelcome) Type() MessageType { return MessageSessionWelcome }

type EntityTransform struct {
	EntityID world.EntityID
	Tick     uint64
	Position world.Position
	Yaw      float32
}

type EntitySpawn struct {
	EntityID  world.EntityID
	Kind      world.EntityKind
	Transform EntityTransform
}

func (EntitySpawn) Type() MessageType { return MessageEntitySpawn }

type EntityDespawn struct{ EntityID world.EntityID }

func (EntityDespawn) Type() MessageType { return MessageEntityDespawn }

// WorldSnapshot 是同一個 authoritative tick 的一個 Realtime transform chunk。
// Client 必須收齊 ChunkCount 個 chunk 後才把該 tick 提交給 interpolation buffer；
// 不完整的舊 tick 可以直接丟棄，不可把半張 snapshot 套用到畫面。
type WorldSnapshot struct {
	Tick       uint64
	ChunkIndex uint16
	ChunkCount uint16
	Entities   []EntityTransform
}

func (WorldSnapshot) Type() MessageType { return MessageWorldSnapshot }

func (s WorldSnapshot) ValidChunk() bool {
	return s.ChunkCount > 0 && s.ChunkIndex < s.ChunkCount && len(s.Entities) <= MaxSnapshotEntitiesPerChunk
}

type PositionCorrection struct {
	Tick                       uint64
	EntityID                   world.EntityID
	Position                   world.Position
	Yaw                        float32
	LastProcessedInputSequence uint32
}

func (PositionCorrection) Type() MessageType { return MessagePositionCorrection }

type WorldBlockerState struct {
	ID      string
	Enabled bool
}

type WorldGateState struct {
	ID        string
	HP        uint32
	MaxHP     uint32
	Destroyed bool
}

// WorldDynamicState 是低頻、Reliable、版本化的 World dynamic snapshot。
// Blocker 與 Gate state 在同一 revision 內同步，讓 Gate destroyed 與 navigation opening 原子呈現。
type WorldDynamicState struct {
	Revision uint64
	Blockers []WorldBlockerState
	Gates    []WorldGateState
}

func (WorldDynamicState) Type() MessageType { return MessageWorldDynamicState }
