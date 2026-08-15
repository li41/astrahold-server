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
	"time"

	"github.com/li41/astrahold-server/internal/characteridentity"
	"github.com/li41/astrahold-server/internal/netadapter/tcpudp"
	"github.com/li41/astrahold-server/internal/sessioncredential"
)

const (
	trustedCharacterAuthLegacySchemaVersion uint16 = 1
	trustedCharacterAuthSchemaVersion       uint16 = 2
	trustedCharacterAuthMagic                      = "ASTRAH1\x00"
	trustedCharacterAuthHeaderBytes                = len(trustedCharacterAuthMagic) + 2
	trustedCharacterAuthMaxCredentialBytes         = 256
	trustedCharacterAuthMaxCredentialIDBytes       = 128
	trustedCharacterAuthRevocationScopePrefix      = "static-v2:"
)

var trustedCharacterAuthFile = flag.String(
	"trusted-character-auth-file",
	"",
	"Optional server-side SHA-256 credential map for trusted character authentication; requires loopback TCP behind a secure local proxy/tunnel",
)

var (
	errTrustedCharacterAuthConfig              = errors.New("worldd: invalid trusted character auth config")
	errTrustedCharacterAuthRequiresLoopback    = errors.New("worldd: trusted character auth requires loopback TCP listen address")
	errTrustedCharacterAuthPreface             = errors.New("worldd: invalid trusted character auth preface")
	errTrustedCharacterAuthCredential          = errors.New("worldd: trusted character credential rejected")
	errTrustedCharacterCredentialProviderGrant = errors.New("worldd: trusted character credential provider returned invalid grant")
	errTrustedCharacterTakeoverScope           = errors.New("worldd: trusted character takeover credential scope mismatch")
)

type trustedCharacterAuthDefinition struct {
	SchemaVersion uint16                           `json:"schema_version"`
	Revision      string                           `json:"revision"`
	Credentials   []trustedCharacterAuthCredential `json:"credentials"`
}

type trustedCharacterAuthCredential struct {
	CredentialID        string `json:"credential_id,omitempty"`
	TokenSHA256         string `json:"token_sha256"`
	CharacterID         string `json:"character_id"`
	AllowActiveTakeover bool   `json:"allow_active_takeover,omitempty"`
	NotBefore           string `json:"not_before,omitempty"`
	ExpiresAt           string `json:"expires_at,omitempty"`
	RevokedAt           string `json:"revoked_at,omitempty"`
}

type staticTrustedCharacterCredentialEntry struct {
	credentialID    string
	tokenDigest     [sha256.Size]byte
	revocationScope string
	grant           sessioncredential.Grant
	lifecycle       sessioncredential.Lifecycle
}

type staticTrustedCharacterCredentialProvider struct {
	schemaVersion   uint16
	revision        string
	credentials     map[[sha256.Size]byte]staticTrustedCharacterCredentialEntry
	credentialsByID map[string]staticTrustedCharacterCredentialEntry
	now             func() time.Time
}

type trustedCharacterAuthenticator struct {
	provider sessioncredential.Provider
}

func loadTrustedCharacterAuthenticator(path, tcpAddress string) (tcpudp.TrustedCharacterConnectionAuthenticator, string, error) {
	if strings.TrimSpace(path) == "" {
		return nil, "", nil
	}
	if err := validateTrustedCharacterAuthListenAddress(tcpAddress); err != nil {
		return nil, "", err
	}
	provider, err := loadStaticTrustedCharacterCredentialProvider(path)
	if err != nil {
		return nil, "", err
	}
	authenticator, err := newTrustedCharacterAuthenticatorWithProvider(provider)
	if err != nil {
		return nil, "", err
	}
	return authenticator.Authenticate, provider.revision, nil
}

func loadStaticTrustedCharacterCredentialProvider(path string) (*staticTrustedCharacterCredentialProvider, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read trusted character auth config %q: %w", path, err)
	}
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	var definition trustedCharacterAuthDefinition
	if err := decoder.Decode(&definition); err != nil {
		return nil, fmt.Errorf("%w: decode: %v", errTrustedCharacterAuthConfig, err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return nil, fmt.Errorf("%w: trailing JSON value", errTrustedCharacterAuthConfig)
		}
		return nil, fmt.Errorf("%w: trailing data: %v", errTrustedCharacterAuthConfig, err)
	}
	return newStaticTrustedCharacterCredentialProvider(definition)
}

