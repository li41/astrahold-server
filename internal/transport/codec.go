package transport

import (
	"encoding/binary"
	"errors"

	"github.com/li41/astrahold-server/internal/protocol"
)

var ErrNilMessage = errors.New("transport: nil protocol message")

// PayloadCodec 只負責 protocol message <-> payload bytes。
// Frame、delivery channel、sequence 與 server tick 不屬於 codec 責任。
//
// 未來可實作 Protobuf、FlatBuffers 或 Astrahold 自訂 binary codec，
// 而不需要修改 simulation、replication 或 session package。
type PayloadCodec interface {
	Marshal(protocol.Message) ([]byte, error)
	Unmarshal(protocol.MessageType, []byte) (protocol.Message, error)
}

// AppendPayloadCodec 是不改變 wire contract 的可選 allocation-aware extension。
// Realtime writer 可直接把 payload append 到已保留 frame header 的 reusable buffer；
// 不支援此介面的 codec 仍會自動 fallback 到 PayloadCodec.Marshal。
type AppendPayloadCodec interface {
	AppendMarshal([]byte, protocol.Message) ([]byte, error)
}

func EncodeEnvelope(envelope protocol.Envelope, codec PayloadCodec) ([]byte, error) {
	if envelope.Message == nil {
		return nil, ErrNilMessage
	}
	payload, err := codec.Marshal(envelope.Message)
	if err != nil {
		return nil, err
	}
	return EncodeFrame(Frame{
		MessageType: envelope.Message.Type(),
		Delivery:    envelope.Delivery,
		Sequence:    envelope.Sequence,
		ServerTick:  envelope.ServerTick,
		Payload:     payload,
	})
}

// AppendEncodeEnvelope 將完整 ASTR frame 直接 append 到 dst。
// 若 dst 有足夠 capacity 且 codec 支援 AppendPayloadCodec，hot path 不需要配置 payload/frame 中間 slice。
func AppendEncodeEnvelope(dst []byte, envelope protocol.Envelope, codec PayloadCodec) ([]byte, error) {
	if envelope.Message == nil {
		return dst, ErrNilMessage
	}
	messageType := envelope.Message.Type()
	if messageType == protocol.MessageUnknown {
		return dst, ErrInvalidMessageType
	}
	if envelope.Delivery != protocol.DeliveryReliableOrdered && envelope.Delivery != protocol.DeliveryRealtimeSequenced {
		return dst, ErrInvalidDelivery
	}

	frameStart := len(dst)
	dst = growBytes(dst, int(HeaderSize))
	payloadStart := len(dst)
	var err error
	if appender, ok := codec.(AppendPayloadCodec); ok {
		dst, err = appender.AppendMarshal(dst, envelope.Message)
	} else {
		var payload []byte
		payload, err = codec.Marshal(envelope.Message)
		if err == nil {
			dst = append(dst, payload...)
		}
	}
	if err != nil {
		return dst[:frameStart], err
	}
	payloadLen := len(dst) - payloadStart
	if uint64(payloadLen) > uint64(MaxPayloadSize) {
		return dst[:frameStart], ErrPayloadTooLarge
	}

	header := dst[frameStart:payloadStart]
	binary.BigEndian.PutUint32(header[0:4], Magic)
	binary.BigEndian.PutUint16(header[4:6], protocol.Version)
	binary.BigEndian.PutUint16(header[6:8], HeaderSize)
	binary.BigEndian.PutUint16(header[8:10], uint16(messageType))
	header[10] = byte(envelope.Delivery)
	header[11] = 0
	binary.BigEndian.PutUint32(header[12:16], envelope.Sequence)
	binary.BigEndian.PutUint64(header[16:24], envelope.ServerTick)
	binary.BigEndian.PutUint32(header[24:28], uint32(payloadLen))
	return dst, nil
}

func growBytes(dst []byte, count int) []byte {
	if count <= 0 {
		return dst
	}
	start := len(dst)
	if count <= cap(dst)-start {
		dst = dst[:start+count]
		clear(dst[start:])
		return dst
	}
	return append(dst, make([]byte, count)...)
}

func DecodeEnvelope(data []byte, codec PayloadCodec) (protocol.Envelope, error) {
	frame, err := DecodeFrame(data)
	if err != nil {
		return protocol.Envelope{}, err
	}
	message, err := codec.Unmarshal(frame.MessageType, frame.Payload)
	if err != nil {
		return protocol.Envelope{}, err
	}
	return protocol.Envelope{
		Delivery:   frame.Delivery,
		Sequence:   frame.Sequence,
		ServerTick: frame.ServerTick,
		Message:    message,
	}, nil
}
