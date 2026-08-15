// Package tcpudp 提供 S2 開發階段的 TCP Reliable + UDP Realtime 網路介面。
package tcpudp

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/binary"
	"encoding/hex"
	"errors"

	"github.com/li41/astrahold-server/internal/protocol"
	"github.com/li41/astrahold-server/internal/transport"
)

const (
	DatagramMagic       uint32 = 0x41535455 // ASTU
	DatagramHeaderSize         = 24
	DatagramAuthTagSize        = 16
	MaxDatagramSize            = 1200

	datagramRouteDomainSize = 8
	datagramAuthDomainSize  = 7
)

var (
	ErrDatagramTooShort       = errors.New("tcpudp: datagram too short")
	ErrDatagramTooLarge       = errors.New("tcpudp: datagram too large")
	ErrBadDatagramMagic       = errors.New("tcpudp: bad datagram magic")
	ErrBadDatagramVersion     = errors.New("tcpudp: bad datagram version")
	ErrBadDatagramHeader      = errors.New("tcpudp: bad datagram header")
	ErrDatagramAuthentication = errors.New("tcpudp: realtime datagram authentication failed")
	ErrInvalidToken           = errors.New("tcpudp: invalid realtime token")
)

var (
	datagramRouteDomain = [datagramRouteDomainSize]byte{'A', 'S', 'T', 'U', '-', 'R', '9', 0}
	datagramAuthDomain  = [datagramAuthDomainSize]byte{'A', 'S', 'T', 'U', '-', 'A', '9'}
)

type Token [16]byte

type datagramDirection byte

const (
	datagramClientToServer datagramDirection = 1
	datagramServerToClient datagramDirection = 2
)

func NewToken() (Token, error) {
	var token Token
	_, err := rand.Read(token[:])
	return token, err
}

func (t Token) String() string { return hex.EncodeToString(t[:]) }

// RoutingID is a public per-session UDP lookup handle. It is deliberately derived one-way
// from the bearer secret so ASTU packets never disclose the realtime token itself.
func (t Token) RoutingID() Token {
	var material [datagramRouteDomainSize + len(Token{})]byte
	copy(material[:datagramRouteDomainSize], datagramRouteDomain[:])
	copy(material[datagramRouteDomainSize:], t[:])
	digest := sha256.Sum256(material[:])
	var route Token
	copy(route[:], digest[:len(route)])
	return route
}

func ParseToken(value string) (Token, error) {
	var token Token
	decoded, err := hex.DecodeString(value)
	if err != nil || len(decoded) != len(token) {
		return Token{}, ErrInvalidToken
	}
	copy(token[:], decoded)
	return token, nil
}

func EncodeClientDatagram(token Token, envelope protocol.Envelope, codec transport.PayloadCodec) ([]byte, error) {
	return appendEncodeDatagram(make([]byte, 0, MaxDatagramSize), token, datagramClientToServer, envelope, codec)
}

// EncodeDatagram is the compatibility name for the historical Client -> Server encoder.
// It intentionally aliases only the authenticated C2S direction; there is no directionless
// DecodeDatagram compatibility path in Protocol v9.
func EncodeDatagram(token Token, envelope protocol.Envelope, codec transport.PayloadCodec) ([]byte, error) {
	return EncodeClientDatagram(token, envelope, codec)
}

func EncodeServerDatagram(token Token, envelope protocol.Envelope, codec transport.PayloadCodec) ([]byte, error) {
	return appendEncodeDatagram(make([]byte, 0, MaxDatagramSize), token, datagramServerToClient, envelope, codec)
}

// AppendEncodeServerDatagram directly materializes ASTU + ASTR + payload + auth tag into dst.
// The realtime writer supplies a reusable MTU-sized buffer, so the hot path keeps the existing
// zero-allocation ownership contract while adding per-datagram authentication.
func AppendEncodeServerDatagram(dst []byte, token Token, envelope protocol.Envelope, codec transport.PayloadCodec) ([]byte, error) {
	return appendEncodeDatagram(dst, token, datagramServerToClient, envelope, codec)
}

func appendEncodeDatagram(dst []byte, token Token, direction datagramDirection, envelope protocol.Envelope, codec transport.PayloadCodec) ([]byte, error) {
	start := len(dst)
	if DatagramHeaderSize <= cap(dst)-start {
		dst = dst[:start+DatagramHeaderSize]
		clear(dst[start:])
	} else {
		dst = append(dst, make([]byte, DatagramHeaderSize)...)
	}

	header := dst[start : start+DatagramHeaderSize]
	binary.BigEndian.PutUint32(header[0:4], DatagramMagic)
	binary.BigEndian.PutUint16(header[4:6], protocol.Version)
	binary.BigEndian.PutUint16(header[6:8], DatagramHeaderSize)
	route := token.RoutingID()
	copy(header[8:24], route[:])

	var err error
	dst, err = transport.AppendEncodeEnvelope(dst, envelope, codec)
	if err != nil {
		return dst[:start], err
	}
	if len(dst)-start+DatagramAuthTagSize > MaxDatagramSize {
		return dst[:start], ErrDatagramTooLarge
	}

	tag := computeDatagramAuthTag(token, direction, dst[start:])
	dst = append(dst, tag[:]...)
	return dst, nil
}

