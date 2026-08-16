package main

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	sessionProxyMTLSSchemaVersion       uint16 = 1
	sessionProxyMTLSMaxDefinitionBytes        = 64 * 1024
	sessionProxyMTLSMaxCABundleBytes           = 256 * 1024
	sessionProxyMTLSMaxRoots                   = 16
	sessionProxyMTLSMaxDNSNames                = 32
	sessionProxyMTLSMaxRevisionBytes           = 128
	sessionProxyMTLSMaxDNSNameBytes            = 253
)

var sessionLoginTrustedProxyMTLSFile = flag.String(
	"session-login-trusted-proxy-mtls-file",
	"",
	"Optional schema-v1 trusted reverse-proxy mTLS policy; requires the F.17 trusted proxy allowlist/header pair and reloads on SIGHUP",
)

var errSessionProxyMTLSConfig = errors.New("worldd: invalid trusted proxy mTLS config")

type sessionProxyMTLSDefinition struct {
	SchemaVersion uint16   `json:"schema_version"`
	Revision      string   `json:"revision"`
	ClientCAFile  string   `json:"client_ca_file"`
	DNSNames      []string `json:"dns_names"`
}

type sessionProxyMTLSSnapshot struct {
	revision   string
	roots      *x509.CertPool
	rootCount  int
	allowedDNS map[string]struct{}
}

type sessionProxyMTLSMetadata struct {
	Generation    uint64
	Revision      string
	RootCount     int
	IdentityCount int
}

type sessionProxyMTLSReloadResult struct {
	PreviousGeneration uint64
	Generation         uint64
	PreviousRevision   string
	Revision           string
	RootCount          int
	IdentityCount      int
}

// reloadableSessionProxyMTLS publishes immutable client-trust generations to
// new TLS handshakes. A handshake snapshots one generation; later reloads do
// not rewrite the authenticated state of an established TLS connection.
type reloadableSessionProxyMTLS struct {
	mu             sync.RWMutex
	definitionFile string
	current        *sessionProxyMTLSSnapshot
	generation     uint64
	now            func() time.Time
}

func newReloadableSessionProxyMTLS(definitionFile string, now func() time.Time) (*reloadableSessionProxyMTLS, error) {
	definitionFile = strings.TrimSpace(definitionFile)
	if definitionFile == "" {
		return nil, errSessionProxyMTLSConfig
	}
	if now == nil {
		now = time.Now
	}
	snapshot, err := loadSessionProxyMTLSSnapshot(definitionFile, now().UTC())
	if err != nil {
		return nil, err
	}
	return &reloadableSessionProxyMTLS{
		definitionFile: definitionFile,
		current:        snapshot,
		generation:     1,
		now:            now,
	}, nil
}

