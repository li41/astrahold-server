// Package tcpudp 提供 S2 開發階段的 TCP Reliable + UDP Realtime 網路介面。
package tcpudp

import (
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"errors"

	"github.com/li41/astrahold-server/internal/protocol"
	"github.com/li41/astrahold-server/internal/transport"
)

const (
	DatagramMagic      uint32 = 0x41535455 // ASTU
	DatagramHeaderSize        = 24
	MaxDatagramSize           = 1200
)

var (
	ErrDatagramTooShort   = errors.New("tcpudp: datagram too short")
	ErrDatagramTooLarge   = errors.New("tcpudp: datagram too large")
	ErrBadDatagramMagic   = errors.New("tcpudp: bad datagram magic")
	ErrBadDatagramVersion = errors.New("tcpudp: bad datagram version")
	ErrBadDatagramHeader  = errors.New("tcpudp: bad datagram header")
	ErrInvalidToken       = errors.New("tcpudp: invalid realtime token")
)

type Token [16]byte

func NewToken() (Token, error) {
	var token Token
	_, err := rand.Read(token[:])
	return token, err
}

func (t Token) String() string { return hex.EncodeToString(t[:]) }

func ParseToken(value string) (Token, error) {
	var token Token
	decoded, err := hex.DecodeString(value)
	if err != nil || len(decoded) != len(token) {
		return Token{}, ErrInvalidToken
	}
	copy(token[:], decoded)
	return token, nil
}

func EncodeDatagram(token Token, envelope protocol.Envelope, codec transport.PayloadCodec) ([]byte, error) {
	frame, err := transport.EncodeEnvelope(envelope, codec)
	if err != nil {
		return nil, err
	}
	total := DatagramHeaderSize + len(frame)
	if total > MaxDatagramSize {
		return nil, ErrDatagramTooLarge
	}
	out := make([]byte, total)
	binary.BigEndian.PutUint32(out[0:4], DatagramMagic)
	binary.BigEndian.PutUint16(out[4:6], protocol.Version)
	binary.BigEndian.PutUint16(out[6:8], DatagramHeaderSize)
	copy(out[8:24], token[:])
	copy(out[24:], frame)
	return out, nil
}

func DecodeDatagram(data []byte, codec transport.PayloadCodec) (Token, protocol.Envelope, error) {
	if len(data) < DatagramHeaderSize+int(transport.HeaderSize) {
		return Token{}, protocol.Envelope{}, ErrDatagramTooShort
	}
	if len(data) > MaxDatagramSize {
		return Token{}, protocol.Envelope{}, ErrDatagramTooLarge
	}
	if binary.BigEndian.Uint32(data[0:4]) != DatagramMagic {
		return Token{}, protocol.Envelope{}, ErrBadDatagramMagic
	}
	if binary.BigEndian.Uint16(data[4:6]) != protocol.Version {
		return Token{}, protocol.Envelope{}, ErrBadDatagramVersion
	}
	if int(binary.BigEndian.Uint16(data[6:8])) != DatagramHeaderSize {
		return Token{}, protocol.Envelope{}, ErrBadDatagramHeader
	}
	var token Token
	copy(token[:], data[8:24])
	envelope, err := transport.DecodeEnvelope(data[24:], codec)
	if err != nil {
		return Token{}, protocol.Envelope{}, err
	}
	return token, envelope, nil
}