func newTrustedCharacterAuthenticator(definition trustedCharacterAuthDefinition) (*trustedCharacterAuthenticator, error) {
	provider, err := newStaticTrustedCharacterCredentialProvider(definition)
	if err != nil {
		return nil, err
	}
	return newTrustedCharacterAuthenticatorWithProvider(provider)
}

func newTrustedCharacterAuthenticatorWithProvider(provider sessioncredential.Provider) (*trustedCharacterAuthenticator, error) {
	if provider == nil {
		return nil, errTrustedCharacterAuthConfig
	}
	return &trustedCharacterAuthenticator{provider: provider}, nil
}

func newStaticTrustedCharacterCredentialProvider(definition trustedCharacterAuthDefinition) (*staticTrustedCharacterCredentialProvider, error) {
	if strings.TrimSpace(definition.Revision) == "" || len(definition.Credentials) == 0 {
		return nil, errTrustedCharacterAuthConfig
	}
	if definition.SchemaVersion != trustedCharacterAuthLegacySchemaVersion && definition.SchemaVersion != trustedCharacterAuthSchemaVersion {
		return nil, errTrustedCharacterAuthConfig
	}

	credentials := make(map[[sha256.Size]byte]staticTrustedCharacterCredentialEntry, len(definition.Credentials))
	credentialsByID := make(map[string]staticTrustedCharacterCredentialEntry, len(definition.Credentials))
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

		credentialID := ""
		revocationScope := ""
		lifecycle := sessioncredential.Lifecycle{}
		if definition.SchemaVersion == trustedCharacterAuthLegacySchemaVersion {
			if item.CredentialID != "" || item.NotBefore != "" || item.ExpiresAt != "" || item.RevokedAt != "" {
				return nil, fmt.Errorf("%w: credential[%d] lifecycle fields require schema_version %d", errTrustedCharacterAuthConfig, index, trustedCharacterAuthSchemaVersion)
			}
		} else {
			credentialID = strings.TrimSpace(item.CredentialID)
			if credentialID == "" || credentialID != item.CredentialID || len(credentialID) > trustedCharacterAuthMaxCredentialIDBytes {
				return nil, fmt.Errorf("%w: credential[%d] credential_id must be 1..%d trimmed bytes", errTrustedCharacterAuthConfig, index, trustedCharacterAuthMaxCredentialIDBytes)
			}
			if _, exists := credentialsByID[credentialID]; exists {
				return nil, fmt.Errorf("%w: duplicate credential_id %q", errTrustedCharacterAuthConfig, credentialID)
			}
			lifecycle, err = parseTrustedCharacterCredentialLifecycle(item)
			if err != nil {
				return nil, fmt.Errorf("%w: credential[%d] %q lifecycle: %v", errTrustedCharacterAuthConfig, index, credentialID, err)
			}
			revocationScope = trustedCharacterCredentialRevocationScope(item, binding)
		}

		entry := staticTrustedCharacterCredentialEntry{
			credentialID:    credentialID,
			tokenDigest:     digest,
			revocationScope: revocationScope,
			grant: sessioncredential.Grant{
				Identity:            binding,
				AllowActiveTakeover: item.AllowActiveTakeover,
				RevocationScope:     revocationScope,
			},
			lifecycle: lifecycle,
		}
		credentials[digest] = entry
		if credentialID != "" {
			credentialsByID[credentialID] = entry
		}
	}
	return &staticTrustedCharacterCredentialProvider{
		schemaVersion:   definition.SchemaVersion,
		revision:        definition.Revision,
		credentials:     credentials,
		credentialsByID: credentialsByID,
		now:             time.Now,
	}, nil
}

// trustedCharacterCredentialRevocationScope identifies the proof/identity/takeover
// generation. Lifecycle timestamps deliberately do not participate in the fingerprint:
// their Server-clock validity controls membership in the allowed scope set, so scheduling a
// future cutoff does not invalidate a currently valid session before that cutoff.
func trustedCharacterCredentialRevocationScope(item trustedCharacterAuthCredential, binding characteridentity.Binding) string {
	hasher := sha256.New()
	writeField := func(value string) {
		var size [4]byte
		binary.BigEndian.PutUint32(size[:], uint32(len(value)))
		_, _ = hasher.Write(size[:])
		_, _ = hasher.Write([]byte(value))
	}
	writeField("astrahold-static-credential-v2")
	writeField(item.CredentialID)
	writeField(item.TokenSHA256)
	writeField(string(binding.ID))
	if item.AllowActiveTakeover {
		writeField("takeover:1")
	} else {
		writeField("takeover:0")
	}
	return trustedCharacterAuthRevocationScopePrefix + hex.EncodeToString(hasher.Sum(nil))
}

