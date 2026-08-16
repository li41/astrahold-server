package main

import (
	"bytes"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"sort"
	"strings"
	"sync"
)

const (
	sessionLeafRevocationSchemaVersion      uint16 = 1
	sessionLeafRevocationMaxDefinitionBytes        = 64 * 1024
	sessionLeafRevocationMaxEntries                = 256
)

var sessionLoginTrustedProxyLeafRevocationFile = flag.String(
	"session-login-trusted-proxy-leaf-revocation-file",
	"",
	"Optional schema-v1 F.24 trusted-proxy leaf credential revocation file using lowercase SHA-256 SPKI identifiers; reloads independently on SIGHUP with LKG retention",
)

var (
	errSessionLeafRevocationConfig  = errors.New("worldd: invalid trusted proxy leaf revocation config")
	errSessionLeafCredentialRevoked = errors.New("worldd: trusted proxy leaf credential revoked")
)

type sessionLeafRevocationDefinition struct {
	SchemaVersion     uint16   `json:"schema_version"`
	Revision          string   `json:"revision"`
	RevokedSPKISHA256 []string `json:"revoked_spki_sha256"`
}

type sessionLeafRevocationSnapshot struct {
	revision        string
	revoked         map[[sha256.Size]byte]struct{}
	authorityDigest [sha256.Size]byte
}

type sessionLeafRevocationMetadata struct {
	Generation             uint64
	Revision               string
	RevokedCredentialCount int
}

type sessionLeafRevocationReloadResult struct {
	PreviousGeneration     uint64
	Generation             uint64
	PreviousRevision       string
	Revision               string
	RevokedCredentialCount int
	AuthorityChanged       bool
}

type reloadableSessionLeafRevocation struct {
	mu             sync.RWMutex
	definitionFile string
	current        *sessionLeafRevocationSnapshot
	generation     uint64
}

func sessionLeafRevocationRequested() bool {
	return sessionLoginTrustedProxyLeafRevocationFile != nil && strings.TrimSpace(*sessionLoginTrustedProxyLeafRevocationFile) != ""
}

func newReloadableSessionLeafRevocation(definitionFile string) (*reloadableSessionLeafRevocation, error) {
	definitionFile = strings.TrimSpace(definitionFile)
	if definitionFile == "" {
		return nil, errSessionLeafRevocationConfig
	}
	snapshot, err := loadSessionLeafRevocationSnapshot(definitionFile)
	if err != nil {
		return nil, err
	}
	return &reloadableSessionLeafRevocation{
		definitionFile: definitionFile,
		current:        snapshot,
		generation:     1,
	}, nil
}

