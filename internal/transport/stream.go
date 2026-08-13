package transport

import (
	"encoding/binary"
	"io"

	"github.com/li41/astrahold-server/internal/protocol"
)

// WriteEnvelope 將一個完整 Astrahold Frame 寫入 stream transport。
func WriteEnvelope(w io.Writer, envelope protocol.Envelope, codec PayloadCodec) error {
	data, err := EncodeEnvelope(envelope, codec)
	if err != nil {
		return err
	}
	for len(data) > 0 {
		n, err := w.Write(data)
		if err != nil {
			return err
		}
		data = data[n:]
	}
	return nil
}

// ReadEnvelope 先讀固定 28-byte header，再依 payload length 精確讀取剩餘 bytes。
// TCP stream 因此不需要再疊一套額外的 length-prefix protocol。
func ReadEnvelope(r io.Reader, codec PayloadCodec) (protocol.Envelope, error) {
	header := make([]byte, int(HeaderSize))
	if _, err := io.ReadFull(r, header); err != nil {
		return protocol.Envelope{}, err
	}
	payloadLength := binary.BigEndian.Uint32(header[24:28])
	if payloadLength > MaxPayloadSize {
		return protocol.Envelope{}, ErrPayloadTooLarge
	}
	data := make([]byte, int(HeaderSize)+int(payloadLength))
	copy(data, header)
	if _, err := io.ReadFull(r, data[HeaderSize:]); err != nil {
		return protocol.Envelope{}, err
	}
	return DecodeEnvelope(data, codec)
}
