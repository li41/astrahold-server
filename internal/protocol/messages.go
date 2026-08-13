// Package protocol 定義 Astrahold 自有協定的語意層；不綁定特定 wire codec 或 transport。
package protocol

import "github.com/li41/astrahold-server/internal/world"

const Version uint16 = 1

type MessageType uint16

const (
	MessageUnknown            MessageType = 0
	MessageClientMoveInput    MessageType = 1
	MessageEntitySpawn        MessageType = 100
	MessageEntityDespawn      MessageType = 101
	MessageWorldSnapshot      MessageType = 102
	MessagePositionCorrection MessageType = 103
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

// ClientMoveInput 只描述移動意圖；input sequence 由外層 Envelope/Frame 提供，避免雙重來源。
type ClientMoveInput struct {
	DirectionX, DirectionZ float32
}

func (ClientMoveInput) Type() MessageType { return MessageClientMoveInput }

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
