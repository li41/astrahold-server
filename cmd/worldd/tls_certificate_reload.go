package main

import (
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
)

var errTLSCertificateReloadConfig = errors.New("worldd: invalid TLS certificate reload config")

type tlsCertificateSnapshot struct {
	Generation  uint64
	Fingerprint string
	NotAfter    time.Time
}

type tlsCertificateReloadResult struct {
	PreviousGeneration  uint64
	Generation          uint64
	PreviousFingerprint string
	Fingerprint         string
	NotAfter            time.Time
}

// reloadableTLSCertificate publishes immutable certificate generations to TLS
// handshakes. A failed reload never mutates the current generation. Once
// GetCertificate returns a generation to a handshake, later reloads cannot
// change the certificate already selected for that handshake or an established
// TLS connection.
type reloadableTLSCertificate struct {
	mu          sync.RWMutex
	certFile    string
	keyFile     string
	current     *tls.Certificate
	generation  uint64
	fingerprint string
	notAfter    time.Time
	now         func() time.Time
}

func newReloadableTLSCertificate(certFile, keyFile string, now func() time.Time) (*reloadableTLSCertificate, error) {
	certFile = strings.TrimSpace(certFile)
	keyFile = strings.TrimSpace(keyFile)
	if certFile == "" || keyFile == "" {
		return nil, errTLSCertificateReloadConfig
	}
	if now == nil {
		now = time.Now
	}
	certificate, fingerprint, notAfter, err := loadValidatedTLSCertificate(certFile, keyFile, now().UTC())
	if err != nil {
		return nil, err
	}
	return &reloadableTLSCertificate{
		certFile:    certFile,
		keyFile:     keyFile,
		current:     certificate,
		generation:  1,
		fingerprint: fingerprint,
		notAfter:    notAfter,
		now:         now,
	}, nil
}

func loadValidatedTLSCertificate(certFile, keyFile string, now time.Time) (*tls.Certificate, string, time.Time, error) {
	certificate, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return nil, "", time.Time{}, fmt.Errorf("%w: load certificate/key pair: %v", errTLSCertificateReloadConfig, err)
	}
	if len(certificate.Certificate) == 0 {
		return nil, "", time.Time{}, fmt.Errorf("%w: certificate chain has no leaf", errTLSCertificateReloadConfig)
	}
	leaf, err := x509.ParseCertificate(certificate.Certificate[0])
	if err != nil {
		return nil, "", time.Time{}, fmt.Errorf("%w: parse leaf certificate: %v", errTLSCertificateReloadConfig, err)
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	if now.Before(leaf.NotBefore) || !now.Before(leaf.NotAfter) {
		return nil, "", time.Time{}, fmt.Errorf("%w: leaf certificate is not currently valid", errTLSCertificateReloadConfig)
	}
	if len(leaf.ExtKeyUsage) > 0 {
		serverUsage := false
		for _, usage := range leaf.ExtKeyUsage {
			if usage == x509.ExtKeyUsageServerAuth || usage == x509.ExtKeyUsageAny {
				serverUsage = true
				break
			}
		}
		if !serverUsage {
			return nil, "", time.Time{}, fmt.Errorf("%w: leaf certificate does not permit server authentication", errTLSCertificateReloadConfig)
		}
	}
	certificate.Leaf = leaf
	digest := sha256.Sum256(leaf.Raw)
	return &certificate, hex.EncodeToString(digest[:]), leaf.NotAfter.UTC(), nil
}

func (r *reloadableTLSCertificate) GetCertificate(*tls.ClientHelloInfo) (*tls.Certificate, error) {
	if r == nil {
		return nil, errTLSCertificateReloadConfig
	}
	r.mu.RLock()
	certificate := r.current
	r.mu.RUnlock()
	if certificate == nil {
		return nil, errTLSCertificateReloadConfig
	}
	return certificate, nil
}

func (r *reloadableTLSCertificate) TLSConfig() *tls.Config {
	if r == nil {
		return nil
	}
	return &tls.Config{
		MinVersion:     tls.VersionTLS13,
		GetCertificate: r.GetCertificate,
	}
}

func (r *reloadableTLSCertificate) Snapshot() tlsCertificateSnapshot {
	if r == nil {
		return tlsCertificateSnapshot{}
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	return tlsCertificateSnapshot{
		Generation:  r.generation,
		Fingerprint: r.fingerprint,
		NotAfter:    r.notAfter,
	}
}

func (r *reloadableTLSCertificate) Reload() (tlsCertificateReloadResult, error) {
	if r == nil || r.now == nil {
		return tlsCertificateReloadResult{}, errTLSCertificateReloadConfig
	}
	certificate, fingerprint, notAfter, err := loadValidatedTLSCertificate(r.certFile, r.keyFile, r.now().UTC())
	if err != nil {
		return tlsCertificateReloadResult{}, err
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if r.current == nil || r.generation == ^uint64(0) {
		return tlsCertificateReloadResult{}, errTLSCertificateReloadConfig
	}
	previousGeneration := r.generation
	previousFingerprint := r.fingerprint
	r.current = certificate
	r.generation++
	r.fingerprint = fingerprint
	r.notAfter = notAfter
	return tlsCertificateReloadResult{
		PreviousGeneration:  previousGeneration,
		Generation:          r.generation,
		PreviousFingerprint: previousFingerprint,
		Fingerprint:         fingerprint,
		NotAfter:            notAfter,
	}, nil
}