func loadSessionLeafRevocationSnapshot(definitionFile string) (*sessionLeafRevocationSnapshot, error) {
	data, err := readSessionProxyMTLSFile(definitionFile, sessionLeafRevocationMaxDefinitionBytes)
	if err != nil {
		return nil, fmt.Errorf("%w: revocation file: %v", errSessionLeafRevocationConfig, err)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var definition sessionLeafRevocationDefinition
	if err := decoder.Decode(&definition); err != nil {
		return nil, fmt.Errorf("%w: decode revocation file: %v", errSessionLeafRevocationConfig, err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return nil, fmt.Errorf("%w: trailing JSON value", errSessionLeafRevocationConfig)
		}
		return nil, fmt.Errorf("%w: trailing revocation data: %v", errSessionLeafRevocationConfig, err)
	}
	if definition.SchemaVersion != sessionLeafRevocationSchemaVersion {
		return nil, fmt.Errorf("%w: schema_version must be %d", errSessionLeafRevocationConfig, sessionLeafRevocationSchemaVersion)
	}
	revision := strings.TrimSpace(definition.Revision)
	if revision == "" || revision != definition.Revision || len(revision) > sessionProxyMTLSMaxRevisionBytes {
		return nil, fmt.Errorf("%w: revision must be 1..%d trimmed bytes", errSessionLeafRevocationConfig, sessionProxyMTLSMaxRevisionBytes)
	}
	if len(definition.RevokedSPKISHA256) > sessionLeafRevocationMaxEntries {
		return nil, fmt.Errorf("%w: revoked_spki_sha256 exceeds %d entries", errSessionLeafRevocationConfig, sessionLeafRevocationMaxEntries)
	}

	revoked := make(map[[sha256.Size]byte]struct{}, len(definition.RevokedSPKISHA256))
	for index, raw := range definition.RevokedSPKISHA256 {
		if len(raw) != sha256.Size*2 || strings.ToLower(raw) != raw || strings.TrimSpace(raw) != raw {
			return nil, fmt.Errorf("%w: revoked_spki_sha256[%d] must be exactly 64 lowercase hex characters", errSessionLeafRevocationConfig, index)
		}
		decoded, err := hex.DecodeString(raw)
		if err != nil || len(decoded) != sha256.Size {
			return nil, fmt.Errorf("%w: revoked_spki_sha256[%d] is invalid", errSessionLeafRevocationConfig, index)
		}
		var identifier [sha256.Size]byte
		copy(identifier[:], decoded)
		revoked[identifier] = struct{}{}
	}
	return &sessionLeafRevocationSnapshot{
		revision:        revision,
		revoked:         revoked,
		authorityDigest: sessionLeafRevocationAuthorityDigest(revoked),
	}, nil
}

func sessionLeafRevocationAuthorityDigest(revoked map[[sha256.Size]byte]struct{}) [sha256.Size]byte {
	identifiers := make([][sha256.Size]byte, 0, len(revoked))
	for identifier := range revoked {
		identifiers = append(identifiers, identifier)
	}
	sort.Slice(identifiers, func(i, j int) bool {
		return bytes.Compare(identifiers[i][:], identifiers[j][:]) < 0
	})
	hasher := sha256.New()
	_, _ = hasher.Write([]byte("astrahold/session-leaf-revocation-authority/v1\x00"))
	for _, identifier := range identifiers {
		_, _ = hasher.Write(identifier[:])
	}
	var digest [sha256.Size]byte
	copy(digest[:], hasher.Sum(nil))
	return digest
}

func sessionProxyLeafCredentialIdentifier(state tls.ConnectionState) ([sha256.Size]byte, error) {
	if len(state.PeerCertificates) == 0 || len(state.VerifiedChains) == 0 || state.PeerCertificates[0] == nil {
		return [sha256.Size]byte{}, fmt.Errorf("%w: trusted proxy client certificate was not verified", errSessionLeafRevocationConfig)
	}
	leaf := state.PeerCertificates[0]
	if leaf.IsCA || len(leaf.RawSubjectPublicKeyInfo) == 0 {
		return [sha256.Size]byte{}, fmt.Errorf("%w: trusted proxy leaf SPKI is unavailable", errSessionLeafRevocationConfig)
	}
	return sha256.Sum256(leaf.RawSubjectPublicKeyInfo), nil
}

func (s *sessionLeafRevocationSnapshot) revokedCredential(identifier [sha256.Size]byte) bool {
	if s == nil {
		return false
	}
	_, revoked := s.revoked[identifier]
	return revoked
}

func (r *reloadableSessionLeafRevocation) currentSnapshot() (*sessionLeafRevocationSnapshot, uint64) {
	if r == nil {
		return nil, 0
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.current, r.generation
}

func (r *reloadableSessionLeafRevocation) revokedCredential(identifier [sha256.Size]byte) bool {
	snapshot, _ := r.currentSnapshot()
	return snapshot != nil && snapshot.revokedCredential(identifier)
}

func (r *reloadableSessionLeafRevocation) Snapshot() sessionLeafRevocationMetadata {
	snapshot, generation := r.currentSnapshot()
	if snapshot == nil {
		return sessionLeafRevocationMetadata{}
	}
	return sessionLeafRevocationMetadata{
		Generation:             generation,
		Revision:               snapshot.revision,
		RevokedCredentialCount: len(snapshot.revoked),
	}
}

func (r *reloadableSessionLeafRevocation) Reload() (sessionLeafRevocationReloadResult, error) {
	if r == nil || strings.TrimSpace(r.definitionFile) == "" {
		return sessionLeafRevocationReloadResult{}, errSessionLeafRevocationConfig
	}
	candidate, err := loadSessionLeafRevocationSnapshot(r.definitionFile)
	if err != nil {
		return sessionLeafRevocationReloadResult{}, err
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if r.current == nil {
		return sessionLeafRevocationReloadResult{}, errSessionLeafRevocationConfig
	}
	previous := r.current
	previousGeneration := r.generation
	if previous.authorityDigest == candidate.authorityDigest {
		return sessionLeafRevocationReloadResult{
			PreviousGeneration:     previousGeneration,
			Generation:             previousGeneration,
			PreviousRevision:       previous.revision,
			Revision:               previous.revision,
			RevokedCredentialCount: len(previous.revoked),
			AuthorityChanged:       false,
		}, nil
	}
	if r.generation == ^uint64(0) {
		return sessionLeafRevocationReloadResult{}, errSessionLeafRevocationConfig
	}
	r.current = candidate
	r.generation++
	return sessionLeafRevocationReloadResult{
		PreviousGeneration:     previousGeneration,
		Generation:             r.generation,
		PreviousRevision:       previous.revision,
		Revision:               candidate.revision,
		RevokedCredentialCount: len(candidate.revoked),
		AuthorityChanged:       true,
	}, nil
}

func (r *reloadableSessionEdgePolicy) verifiedLeafCredential(state tls.ConnectionState) ([sha256.Size]byte, bool, error) {
	identifier, err := sessionProxyLeafCredentialIdentifier(state)
	if err != nil {
		if r != nil && r.leafRevocation != nil {
			return [sha256.Size]byte{}, false, err
		}
		return [sha256.Size]byte{}, false, nil
	}
	if r != nil && r.leafRevocation != nil && r.leafRevocation.revokedCredential(identifier) {
		return [sha256.Size]byte{}, false, errSessionLeafCredentialRevoked
	}
	return identifier, true, nil
}

func (r *reloadableSessionEdgePolicy) bindVerifiedConnectionCredential(remote string, snapshot *sessionEdgePolicySnapshot, generation uint64, bindingIndex int, matchedDNS []string, credentialID [sha256.Size]byte, credentialPinned bool) error {
	if r == nil || strings.TrimSpace(remote) == "" || snapshot == nil || generation == 0 || bindingIndex < 0 || bindingIndex >= len(snapshot.bindings) {
		return errSessionEdgePolicyConfig
	}
	if r.leafRevocation != nil {
		if !credentialPinned || r.leafRevocation.revokedCredential(credentialID) {
			return errSessionLeafCredentialRevoked
		}
	}
	allowedDNS := snapshot.bindings[bindingIndex].allowedDNS
	pinned := make(map[string]struct{}, len(matchedDNS))
	for _, candidate := range matchedDNS {
		normalized, err := normalizeSessionProxyDNSIdentity(candidate)
		if err != nil {
			continue
		}
		if _, allowed := allowedDNS[normalized]; !allowed {
			continue
		}
		pinned[normalized] = struct{}{}
	}
	identities := sessionEdgePolicySortedIdentities(pinned)
	if len(identities) == 0 || len(identities) > sessionEdgePolicyMaxIdentities {
		return errSessionEdgePolicyConfig
	}

	r.connectionsMu.Lock()
	defer r.connectionsMu.Unlock()
	if r.leafRevocation != nil && (!credentialPinned || r.leafRevocation.revokedCredential(credentialID)) {
		return errSessionLeafCredentialRevoked
	}
	r.connections[remote] = sessionEdgePolicyConnection{
		snapshot:         snapshot,
		generation:       generation,
		bindingIndex:     bindingIndex,
		matchedDNS:       append([]string(nil), identities...),
		credentialID:     credentialID,
		credentialPinned: credentialPinned,
	}
	return nil
}

func (r *reloadableSessionEdgePolicy) connectionCredentialRevoked(connection sessionEdgePolicyConnection) bool {
	if r == nil || r.leafRevocation == nil {
		return false
	}
	if !connection.credentialPinned {
		return true
	}
	return r.leafRevocation.revokedCredential(connection.credentialID)
}

func (r *reloadableSessionEdgePolicy) LeafRevocationSnapshot() sessionLeafRevocationMetadata {
	if r == nil || r.leafRevocation == nil {
		return sessionLeafRevocationMetadata{}
	}
	return r.leafRevocation.Snapshot()
}

func (r *reloadableSessionEdgePolicy) ReloadLeafRevocation() (sessionLeafRevocationReloadResult, error) {
	if r == nil || r.leafRevocation == nil {
		return sessionLeafRevocationReloadResult{}, errSessionLeafRevocationConfig
	}
	return r.leafRevocation.Reload()
}

func sessionLeafRevocationMetadataForAttributor(a *sessionSourceAttributor) sessionLeafRevocationMetadata {
	if a == nil || a.edgePolicy == nil {
		return sessionLeafRevocationMetadata{}
	}
	return a.edgePolicy.LeafRevocationSnapshot()
}
