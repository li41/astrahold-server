package main

import (
	"context"
	"crypto/tls"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"
)

const trustedTLSHandshakeTimeout = 5 * time.Second

var (
	trustedTLSListen = flag.String(
		"trusted-tls-listen",
		"",
		"Optional TLS 1.3 ingress listen address for trusted TCP clients; forwards only to the loopback -tcp listener",
	)
	trustedTLSCertFile = flag.String(
		"trusted-tls-cert",
		"",
		"PEM certificate chain for -trusted-tls-listen; reloaded with its key on SIGHUP",
	)
	trustedTLSKeyFile = flag.String(
		"trusted-tls-key",
		"",
		"PEM private key for -trusted-tls-listen; reloaded with its certificate on SIGHUP",
	)
)

var (
	errTrustedTLSIngressConfig       = errors.New("worldd: invalid trusted TLS ingress config")
	errTrustedTLSIngressRequiresAuth = errors.New("worldd: trusted TLS ingress requires trusted character authentication")
	errTrustedTLSIngressUnavailable  = errors.New("worldd: trusted TLS ingress is not open")
)

type trustedTLSIngressConfig struct {
	ListenAddress   string
	UpstreamAddress string
	TLSConfig       *tls.Config
	Certificate     *reloadableTLSCertificate
}

type trustedTLSIngress struct {
	listener    net.Listener
	upstream    string
	certificate *reloadableTLSCertificate
	closeOnce   sync.Once
}

func loadTrustedTLSIngressConfig(listenAddress, certFile, keyFile, upstreamAddress string, trustedAuthEnabled bool) (*trustedTLSIngressConfig, error) {
	listenAddress = strings.TrimSpace(listenAddress)
	certFile = strings.TrimSpace(certFile)
	keyFile = strings.TrimSpace(keyFile)
	if listenAddress == "" && certFile == "" && keyFile == "" {
		return nil, nil
	}
	if !trustedAuthEnabled {
		return nil, errTrustedTLSIngressRequiresAuth
	}
	if listenAddress == "" || certFile == "" || keyFile == "" {
		return nil, fmt.Errorf("%w: trusted-tls-listen, trusted-tls-cert, and trusted-tls-key must be set together", errTrustedTLSIngressConfig)
	}
	if _, _, err := net.SplitHostPort(listenAddress); err != nil {
		return nil, fmt.Errorf("%w: listen address: %v", errTrustedTLSIngressConfig, err)
	}
	if err := validateTrustedCharacterAuthListenAddress(upstreamAddress); err != nil {
		return nil, fmt.Errorf("%w: upstream must remain loopback: %v", errTrustedTLSIngressConfig, err)
	}
	certificate, err := newReloadableTLSCertificate(certFile, keyFile, time.Now)
	if err != nil {
		return nil, fmt.Errorf("%w: load certificate: %v", errTrustedTLSIngressConfig, err)
	}
	return &trustedTLSIngressConfig{
		ListenAddress:   listenAddress,
		UpstreamAddress: upstreamAddress,
		TLSConfig:       certificate.TLSConfig(),
		Certificate:     certificate,
	}, nil
}

func openTrustedTLSIngress(config *trustedTLSIngressConfig) (*trustedTLSIngress, error) {
	if config == nil || config.TLSConfig == nil || config.Certificate == nil || strings.TrimSpace(config.ListenAddress) == "" || strings.TrimSpace(config.UpstreamAddress) == "" {
		return nil, errTrustedTLSIngressConfig
	}
	base, err := net.Listen("tcp", config.ListenAddress)
	if err != nil {
		return nil, err
	}
	return &trustedTLSIngress{
		listener:    tls.NewListener(base, config.TLSConfig.Clone()),
		upstream:    config.UpstreamAddress,
		certificate: config.Certificate,
	}, nil
}

func (i *trustedTLSIngress) Addr() net.Addr {
	if i == nil || i.listener == nil {
		return nil
	}
	return i.listener.Addr()
}

func (i *trustedTLSIngress) TLSCertificateSnapshot() tlsCertificateSnapshot {
	if i == nil || i.certificate == nil {
		return tlsCertificateSnapshot{}
	}
	return i.certificate.Snapshot()
}

func (i *trustedTLSIngress) reloadTLSCertificate() (tlsCertificateReloadResult, error) {
	if i == nil || i.certificate == nil {
		return tlsCertificateReloadResult{}, errTrustedTLSIngressConfig
	}
	return i.certificate.Reload()
}

func (i *trustedTLSIngress) Close() error {
	if i == nil {
		return nil
	}
	var err error
	i.closeOnce.Do(func() {
		if i.listener != nil {
			err = i.listener.Close()
		}
	})
	return err
}

func (i *trustedTLSIngress) Serve(ctx context.Context) error {
	if i == nil || i.listener == nil {
		return errTrustedTLSIngressUnavailable
	}
	if ctx == nil {
		ctx = context.Background()
	}
	reloadSignals := make(chan os.Signal, 1)
	signal.Notify(reloadSignals, syscall.SIGHUP)
	defer signal.Stop(reloadSignals)
	go runTrustedTLSCertificateRuntime(ctx, reloadSignals, i, log.Printf)
	go func() {
		<-ctx.Done()
		_ = i.Close()
	}()
	for {
		connection, err := i.listener.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return err
		}
		go i.proxy(ctx, connection)
	}
}

func runTrustedTLSCertificateRuntime(
	ctx context.Context,
	reloadSignals <-chan os.Signal,
	ingress *trustedTLSIngress,
	logf func(string, ...any),
) {
	if ctx == nil || ingress == nil || ingress.certificate == nil {
		return
	}
	if logf == nil {
		logf = func(string, ...any) {}
	}
	initial := ingress.TLSCertificateSnapshot()
	logf("trusted TLS certificate runtime: reload=sighup generation=%d not_after=%s", initial.Generation, initial.NotAfter.UTC().Format(time.RFC3339Nano))
	for {
		select {
		case <-ctx.Done():
			return
		case <-reloadSignals:
			result, err := ingress.reloadTLSCertificate()
			if err != nil {
				current := ingress.TLSCertificateSnapshot()
				logf("trusted TLS certificate reload rejected; last-known-good retained: generation=%d err=%v", current.Generation, err)
				continue
			}
			logf("trusted TLS certificate reload applied: previous_generation=%d generation=%d not_after=%s", result.PreviousGeneration, result.Generation, result.NotAfter.UTC().Format(time.RFC3339Nano))
		}
	}
}

func (i *trustedTLSIngress) proxy(ctx context.Context, downstream net.Conn) {
	if downstream == nil {
		return
	}
	defer downstream.Close()

	if tlsConnection, ok := downstream.(*tls.Conn); ok {
		handshakeCtx, cancel := context.WithTimeout(ctx, trustedTLSHandshakeTimeout)
		err := tlsConnection.HandshakeContext(handshakeCtx)
		cancel()
		if err != nil {
			return
		}
	}

	dialer := net.Dialer{Timeout: trustedTLSHandshakeTimeout}
	upstream, err := dialer.DialContext(ctx, "tcp", i.upstream)
	if err != nil {
		return
	}
	defer upstream.Close()

	done := make(chan struct{}, 2)
	go func() {
		_, _ = io.Copy(upstream, downstream)
		done <- struct{}{}
	}()
	go func() {
		_, _ = io.Copy(downstream, upstream)
		done <- struct{}{}
	}()

	select {
	case <-ctx.Done():
	case <-done:
	}
}