func loadSessionProxyMTLSSnapshot(definitionFile string, now time.Time) (*sessionProxyMTLSSnapshot, error) {
	data, err := readSessionProxyMTLSFile(definitionFile, sessionProxyMTLSMaxDefinitionBytes)
	if err != nil {
		return nil, fmt.Errorf("%w: policy file: %v", errSessionProxyMTLSConfig, err)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var definition sessionProxyMTLSDefinition
	if err := decoder.Decode(&definition); err != nil {
		return nil, fmt.Errorf("%w: decode policy: %v", errSessionProxyMTLSConfig, err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return nil, fmt.Errorf("%w: trailing JSON value", errSessionProxyMTLSConfig)
		}
		return nil, fmt.Errorf("%w: trailing policy data: %v", errSessionProxyMTLSConfig, err)
	}
	if definition.SchemaVersion != sessionProxyMTLSSchemaVersion {
		return nil, fmt.Errorf("%w: schema_version must be %d", errSessionProxyMTLSConfig, sessionProxyMTLSSchemaVersion)
	}
	revision := strings.TrimSpace(definition.Revision)
	if revision == "" || revision != definition.Revision || len(revision) > sessionProxyMTLSMaxRevisionBytes {
		return nil, fmt.Errorf("%w: revision must be 1..%d trimmed bytes", errSessionProxyMTLSConfig, sessionProxyMTLSMaxRevisionBytes)
	}
	caFile := strings.TrimSpace(definition.ClientCAFile)
	if caFile == "" || caFile != definition.ClientCAFile {
		return nil, fmt.Errorf("%w: client_ca_file must be a non-empty trimmed path", errSessionProxyMTLSConfig)
	}
	if !filepath.IsAbs(caFile) {
		caFile = filepath.Join(filepath.Dir(definitionFile), caFile)
	}
	if len(definition.DNSNames) == 0 || len(definition.DNSNames) > sessionProxyMTLSMaxDNSNames {
		return nil, fmt.Errorf("%w: dns_names must contain 1..%d entries", errSessionProxyMTLSConfig, sessionProxyMTLSMaxDNSNames)
	}
	allowedDNS := make(map[string]struct{}, len(definition.DNSNames))
	for _, name := range definition.DNSNames {
		normalized, err := normalizeSessionProxyDNSIdentity(name)
		if err != nil {
			return nil, err
		}
		allowedDNS[normalized] = struct{}{}
	}
	if len(allowedDNS) == 0 {
		return nil, fmt.Errorf("%w: dns_names is empty", errSessionProxyMTLSConfig)
	}
	roots, rootCount, err := loadSessionProxyMTLSRoots(caFile, now)
	if err != nil {
		return nil, err
	}
	return &sessionProxyMTLSSnapshot{
		revision:   revision,
		roots:      roots,
		rootCount:  rootCount,
		allowedDNS: allowedDNS,
	}, nil
}

func readSessionProxyMTLSFile(path string, maxBytes int64) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("%q is not a regular file", path)
	}
	if info.Size() < 0 || info.Size() > maxBytes {
		return nil, fmt.Errorf("%q exceeds %d bytes", path, maxBytes)
	}
	data, err := io.ReadAll(io.LimitReader(file, maxBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > maxBytes {
		return nil, fmt.Errorf("%q exceeds %d bytes", path, maxBytes)
	}
	return data, nil
}

func loadSessionProxyMTLSRoots(path string, now time.Time) (*x509.CertPool, int, error) {
	data, err := readSessionProxyMTLSFile(path, sessionProxyMTLSMaxCABundleBytes)
	if err != nil {
		return nil, 0, fmt.Errorf("%w: client CA bundle: %v", errSessionProxyMTLSConfig, err)
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	roots := x509.NewCertPool()
	rootCount := 0
	remaining := data
	for len(bytes.TrimSpace(remaining)) > 0 {
		block, rest := pem.Decode(remaining)
		if block == nil {
			return nil, 0, fmt.Errorf("%w: client CA bundle contains invalid PEM data", errSessionProxyMTLSConfig)
		}
		if block.Type != "CERTIFICATE" {
			return nil, 0, fmt.Errorf("%w: client CA bundle contains non-certificate PEM block", errSessionProxyMTLSConfig)
		}
		certificate, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			return nil, 0, fmt.Errorf("%w: parse client CA: %v", errSessionProxyMTLSConfig, err)
		}
		if !certificate.BasicConstraintsValid || !certificate.IsCA {
			return nil, 0, fmt.Errorf("%w: client trust anchor is not a CA", errSessionProxyMTLSConfig)
		}
		if certificate.KeyUsage != 0 && certificate.KeyUsage&x509.KeyUsageCertSign == 0 {
			return nil, 0, fmt.Errorf("%w: client CA does not permit certificate signing", errSessionProxyMTLSConfig)
		}
		if now.Before(certificate.NotBefore) || !now.Before(certificate.NotAfter) {
			return nil, 0, fmt.Errorf("%w: client CA is not currently valid", errSessionProxyMTLSConfig)
		}
		rootCount++
		if rootCount > sessionProxyMTLSMaxRoots {
			return nil, 0, fmt.Errorf("%w: client CA bundle exceeds %d roots", errSessionProxyMTLSConfig, sessionProxyMTLSMaxRoots)
		}
		roots.AddCert(certificate)
		remaining = rest
	}
	if rootCount == 0 {
		return nil, 0, fmt.Errorf("%w: client CA bundle contains no roots", errSessionProxyMTLSConfig)
	}
	return roots, rootCount, nil
}

func normalizeSessionProxyDNSIdentity(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > sessionProxyMTLSMaxDNSNameBytes || strings.HasSuffix(value, ".") || strings.Contains(value, "*") {
		return "", fmt.Errorf("%w: invalid proxy DNS identity", errSessionProxyMTLSConfig)
	}
	if _, err := netip.ParseAddr(value); err == nil {
		return "", fmt.Errorf("%w: proxy DNS identity must not be an IP address", errSessionProxyMTLSConfig)
	}
	value = strings.ToLower(value)
	labels := strings.Split(value, ".")
	for _, label := range labels {
		if len(label) == 0 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return "", fmt.Errorf("%w: invalid proxy DNS identity", errSessionProxyMTLSConfig)
		}
		for _, character := range label {
			if (character >= 'a' && character <= 'z') || (character >= '0' && character <= '9') || character == '-' {
				continue
			}
			return "", fmt.Errorf("%w: invalid proxy DNS identity", errSessionProxyMTLSConfig)
		}
	}
	return value, nil
}

