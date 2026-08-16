package main

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/netip"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	sessionEdgePolicySchemaVersion        uint16 = 1
	sessionEdgePolicyMaxDefinitionBytes         = 64 * 1024
	sessionEdgePolicyMaxBindings                = 32
	sessionEdgePolicyMaxIdentities              = 64
)

var sessionLoginTrustedProxyEdgePolicyFile = flag.String(
	"session-login-trusted-proxy-edge-policy-file",
	"",
	"Optional schema-v1 F.19 trusted reverse-proxy edge policy; atomically binds network prefixes, forwarding mode, client CA, and exact DNS identities and reloads on SIGHUP",
)

var errSessionEdgePolicyConfig = errors.New("worldd: invalid trusted proxy edge policy config")

type sessionEdgePolicyDefinition struct {
	SchemaVersion    uint16                               `json:"schema_version"`
	Revision         string                               `json:"revision"`
	ForwardedHeader  string                               `json:"forwarded_header"`
	ClientCAFile     string                               `json:"client_ca_file"`
	Bindings         []sessionEdgePolicyBindingDefinition `json:"bindings"`
}

type sessionEdgePolicyBindingDefinition struct {
	Prefixes []string `json:"prefixes"`
	DNSNames []string `json:"dns_names"`
}

type sessionEdgePolicyBinding struct {
	prefixes   []netip.Prefix
	allowedDNS map[string]struct{}
}

type sessionEdgePolicySnapshot struct {
	revision        string
	mode            sessionSourceHeaderMode
	headerName      string
	roots           *x509.CertPool
	rootCount       int
	bindings        []sessionEdgePolicyBinding
	trustedPrefixes []netip.Prefix
	identityCount   int
}

type sessionEdgePolicyMetadata struct {
	Generation    uint64
	Revision      string
	HeaderMode    string
	RootCount     int
	BindingCount  int
	PrefixCount   int
	IdentityCount int
}

type sessionEdgePolicyReloadResult struct {
	PreviousGeneration uint64
	Generation         uint64
	PreviousRevision   string
	Revision           string
	PreviousHeaderMode string
	HeaderMode         string
	RootCount          int
	BindingCount       int
	PrefixCount        int
	IdentityCount      int
}

type sessionEdgePolicyConnection struct {
	snapshot     *sessionEdgePolicySnapshot
	generation   uint64
	bindingIndex int
}

// reloadableSessionEdgePolicy publishes immutable network+identity generations.
// A trusted proxy TLS connection is pinned to the snapshot that authenticated
// its handshake, so later reloads cannot mix old TLS identity state with a new
// forwarding mode or trusted-prefix set on the same established connection.
type reloadableSessionEdgePolicy struct {
	mu             sync.RWMutex
	definitionFile string
	current        *sessionEdgePolicySnapshot
	generation     uint64
	now            func() time.Time

	connectionsMu sync.RWMutex
	connections   map[string]sessionEdgePolicyConnection
}

func newReloadableSessionEdgePolicy(definitionFile string, now func() time.Time) (*reloadableSessionEdgePolicy, error) {
	definitionFile = strings.TrimSpace(definitionFile)
	if definitionFile == "" {
		return nil, errSessionEdgePolicyConfig
	}
	if now == nil {
		now = time.Now
	}
	snapshot, err := loadSessionEdgePolicySnapshot(definitionFile, now().UTC())
	if err != nil {
		return nil, err
	}
	return &reloadableSessionEdgePolicy{
		definitionFile: definitionFile,
		current:        snapshot,
		generation:     1,
		now:            now,
		connections:    make(map[string]sessionEdgePolicyConnection),
	}, nil
}

