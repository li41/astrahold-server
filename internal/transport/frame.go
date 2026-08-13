// Package transport 定義與底層 UDP/TCP/QUIC 無關的 Astrahold frame 格式。
package transport

import (
	"encoding/binary"
	"errors"
	"fmt"

	"github.com/li41/astrahold-server/internal/protocol"
)

const (
	Magic          uint32 = 0x41535452 // ASTR
	HeaderSize     uint16 = 28
	MaxPayloadSize uint32 = 4 * 1024 * 1024
)

var (
	ErrFrameTooShort      = errors.New("transport: frame too short")
	ErrBadMagic           = errors.New("transport: bad magic")
	ErrUnsupportedVersion = errors.New("transport: unsupported protocol version")
	ErrBadHeaderSize      = errors.New("transport: bad header size")
	ErrPayloadTooLarge    = errors.New("transport: payload too large")
	ErrPayloadLength      = errors.New("transport: payload length mismatch")
	ErrInvalidDelivery    = errors.New("transport: invalid delivery class")
	ErrInvalidMessageType = errors.New("transport: invalid message type")
)

type Frame struct {
	MessageType protocol.MessageType
	Delivery    protocol.Delivery
	Flags       uint8
	Sequence    uint32
	ServerTick  uint64
	Payload     []byte
}

func EncodeFrame(f Frame) ([]byte, error) {
	if f.MessageType == protocol.MessageUnknown {
		return nil, ErrInvalidMessageType
	}
	if f.Delivery != protocol.DeliveryReliableOrdered && f.Delivery != protocol.DeliveryRealtimeSequenced {
		return nil, ErrInvalidDelivery
	}
	if uint64(len(f.Payload)) > uint64(MaxPayloadSize) {
		return nil, ErrPayloadTooLarge
	}
	out := make([]byte, int(HeaderSize)+len(f.Payload))
	binary.BigEndian.PutUint32(out[0:4], Magic)
	binary.BigEndian.PutUint16(out[4:6], protocol.Version)
	binary.BigEndian.PutUint16(out[6:8], HeaderSize)
	binary.BigEndian.PutUint16(out[8:10], uint16(f.MessageType))
	out[10] = byte(f.Delivery)
	out[11] = f.Flags
	binary.BigEndian.PutUint32(out[12:16], f.Sequence)
	binary.BigEndian.PutUint64(out[16:24], f.ServerTick)
	binary.BigEndian.PutUint32(out[24:28], uint32(len(f.Payload)))
	copy(out[HeaderSize:], f.Payload)
	return out, nil
}

func DecodeFrame(data []byte) (Frame, error) {
	if len(data) < int(HeaderSize) {
		return Frame{}, ErrFrameTooShort
	}
	if binary.BigEndian.Uint32(data[0:4]) != Magic {
		return Frame{}, ErrBadMagic
	}
	if binary.BigEndian.Uint16(data[4:6]) != protocol.Version {
		return Frame{}, ErrUnsupportedVersion
	}
	headerSize := binary.BigEndian.Uint16(data[6:8])
	if headerSize != HeaderSize {
		return Frame{}, fmt.Errorf("%w: %d", ErrBadHeaderSize, headerSize)
	}
	mt := protocol.MessageType(binary.BigEndian.Uint16(data[8:10]))
	if mt == protocol.MessageUnknown {
		return Frame{}, ErrInvalidMessageType
	}
	delivery := protocol.Delivery(data[10])
	if delivery != protocol.DeliveryReliableOrdered && delivery != protocol.DeliveryRealtimeSequenced {
		return Frame{}, ErrInvalidDelivery
	}
	payloadLen := binary.BigEndian.Uint32(data[24:28])
	if payloadLen > MaxPayloadSize {
		return Frame{}, ErrPayloadTooLarge
	}
	if len(data) != int(headerSize)+int(payloadLen) {
		return Frame{}, ErrPayloadLength
	}
	payload := make([]byte, int(payloadLen))
	copy(payload, data[headerSize:])
	return Frame{MessageType: mt, Delivery: delivery, Flags: data[11], Sequence: binary.BigEndian.Uint32(data[12:16]), ServerTick: binary.BigEndian.Uint64(data[16:24]), Payload: payload}, nil
}
