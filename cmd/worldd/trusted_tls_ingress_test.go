package main

import (
	"context"
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

func TestTrustedTLSIngressDisabledByDefault(t *testing.T) {
	config, err := loadTrustedTLSIngressConfig("", "", "", "127.0.0.1:7777", false)
	if err != nil {
		t.Fatal(err)
	}
	if config != nil {
		t.Fatalf("config=%+v want nil", config)
	}
}

func TestTrustedTLSIngressRequiresTrustedAuthAndCompleteTLSFiles(t *testing.T) {
	certFile, keyFile, _ := writeTrustedTLSCertificate(t)
	if _, err := loadTrustedTLSIngressConfig("127.0.0.1:8443", certFile, keyFile, "127.0.0.1:7777", false); err == nil {
		t.Fatal("secure ingress without trusted auth must fail")
	}
	if _, err := loadTrustedTLSIngressConfig("127.0.0.1:8443", certFile, "", "127.0.0.1:7777", true); err == nil {
		t.Fatal("partial TLS config must fail")
	}
	if _, err := loadTrustedTLSIngressConfig("127.0.0.1:8443", certFile, keyFile, "0.0.0.0:7777", true); err == nil {
		t.Fatal("secure ingress upstream must remain loopback")
	}
}

func TestTrustedTLSIngressProxiesTLS13ToLoopback(t *testing.T) {
	upstream, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer upstream.Close()

	upstreamDone := make(chan error, 1)
	go func() {
		connection, err := upstream.Accept()
		if err != nil {
			upstreamDone <- err
			return
		}
		defer connection.Close()
		buffer := make([]byte, len("ASTRAHOLD-TLS13"))
		if _, err := io.ReadFull(connection, buffer); err != nil {
			upstreamDone <- err
			return
		}
		_, err = connection.Write(buffer)
		upstreamDone <- err
	}()

	certFile, keyFile, roots := writeTrustedTLSCertificate(t)
	config, err := loadTrustedTLSIngressConfig("127.0.0.1:0", certFile, keyFile, upstream.Addr().String(), true)
	if err != nil {
		t.Fatal(err)
	}
	if config.TLSConfig.MinVersion != tls.VersionTLS13 {
		t.Fatalf("min TLS version=%x want TLS1.3", config.TLSConfig.MinVersion)
	}
	ingress, err := openTrustedTLSIngress(config)
	if err != nil {
		t.Fatal(err)
	}
	defer ingress.Close()

	ctx, cancel := context.WithCancel(context.Background())
	serveDone := make(chan error, 1)
	go func() { serveDone <- ingress.Serve(ctx) }()

	connection, err := tls.Dial("tcp", ingress.Addr().String(), &tls.Config{
		RootCAs:    roots,
		ServerName: "localhost",
		MinVersion: tls.VersionTLS13,
	})
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	payload := []byte("ASTRAHOLD-TLS13")
	if _, err := connection.Write(payload); err != nil {
		connection.Close()
		cancel()
		t.Fatal(err)
	}
	got := make([]byte, len(payload))
	if _, err := io.ReadFull(connection, got); err != nil {
		connection.Close()
		cancel()
		t.Fatal(err)
	}
	if string(got) != string(payload) {
		t.Fatalf("echo=%q want %q", got, payload)
	}
	if connection.ConnectionState().Version != tls.VersionTLS13 {
		t.Fatalf("negotiated TLS=%x want TLS1.3", connection.ConnectionState().Version)
	}
	_ = connection.Close()
	cancel()

	select {
	case err := <-serveDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("secure ingress did not stop after cancellation")
	}
	select {
	case err := <-upstreamDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("upstream echo did not stop")
	}
}

func writeTrustedTLSCertificate(t *testing.T) (string, string, *x509.CertPool) {
	t.Helper()
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "localhost"},
		DNSNames:     []string{"localhost"},
		NotBefore:    time.Now().Add(-time.Minute),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	certificateDER, err := x509.CreateCertificate(rand.Reader, template, template, publicKey, privateKey)
	if err != nil {
		t.Fatal(err)
	}
	privateDER, err := x509.MarshalPKCS8PrivateKey(privateKey)
	if err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()
	certFile := filepath.Join(dir, "server.crt")
	keyFile := filepath.Join(dir, "server.key")
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certificateDER})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: privateDER})
	if err := os.WriteFile(certFile, certPEM, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keyFile, keyPEM, 0o600); err != nil {
		t.Fatal(err)
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(certPEM) {
		t.Fatal("append test root")
	}
	return certFile, keyFile, roots
}