func loadSessionEdgePolicySnapshot(definitionFile string, now time.Time) (*sessionEdgePolicySnapshot, error) {
	data, err := readSessionProxyMTLSFile(definitionFile, sessionEdgePolicyMaxDefinitionBytes)
	if err != nil {
		return nil, fmt.Errorf("%w: policy file: %v", errSessionEdgePolicyConfig, err)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var definition sessionEdgePolicyDefinition
	if err := decoder.Decode(&definition); err != nil {
		return nil, fmt.Errorf("%w: decode policy: %v", errSessionEdgePolicyConfig, err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return nil, fmt.Errorf("%w: trailing JSON value", errSessionEdgePolicyConfig)
		}
		return nil, fmt.Errorf("%w: trailing policy data: %v", errSessionEdgePolicyConfig, err)
	}
	if definition.SchemaVersion != sessionEdgePolicySchemaVersion {
		return nil, fmt.Errorf("%w: schema_version must be %d", errSessionEdgePolicyConfig, sessionEdgePolicySchemaVersion)
	}
	revision := strings.TrimSpace(definition.Revision)
	if revision == "" || revision != definition.Revision || len(revision) > sessionProxyMTLSMaxRevisionBytes {
		return nil, fmt.Errorf("%w: revision must be 1..%d trimmed bytes", errSessionEdgePolicyConfig, sessionProxyMTLSMaxRevisionBytes)
	}
	mode, headerName, err := parseSessionEdgePolicyHeaderMode(definition.ForwardedHeader)
	if err != nil {
		return nil, err
	}
	caFile := strings.TrimSpace(definition.ClientCAFile)
	if caFile == "" || caFile != definition.ClientCAFile {
		return nil, fmt.Errorf("%w: client_ca_file must be a non-empty trimmed path", errSessionEdgePolicyConfig)
	}
	if !filepath.IsAbs(caFile) {
		caFile = filepath.Join(filepath.Dir(definitionFile), caFile)
	}
	roots, rootCount, err := loadSessionProxyMTLSRoots(caFile, now)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", errSessionEdgePolicyConfig, err)
	}
	if len(definition.Bindings) == 0 || len(definition.Bindings) > sessionEdgePolicyMaxBindings {
		return nil, fmt.Errorf("%w: bindings must contain 1..%d entries", errSessionEdgePolicyConfig, sessionEdgePolicyMaxBindings)
	}

	bindings := make([]sessionEdgePolicyBinding, 0, len(definition.Bindings))
	trustedPrefixes := make([]netip.Prefix, 0)
	identityCount := 0
	for bindingIndex, item := range definition.Bindings {
		if len(item.Prefixes) == 0 {
			return nil, fmt.Errorf("%w: binding[%d] prefixes must not be empty", errSessionEdgePolicyConfig, bindingIndex)
		}
		if len(item.DNSNames) == 0 {
			return nil, fmt.Errorf("%w: binding[%d] dns_names must not be empty", errSessionEdgePolicyConfig, bindingIndex)
		}
		binding := sessionEdgePolicyBinding{
			prefixes:   make([]netip.Prefix, 0, len(item.Prefixes)),
			allowedDNS: make(map[string]struct{}, len(item.DNSNames)),
		}
		seenPrefixes := make(map[string]struct{}, len(item.Prefixes))
		for _, rawPrefix := range item.Prefixes {
			prefix, err := parseSessionTrustedProxyPrefix(rawPrefix)
			if err != nil {
				return nil, fmt.Errorf("%w: binding[%d]: %v", errSessionEdgePolicyConfig, bindingIndex, err)
			}
			key := prefix.String()
			if _, exists := seenPrefixes[key]; exists {
				continue
			}
			for _, existing := range trustedPrefixes {
				if prefix.Overlaps(existing) {
					return nil, fmt.Errorf("%w: binding[%d] prefix %s overlaps another binding prefix %s", errSessionEdgePolicyConfig, bindingIndex, prefix, existing)
				}
			}
			seenPrefixes[key] = struct{}{}
			binding.prefixes = append(binding.prefixes, prefix)
			trustedPrefixes = append(trustedPrefixes, prefix)
			if len(trustedPrefixes) > sessionSourceAttributionMaxTrustedPrefixes {
				return nil, fmt.Errorf("%w: total trusted prefixes exceed %d", errSessionEdgePolicyConfig, sessionSourceAttributionMaxTrustedPrefixes)
			}
		}
		if len(binding.prefixes) == 0 {
			return nil, fmt.Errorf("%w: binding[%d] prefixes are empty after normalization", errSessionEdgePolicyConfig, bindingIndex)
		}
		for _, rawName := range item.DNSNames {
			name, err := normalizeSessionProxyDNSIdentity(rawName)
			if err != nil {
				return nil, fmt.Errorf("%w: binding[%d]: %v", errSessionEdgePolicyConfig, bindingIndex, err)
			}
			if _, exists := binding.allowedDNS[name]; exists {
				continue
			}
			binding.allowedDNS[name] = struct{}{}
			identityCount++
			if identityCount > sessionEdgePolicyMaxIdentities {
				return nil, fmt.Errorf("%w: total DNS identities exceed %d", errSessionEdgePolicyConfig, sessionEdgePolicyMaxIdentities)
			}
		}
		if len(binding.allowedDNS) == 0 {
			return nil, fmt.Errorf("%w: binding[%d] dns_names are empty after normalization", errSessionEdgePolicyConfig, bindingIndex)
		}
		bindings = append(bindings, binding)
	}

	return &sessionEdgePolicySnapshot{
		revision:        revision,
		mode:            mode,
		headerName:      headerName,
		roots:           roots,
		rootCount:       rootCount,
		bindings:        bindings,
		trustedPrefixes: trustedPrefixes,
		identityCount:   identityCount,
	}, nil
}

