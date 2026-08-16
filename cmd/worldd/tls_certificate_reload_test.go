package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"io"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestReloadableTLSCertificateValidReloadAndMismatchLKG(t *testing.T) {
	dir := t.TempDir()
	certFile := filepath.Join(dir, "server.crt")
	keyFile := filepath.Join(dir, "server.key")
	writeTLSReloadPair(t, certFile, keyFile, 1, time.Now().Add(-time.Minute), time.Now().Add(time.Hour))

	provider, err := newReloadableTLSCertificate(certFile, keyFile, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	initial := provider.Snapshot()
	if initial.Generation != 1 || initial.Fingerprint == "" {
		t.Fatalf("initial snapshot=%+v", initial)
	}

	secondCert := filepath.Join(dir, "second.crt")
	secondKey := filepath.Join(dir, "second.key")
	writeTLSReloadPair(t, secondCert, secondKey, 2, time.Now().Add(-time.Minute), time.Now().Add(2*time.Hour))
	secondCertPEM, err := os.ReadFile(secondCert)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(certFile, secondCertPEM, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := provider.Reload(); err == nil {
		t.Fatal("mismatched replacement keypair must be rejected")
	}
	if got := provider.Snapshot(); got.Generation != 1 || got.Fingerprint != initial.Fingerprint {
		t.Fatalf("last-known-good changed after rejected reload: %+v", got)
	}

	secondKeyPEM, err := os.ReadFile(secondKey)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keyFile, secondKeyPEM, 0o600); err != nil {
		t.Fatal(err)
	}
	result, err := provider.Reload()
	if err != nil {
		t.Fatal(err)
	}
	if result.PreviousGeneration != 1 || result.Generation != 2 {
		t.Fatalf("reload result=%+v", result)
	}
	if result.Fingerprint == initial.Fingerprint || provider.Snapshot().Generation != 2 {
		t.Fatalf("replacement not published: result=%+v snapshot=%+v", result, provider.Snapshot())
	}
}

func TestReloadableTLSCertificateEstablishedConnectionSurvivesCutover(t *testing.T) {
	dir := t.TempDir()
	certFile := filepath.Join(dir, "server.crt")
	keyFile := filepath.Join(dir, "server.key")
	writeTLSReloadPair(t, certFile, keyFile, 11, time.Now().Add(-time.Minute), time.Now().Add(time.Hour))
	provider, err := newReloadableTLSCertificate(certFile, keyFile, time.Now)
	if err != nil {
		t.Fatal(err)
	}

	base, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	listener := tls.NewListener(base, provider.TLSConfig())
	defer listener.Close()
	serveDone := make(chan error, 1)
	go func() {
		for accepted := 0; accepted < 2; accepted++ {
			connection, err := listener.Accept()
			if err != nil {
				serveDone <- err
				return
			}
			go func(connection net.Conn) {
				defer connection.Close()
				buffer := make([]byte, 1)
				for {
					if _, err := io.ReadFull(connection, buffer); err != nil {
						return
					}
					if _, err := connection.Write(buffer); err != nil {
						return
					}
				}
			}(connection)
		}
		serveDone <- nil
	}()

	clientConfig := &tls.Config{InsecureSkipVerify: true, MinVersion: tls.VersionTLS13} // test-only trust bypass
	first, err := tls.Dial("tcp", listener.Addr().String(), clientConfig)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	if got := first.ConnectionState().PeerCertificates[0].SerialNumber.Int64(); got != 11 {
		t.Fatalf("first serial=%d", got)
	}

	writeTLSReloadPair(t, certFile, keyFile, 22, time.Now().Add(-time.Minute), time.Now().Add(2*time.Hour))
	if _, err := provider.Reload(); err != nil {
		t.Fatal(err)
	}
	if _, err := first.Write([]byte{'A'}); err != nil {
		t.Fatal(err)
	}
	var echo [1]byte
	if _, err := io.ReadFull(first, echo[:]); err != nil || echo[0] != 'A' {
		t.Fatalf("established connection failed after reload: echo=%q err=%v", echo, err)
	}

	second, err := tls.Dial("tcp", listener.Addr().String(), clientConfig)
	if err != nil {
		t.Fatal(err)
	}
	if got := second.ConnectionState().PeerCertificates[0].SerialNumber.Int64(); got != 22 {
		second.Close()
		t.Fatalf("second serial=%d", got)
	}
	if _, err := second.Write([]byte{'B'}); err != nil {
		second.Close()
		t.Fatal(err)
	}
	if _, err := io.ReadFull(second, echo[:]); err != nil || echo[0] != 'B' {
		second.Close()
		t.Fatalf("new connection echo=%q err=%v", echo, err)
	}
	_ = second.Close()
	_ = first.Close()
	_ = listener.Close()
	select {
	case err := <-serveDone:
		if err != nil && !isClosedNetworkError(err) {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("TLS reload test listener did not stop")
	}
}

func TestReloadableTLSCertificateConcurrentReadReload(t *testing.T) {
	dir := t.TempDir()
	certFile := filepath.Join(dir, "server.crt")
	keyFile := filepath.Join(dir, "server.key")
	writeTLSReloadPair(t, certFile, keyFile, 100, time.Now().Add(-time.Minute), time.Now().Add(time.Hour))
	provider, err := newReloadableTLSCertificate(certFile, keyFile, time.Now)
	if err != nil {
		t.Fatal(err)
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 20000; i++ {
			if _, err := provider.GetCertificate(nil); err != nil {
				return
			}
		}
	}()
	for serial := int64(101); serial < 130; serial++ {
		writeTLSReloadPair(t, certFile, keyFile, serial, time.Now().Add(-time.Minute), time.Now().Add(time.Hour))
		if _, err := provider.Reload(); err != nil {
			t.Fatal(err)
		}
	}
	<-done
	if got := provider.Snapshot().Generation; got != 30 {
		t.Fatalf("generation=%d want 30", got)
	}
}

func TestReloadableTLSCertificateRejectsExpiredAndClientOnly(t *testing.T) {
	dir := t.TempDir()
	certFile := filepath.Join(dir, "server.crt")
	keyFile := filepath.Join(dir, "server.key")
	writeTLSReloadPairWithUsage(t, certFile, keyFile, 1, time.Now().Add(-2*time.Hour), time.Now().Add(-time.Hour), []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth})
	if _, err := newReloadableTLSCertificate(certFile, keyFile, time.Now); err == nil {
		t.Fatal("expired certificate must be rejected")
	}
	writeTLSReloadPairWithUsage(t, certFile, keyFile, 2, time.Now().Add(-time.Minute), time.Now().Add(time.Hour), []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth})
	if _, err := newReloadableTLSCertificate(certFile, keyFile, time.Now); err == nil {
		t.Fatal("client-auth-only certificate must be rejected")
	}
}