func (s *sessionProxyMTLSSnapshot) verifyConnection(state tls.ConnectionState) error {
	if s == nil || s.roots == nil || len(s.allowedDNS) == 0 || len(state.PeerCertificates) == 0 || len(state.VerifiedChains) == 0 {
		return fmt.Errorf("%w: trusted proxy client certificate was not verified", errSessionProxyMTLSConfig)
	}
	leaf := state.PeerCertificates[0]
	if leaf == nil || leaf.IsCA {
		return fmt.Errorf("%w: trusted proxy leaf certificate is invalid", errSessionProxyMTLSConfig)
	}
	for _, candidate := range leaf.DNSNames {
		normalized, err := normalizeSessionProxyDNSIdentity(candidate)
		if err != nil {
			continue
		}
		if _, allowed := s.allowedDNS[normalized]; allowed {
			return nil
		}
	}
	return fmt.Errorf("%w: trusted proxy certificate DNS identity is not allowlisted", errSessionProxyMTLSConfig)
}

func (r *reloadableSessionProxyMTLS) currentSnapshot() (*sessionProxyMTLSSnapshot, uint64) {
	if r == nil {
		return nil, 0
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.current, r.generation
}

func (r *reloadableSessionProxyMTLS) TLSConfig(certificate *reloadableTLSCertificate) (*tls.Config, error) {
	if r == nil || certificate == nil {
		return nil, errSessionProxyMTLSConfig
	}
	snapshot, _ := r.currentSnapshot()
	if snapshot == nil || snapshot.roots == nil || len(snapshot.allowedDNS) == 0 {
		return nil, errSessionProxyMTLSConfig
	}
	config := certificate.TLSConfig()
	if config == nil {
		return nil, errSessionProxyMTLSConfig
	}
	config.ClientAuth = tls.RequireAndVerifyClientCert
	config.ClientCAs = snapshot.roots
	config.VerifyConnection = snapshot.verifyConnection
	config.GetConfigForClient = nil
	return config, nil
}

func (r *reloadableSessionProxyMTLS) Snapshot() sessionProxyMTLSMetadata {
	snapshot, generation := r.currentSnapshot()
	if snapshot == nil {
		return sessionProxyMTLSMetadata{}
	}
	return sessionProxyMTLSMetadata{
		Generation:    generation,
		Revision:      snapshot.revision,
		RootCount:     snapshot.rootCount,
		IdentityCount: len(snapshot.allowedDNS),
	}
}

func (r *reloadableSessionProxyMTLS) Reload() (sessionProxyMTLSReloadResult, error) {
	if r == nil || r.now == nil || strings.TrimSpace(r.definitionFile) == "" {
		return sessionProxyMTLSReloadResult{}, errSessionProxyMTLSConfig
	}
	candidate, err := loadSessionProxyMTLSSnapshot(r.definitionFile, r.now().UTC())
	if err != nil {
		return sessionProxyMTLSReloadResult{}, err
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if r.current == nil || r.generation == ^uint64(0) {
		return sessionProxyMTLSReloadResult{}, errSessionProxyMTLSConfig
	}
	previousGeneration := r.generation
	previousRevision := r.current.revision
	r.current = candidate
	r.generation++
	return sessionProxyMTLSReloadResult{
		PreviousGeneration: previousGeneration,
		Generation:         r.generation,
		PreviousRevision:   previousRevision,
		Revision:           candidate.revision,
		RootCount:          candidate.rootCount,
		IdentityCount:      len(candidate.allowedDNS),
	}, nil
}