func parseSessionEdgePolicyHeaderMode(value string) (sessionSourceHeaderMode, string, error) {
	value = strings.ToLower(strings.TrimSpace(value))
	switch value {
	case "x-forwarded-for":
		return sessionSourceHeaderXForwardedFor, "X-Forwarded-For", nil
	case "forwarded":
		return sessionSourceHeaderForwarded, "Forwarded", nil
	default:
		return sessionSourceHeaderNone, "", fmt.Errorf("%w: forwarded_header must be x-forwarded-for or forwarded", errSessionEdgePolicyConfig)
	}
}

func sessionSourceHeaderModeString(mode sessionSourceHeaderMode) string {
	switch mode {
	case sessionSourceHeaderXForwardedFor:
		return "x-forwarded-for"
	case sessionSourceHeaderForwarded:
		return "forwarded"
	default:
		return "invalid"
	}
}

func (s *sessionEdgePolicySnapshot) trusted(address netip.Addr) bool {
	if s == nil || !address.IsValid() {
		return false
	}
	address = address.Unmap()
	for _, prefix := range s.trustedPrefixes {
		if prefix.Contains(address) {
			return true
		}
	}
	return false
}

func (s *sessionEdgePolicySnapshot) bindingForPeer(address netip.Addr) (int, bool) {
	if s == nil || !address.IsValid() {
		return 0, false
	}
	address = address.Unmap()
	for bindingIndex, binding := range s.bindings {
		for _, prefix := range binding.prefixes {
			if prefix.Contains(address) {
				return bindingIndex, true
			}
		}
	}
	return 0, false
}

func (s *sessionEdgePolicySnapshot) verifyConnection(state tls.ConnectionState, bindingIndex int) error {
	if s == nil || s.roots == nil || bindingIndex < 0 || bindingIndex >= len(s.bindings) || len(state.PeerCertificates) == 0 || len(state.VerifiedChains) == 0 {
		return fmt.Errorf("%w: trusted proxy client certificate was not verified", errSessionEdgePolicyConfig)
	}
	leaf := state.PeerCertificates[0]
	if leaf == nil || leaf.IsCA {
		return fmt.Errorf("%w: trusted proxy leaf certificate is invalid", errSessionEdgePolicyConfig)
	}
	allowedDNS := s.bindings[bindingIndex].allowedDNS
	for _, candidate := range leaf.DNSNames {
		normalized, err := normalizeSessionProxyDNSIdentity(candidate)
		if err != nil {
			continue
		}
		if _, allowed := allowedDNS[normalized]; allowed {
			return nil
		}
	}
	return fmt.Errorf("%w: trusted proxy certificate DNS identity does not match the socket binding", errSessionEdgePolicyConfig)
}