func writeTLSReloadPair(t *testing.T, certFile, keyFile string, serial int64, notBefore, notAfter time.Time) {
	t.Helper()
	writeTLSReloadPairWithUsage(t, certFile, keyFile, serial, notBefore, notAfter, []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth})
}

func writeTLSReloadPairWithUsage(t *testing.T, certFile, keyFile string, serial int64, notBefore, notAfter time.Time, usages []x509.ExtKeyUsage) {
	t.Helper()
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{
		SerialNumber: big.NewInt(serial),
		Subject:      pkix.Name{CommonName: "localhost"},
		DNSNames:     []string{"localhost"},
		NotBefore:    notBefore,
		NotAfter:     notAfter,
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  usages,
	}
	certificateDER, err := x509.CreateCertificate(rand.Reader, template, template, publicKey, privateKey)
	if err != nil {
		t.Fatal(err)
	}
	privateDER, err := x509.MarshalPKCS8PrivateKey(privateKey)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(certFile, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certificateDER}), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keyFile, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: privateDER}), 0o600); err != nil {
		t.Fatal(err)
	}
}

func isClosedNetworkError(err error) bool {
	if err == nil {
		return false
	}
	if networkError, ok := err.(net.Error); ok && networkError.Timeout() {
		return false
	}
	return true
}
