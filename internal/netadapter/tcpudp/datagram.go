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
	return AppendEncodeDatagram(make([]byte, 0, MaxDatagramSize), token, envelope, codec)
}

// AppendEncodeDatagram 直接把 ASTU header + ASTR frame + payload 寫入 dst。
// realtime writer 以每連線 reusable 1200-byte buffer 呼叫，避免每個 datagram 配置 payload/frame/datagram 三份 slice。
func AppendEncodeDatagram(dst []byte, token Token, envelope protocol.Envelope, codec transport.PayloadCodec) ([]byte, error) {
	start := len(dst)
	if DatagramHeaderSize <= cap(dst)-start {
		dst = dst[:start+DatagramHeaderSize]
		clear(dst[start:])
	} else {
		dst = append(dst, make([]byte, DatagramHeaderSize)...)
	}

	var err error
	dst, err = transport.AppendEncodeEnvelope(dst, envelope, codec)
	if err != nil {
		return dst[:start], err
	}
	if len(dst)-start > MaxDatagramSize {
		return dst[:start], ErrDatagramTooLarge
	}

	header := dst[start : start+DatagramHeaderSize]
	binary.BigEndian.PutUint32(header[0:4], DatagramMagic)
	binary.BigEndian.PutUint16(header[4:6], protocol.Version)
	binary.BigEndian.PutUint16(header[6:8], DatagramHeaderSize)
	copy(header[8:24], token[:])
	return dst, nil
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
