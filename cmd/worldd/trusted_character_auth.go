package main

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"strings"

	"github.com/li41/astrahold-server/internal/characteridentity"
	"github.com/li41/astrahold-server/internal/netadapter/tcpudp"
)

const (
	trustedCharacterAuthSchemaVersion   uint16 = 1
	trustedCharacterAuthMagic                  = "ASTRAH1\x00"
	trustedCharacterAuthHeaderBytes            = len(trustedCharacterAuthMagic) + 2
	trustedCharacterAuthMaxCredentialBytes     = 256
)

var trustedCharacterAuthFile = flag.String(
	"trusted-character-auth-file",
	"",
	"Optional server-side SHA-256 credential map for trusted character authentication; requires loopback TCP behind a secure local proxy/tunnel",
)

var (
	errTrustedCharacterAuthConfig          = errors.New("worldd: invalid trusted character auth config")
	errTrustedCharacterAuthRequiresLoopback = errors.New("worldd: trusted character auth requires loopback TCP listen address")
	errTrustedCharacterAuthPreface         = errors.New("worldd: invalid trusted character auth preface")
	errTrustedCharacterAuthCredential      = errors.New("worldd: trusted character credential rejected")
)

type trustedCharacterAuthDefinition struct {
	SchemaVersion uint16                           `json:"schema_version"`
	Revision      string                           `json:"revision"`
	Credentials   []trustedCharacterAuthCredential `json:"credentials"`
}

type trustedCharacterAuthCredential struct {
	TokenSHA256 string `json:"token_sha256"`
	CharacterID string `json:"character_id"`
}

type trustedCharacterAuthenticator struct {
	revision    string
	credentials map[[sha256.Size]byte]characteridentity.Binding
}

func loadTrustedCharacterAuthenticator(path, tcpAddress string) (tcpudp.TrustedCharacterConnectionAuthenticator, string, error) {
	if strings.TrimSpace(path) == "" {
		return nil, "", nil
	}
	if err := validateTrustedCharacterAuthListenAddress(tcpAddress); err != nil {
		return nil, "", err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, "", fmt.Errorf("read trusted character auth config %q: %w", path, err)
	}
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	var definition trustedCharacterAuthDefinition
	if err := decoder.Decode(&definition); err != nil {
		return nil, "", fmt.Errorf("%w: decode: %v", errTrustedCharacterAuthConfig, err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return nil, "", fmt.Errorf("%w: trailing JSON value", errTrustedCharacterAuthConfig)
		}
		return nil, "", fmt.Errorf("%w: trailing data: %v", errTrustedCharacterAuthConfig, err)
	}
	authenticator, err := newTrustedCharacterAuthenticator(definition)
	if err != nil {
		return nil, "", err
	}
	return authenticator.Authenticate, authenticator.revision, nil
}

func newTrustedCharacterAuthenticator(definition trustedCharacterAuthDefinition) (*trustedCharacterAuthenticator, error) {
	if definition.SchemaVersion != trustedCharacterAuthSchemaVersion || strings.TrimSpace(definition.Revision) == "" || len(definition.Credentials) == 0 {
		return nil, errTrustedCharacterAuthConfig
	}
	credentials := make(map[[sha256.Size]byte]characteridentity.Binding, len(definition.Credentials))
	for index, item := range definition.Credentials {
		if len(item.TokenSHA256) != sha256.Size*2 || strings.ToLower(item.TokenSHA256) != item.TokenSHA256 {
			return nil, fmt.Errorf("%w: credential[%d] token_sha256 must be 64 lowercase hex characters", errTrustedCharacterAuthConfig, index)
		}
		decoded, err := hex.DecodeString(item.TokenSHA256)
		if err != nil || len(decoded) != sha256.Size {
			return nil, fmt.Errorf("%w: credential[%d] token_sha256", errTrustedCharacterAuthConfig, index)
		}
		var digest [sha256.Size]byte
		copy(digest[:], decoded)
		if _, exists := credentials[digest]; exists {
			return nil, fmt.Errorf("%w: duplicate credential digest at index %d", errTrustedCharacterAuthConfig, index)
		}
		binding, err := characteridentity.NewTrusted(item.CharacterID)
		if err != nil {
			return nil, fmt.Errorf("%w: credential[%d] character_id: %v", errTrustedCharacterAuthConfig, index, err)
		}
		credentials[digest] = binding
	}
	return &trustedCharacterAuthenticator{revision: definition.Revision, credentials: credentials}, nil
}

func validateTrustedCharacterAuthListenAddress(address string) error {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return fmt.Errorf("%w: %v", errTrustedCharacterAuthRequiresLoopback, err)
	}
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() {
		return fmt.Errorf("%w: %q", errTrustedCharacterAuthRequiresLoopback, address)
	}
	return nil
}

func (a *trustedCharacterAuthenticator) Authenticate(_ context.Context, request tcpudp.TrustedCharacterConnectionAuthenticationRequest) (tcpudp.TrustedCharacterConnectionAuthentication, error) {
	if a == nil || len(a.credentials) == 0 || !request.Valid() {
		return tcpudp.TrustedCharacterConnectionAuthentication{}, errTrustedCharacterAuthPreface
	}
	var header [trustedCharacterAuthHeaderBytes]byte
	if _, err := io.ReadFull(request.Connection, header[:]); err != nil {
		return tcpudp.TrustedCharacterConnectionAuthentication{}, fmt.Errorf("%w: header: %v", errTrustedCharacterAuthPreface, err)
	}
	if string(header[:len(trustedCharacterAuthMagic)]) != trustedCharacterAuthMagic {
		return tcpudp.TrustedCharacterConnectionAuthentication{}, errTrustedCharacterAuthPreface
	}
	credentialLength := int(binary.BigEndian.Uint16(header[len(trustedCharacterAuthMagic):]))
	if credentialLength <= 0 || credentialLength > trustedCharacterAuthMaxCredentialBytes {
		return tcpudp.TrustedCharacterConnectionAuthentication{}, errTrustedCharacterAuthPreface
	}
	credential := make([]byte, credentialLength)
	defer clear(credential)
	if _, err := io.ReadFull(request.Connection, credential); err != nil {
		return tcpudp.TrustedCharacterConnectionAuthentication{}, fmt.Errorf("%w: credential: %v", errTrustedCharacterAuthPreface, err)
	}
	digest := sha256.Sum256(credential)
	identity, ok := a.credentials[digest]
	if !ok {
		return tcpudp.TrustedCharacterConnectionAuthentication{}, errTrustedCharacterAuthCredential
	}
	return tcpudp.TrustedCharacterConnectionAuthentication{Identity: identity}, nil
}