func parseTrustedCharacterCredentialLifecycle(item trustedCharacterAuthCredential) (sessioncredential.Lifecycle, error) {
	notBefore, err := parseOptionalTrustedCharacterCredentialTime(item.NotBefore)
	if err != nil {
		return sessioncredential.Lifecycle{}, fmt.Errorf("not_before: %w", err)
	}
	expiresAt, err := parseOptionalTrustedCharacterCredentialTime(item.ExpiresAt)
	if err != nil {
		return sessioncredential.Lifecycle{}, fmt.Errorf("expires_at: %w", err)
	}
	revokedAt, err := parseOptionalTrustedCharacterCredentialTime(item.RevokedAt)
	if err != nil {
		return sessioncredential.Lifecycle{}, fmt.Errorf("revoked_at: %w", err)
	}
	lifecycle := sessioncredential.Lifecycle{
		NotBefore: notBefore,
		ExpiresAt: expiresAt,
		RevokedAt: revokedAt,
	}
	if err := lifecycle.Validate(); err != nil {
		return sessioncredential.Lifecycle{}, err
	}
	return lifecycle, nil
}

func parseOptionalTrustedCharacterCredentialTime(value string) (time.Time, error) {
	if value == "" {
		return time.Time{}, nil
	}
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return time.Time{}, err
	}
	return parsed.UTC(), nil
}

func (p *staticTrustedCharacterCredentialProvider) Resolve(ctx context.Context, credential []byte) (sessioncredential.Grant, error) {
	if p == nil || len(p.credentials) == 0 || len(credential) == 0 || p.now == nil {
		return sessioncredential.Grant{}, errTrustedCharacterAuthCredential
	}
	if ctx != nil {
		select {
		case <-ctx.Done():
			return sessioncredential.Grant{}, ctx.Err()
		default:
		}
	}
	digest := sha256.Sum256(credential)
	entry, ok := p.credentials[digest]
	if !ok {
		return sessioncredential.Grant{}, errTrustedCharacterAuthCredential
	}
	if err := entry.lifecycle.ValidateAt(p.now().UTC()); err != nil {
		if entry.credentialID == "" {
			return sessioncredential.Grant{}, errors.Join(errTrustedCharacterAuthCredential, err)
		}
		return sessioncredential.Grant{}, errors.Join(
			errTrustedCharacterAuthCredential,
			fmt.Errorf("credential_id %q: %w", entry.credentialID, err),
		)
	}
	return entry.grant, nil
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

func (a *trustedCharacterAuthenticator) Authenticate(ctx context.Context, request tcpudp.TrustedCharacterConnectionAuthenticationRequest) (tcpudp.TrustedCharacterConnectionAuthentication, error) {
	if a == nil || a.provider == nil || !request.Valid() {
		return tcpudp.TrustedCharacterConnectionAuthentication{}, errTrustedCharacterAuthPreface
	}
	if ctx == nil {
		ctx = context.Background()
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
	grant, err := a.provider.Resolve(ctx, credential)
	if err != nil {
		return tcpudp.TrustedCharacterConnectionAuthentication{}, err
	}
	if !grant.Valid() {
		return tcpudp.TrustedCharacterConnectionAuthentication{}, errTrustedCharacterCredentialProviderGrant
	}

	result := tcpudp.TrustedCharacterConnectionAuthentication{
		Identity:        grant.Identity,
		RevocationScope: grant.RevocationScope,
	}
	if grant.AllowActiveTakeover {
		characterID := grant.Identity.ID
		result.TakeoverAuthorizer = func(_ context.Context, takeover tcpudp.CharacterTakeoverRequest) error {
			if !takeover.Valid() || takeover.Identity.ID != characterID || takeover.ExpectedOwnership.CharacterID != characterID {
				return errTrustedCharacterTakeoverScope
			}
			return nil
		}
	}
	return result, nil
}
