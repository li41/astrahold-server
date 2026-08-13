// Package protocol 定義 Astrahold 自有協定的語意層；不綁定特定 wire codec 或 transport。
package protocol

import (
	"encoding/hex"

	"github.com/li41/astrahold-server/internal/world"
)

// Version 在 wire-incompatible contract 變更時必須遞增。
// v2 新增 World Identity 與 WorldDynamicState；舊 v1 Client 應在 Frame 層直接拒絕。
const Version uint16 = 2

type MessageType uint16

const (
	MessageUnknown         MessageType = 0
	MessageClientMoveInput MessageType = 1

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

type WorldSnapshot struct {
	Tick     uint64
	Entities []EntityTransform
}

func (WorldSnapshot) Type() MessageType { return MessageWorldSnapshot }

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

// WorldDynamicState 是低頻、Reliable、版本化的 World dynamic snapshot。
// S3-B 先包含 blocker；未來可擴充 objective/gate phase，但不能把高頻 Entity transform 塞進來。
type WorldDynamicState struct {
	Revision uint64
	Blockers []WorldBlockerState
}

func (WorldDynamicState) Type() MessageType { return MessageWorldDynamicState }
