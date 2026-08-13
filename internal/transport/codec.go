package transport

import (
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