func (r *reloadableSessionEdgePolicy) currentSnapshot() (*sessionEdgePolicySnapshot, uint64) {
	if r == nil {
		return nil, 0
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.current, r.generation
}

func (r *reloadableSessionEdgePolicy) TLSConfigForPeer(certificate *reloadableTLSCertificate, remote string, peer netip.Addr) (*tls.Config, bool, error) {
	if r == nil || certificate == nil || strings.TrimSpace(remote) == "" || !peer.IsValid() {
		return nil, false, errSessionEdgePolicyConfig
	}
	snapshot, generation := r.currentSnapshot()
	if snapshot == nil {
		return nil, false, errSessionEdgePolicyConfig
	}
	bindingIndex, trusted := snapshot.bindingForPeer(peer)
	if !trusted {
		return nil, false, nil
	}
	config := certificate.TLSConfig()
	if config == nil {
		return nil, false, errSessionEdgePolicyConfig
	}
	config.ClientAuth = tls.RequireAndVerifyClientCert
	config.ClientCAs = snapshot.roots
	config.VerifyConnection = func(state tls.ConnectionState) error {
		if err := snapshot.verifyConnection(state, bindingIndex); err != nil {
			return err
		}
		r.bindConnection(remote, snapshot, generation, bindingIndex)
		return nil
	}
	config.GetConfigForClient = nil
	return config, true, nil
}

func (r *reloadableSessionEdgePolicy) bindConnection(remote string, snapshot *sessionEdgePolicySnapshot, generation uint64, bindingIndex int) {
	if r == nil || strings.TrimSpace(remote) == "" || snapshot == nil || generation == 0 || bindingIndex < 0 || bindingIndex >= len(snapshot.bindings) {
		return
	}
	r.connectionsMu.Lock()
	r.connections[remote] = sessionEdgePolicyConnection{snapshot: snapshot, generation: generation, bindingIndex: bindingIndex}
	r.connectionsMu.Unlock()
}

func (r *reloadableSessionEdgePolicy) connection(remote string) (sessionEdgePolicyConnection, bool) {
	if r == nil || strings.TrimSpace(remote) == "" {
		return sessionEdgePolicyConnection{}, false
	}
	r.connectionsMu.RLock()
	binding, ok := r.connections[remote]
	r.connectionsMu.RUnlock()
	return binding, ok
}

func (r *reloadableSessionEdgePolicy) releaseConnection(remote string) {
	if r == nil || strings.TrimSpace(remote) == "" {
		return
	}
	r.connectionsMu.Lock()
	delete(r.connections, remote)
	r.connectionsMu.Unlock()
}

func (r *reloadableSessionEdgePolicy) Snapshot() sessionEdgePolicyMetadata {
	snapshot, generation := r.currentSnapshot()
	if snapshot == nil {
		return sessionEdgePolicyMetadata{}
	}
	return sessionEdgePolicyMetadata{
		Generation:    generation,
		Revision:      snapshot.revision,
		HeaderMode:    sessionSourceHeaderModeString(snapshot.mode),
		RootCount:     snapshot.rootCount,
		BindingCount:  len(snapshot.bindings),
		PrefixCount:   len(snapshot.trustedPrefixes),
		IdentityCount: snapshot.identityCount,
	}
}

func (r *reloadableSessionEdgePolicy) Reload() (sessionEdgePolicyReloadResult, error) {
	if r == nil || r.now == nil || strings.TrimSpace(r.definitionFile) == "" {
		return sessionEdgePolicyReloadResult{}, errSessionEdgePolicyConfig
	}
	candidate, err := loadSessionEdgePolicySnapshot(r.definitionFile, r.now().UTC())
	if err != nil {
		return sessionEdgePolicyReloadResult{}, err
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if r.current == nil || r.generation == ^uint64(0) {
		return sessionEdgePolicyReloadResult{}, errSessionEdgePolicyConfig
	}
	previousGeneration := r.generation
	previous := r.current
	r.current = candidate
	r.generation++
	return sessionEdgePolicyReloadResult{
		PreviousGeneration: previousGeneration,
		Generation:         r.generation,
		PreviousRevision:   previous.revision,
		Revision:           candidate.revision,
		PreviousHeaderMode: sessionSourceHeaderModeString(previous.mode),
		HeaderMode:         sessionSourceHeaderModeString(candidate.mode),
		RootCount:          candidate.rootCount,
		BindingCount:       len(candidate.bindings),
		PrefixCount:        len(candidate.trustedPrefixes),
		IdentityCount:      candidate.identityCount,
	}, nil
}