// ParseDatagramRoute validates only the public fixed ASTU header and returns its routing ID.
// Callers MUST authenticate the packet with DecodeClientDatagram/DecodeServerDatagram before
// treating any frame bytes, sequence, or endpoint as trusted.
func ParseDatagramRoute(data []byte) (Token, error) {
	if err := validateDatagramHeader(data); err != nil {
		return Token{}, err
	}
	var route Token
	copy(route[:], data[8:24])
	return route, nil
}

func DecodeClientDatagram(expectedToken Token, data []byte, codec transport.PayloadCodec) (protocol.Envelope, error) {
	return decodeDatagram(expectedToken, datagramClientToServer, data, codec)
}

func DecodeServerDatagram(expectedToken Token, data []byte, codec transport.PayloadCodec) (protocol.Envelope, error) {
	return decodeDatagram(expectedToken, datagramServerToClient, data, codec)
}

func decodeDatagram(expectedToken Token, direction datagramDirection, data []byte, codec transport.PayloadCodec) (protocol.Envelope, error) {
	route, err := ParseDatagramRoute(data)
	if err != nil {
		return protocol.Envelope{}, err
	}
	expectedRoute := expectedToken.RoutingID()
	if subtle.ConstantTimeCompare(route[:], expectedRoute[:]) != 1 {
		return protocol.Envelope{}, ErrDatagramAuthentication
	}

	authenticated := data[:len(data)-DatagramAuthTagSize]
	actualTag := data[len(data)-DatagramAuthTagSize:]
	expectedTag := computeDatagramAuthTag(expectedToken, direction, authenticated)
	if subtle.ConstantTimeCompare(actualTag, expectedTag[:]) != 1 {
		return protocol.Envelope{}, ErrDatagramAuthentication
	}

	envelope, err := transport.DecodeEnvelope(authenticated[DatagramHeaderSize:], codec)
	if err != nil {
		return protocol.Envelope{}, err
	}
	return envelope, nil
}

func validateDatagramHeader(data []byte) error {
	if len(data) < DatagramHeaderSize+int(transport.HeaderSize)+DatagramAuthTagSize {
		return ErrDatagramTooShort
	}
	if len(data) > MaxDatagramSize {
		return ErrDatagramTooLarge
	}
	if binary.BigEndian.Uint32(data[0:4]) != DatagramMagic {
		return ErrBadDatagramMagic
	}
	if binary.BigEndian.Uint16(data[4:6]) != protocol.Version {
		return ErrBadDatagramVersion
	}
	if int(binary.BigEndian.Uint16(data[6:8])) != DatagramHeaderSize {
		return ErrBadDatagramHeader
	}
	return nil
}

// computeDatagramAuthTag is HMAC-SHA256 truncated to 128 bits. The direction byte is an
// out-of-band domain separator: a captured S2C snapshot/correction can never be reflected as
// a valid C2S input even though both directions share the session's realtime secret.
// Fixed stack buffers preserve the realtime encoder's zero-allocation hot-path contract.
func computeDatagramAuthTag(token Token, direction datagramDirection, authenticated []byte) [DatagramAuthTagSize]byte {
	var inner [sha256.BlockSize + datagramAuthDomainSize + 1 + MaxDatagramSize]byte
	var outer [sha256.BlockSize + sha256.Size]byte
	for i := 0; i < sha256.BlockSize; i++ {
		var key byte
		if i < len(token) {
			key = token[i]
		}
		inner[i] = key ^ 0x36
		outer[i] = key ^ 0x5c
	}

	offset := sha256.BlockSize
	copy(inner[offset:], datagramAuthDomain[:])
	offset += datagramAuthDomainSize
	inner[offset] = byte(direction)
	offset++
	copy(inner[offset:], authenticated)
	offset += len(authenticated)
	innerDigest := sha256.Sum256(inner[:offset])
	copy(outer[sha256.BlockSize:], innerDigest[:])
	outerDigest := sha256.Sum256(outer[:])

	var tag [DatagramAuthTagSize]byte
	copy(tag[:], outerDigest[:DatagramAuthTagSize])
	return tag
}
