package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/li41/astrahold-server/internal/codec/gamev1"
	"github.com/li41/astrahold-server/internal/netadapter/browserws"
	"github.com/li41/astrahold-server/internal/netadapter/tcpudp"
	"github.com/li41/astrahold-server/internal/protocol"
	"github.com/li41/astrahold-server/internal/session"
	"github.com/li41/astrahold-server/internal/world"
	"github.com/li41/astrahold-server/internal/worldruntime"
)

const (
	worldNetworkTCPUDP       = "tcpudp"
	worldNetworkBrowserWSDev = "browserws-dev"
)

type worldNetworkConfig struct {
	Mode                 string
	TCPAddress           string
	UDPAddress           string
	BrowserWSAddress     string
	TickRateHz           uint16
	SnapshotRateHz       uint16
	WorldIdentity        protocol.WorldIdentity
	PlayerFactory        tcpudp.PlayerFactory
	CharacterRestore     tcpudp.CharacterRestoreFactory
	TrustedAuthenticator tcpudp.TrustedCharacterConnectionAuthenticator
}

type openedWorldNetwork struct {
	mode    string
	tcp     *tcpudp.Server
	browser *browserWebSocketServer
}

func validateWorldNetworkMode(mode string) error {
	switch mode {
	case worldNetworkTCPUDP, worldNetworkBrowserWSDev:
		return nil
	default:
		return fmt.Errorf("network-mode must be %q or %q", worldNetworkTCPUDP, worldNetworkBrowserWSDev)
	}
}

func openWorldNetwork(config worldNetworkConfig, runtime *worldruntime.Runtime) (*openedWorldNetwork, error) {
	if err := validateWorldNetworkMode(config.Mode); err != nil {
		return nil, err
	}
	if runtime == nil {
		return nil, errors.New("world network runtime is required")
	}

	switch config.Mode {
	case worldNetworkTCPUDP:
		networkConfig := tcpudp.DefaultConfig()
		networkConfig.TCPAddress = config.TCPAddress
		networkConfig.UDPAddress = config.UDPAddress
		networkConfig.TickRateHz = config.TickRateHz
		networkConfig.SnapshotRateHz = config.SnapshotRateHz
		networkConfig.WorldIdentity = config.WorldIdentity
		networkConfig.PlayerFactory = config.PlayerFactory
		networkConfig.CharacterRestoreFactory = config.CharacterRestore
		if config.TrustedAuthenticator != nil {
			networkConfig.TrustedCharacterConnectionAuthenticator = config.TrustedAuthenticator
		}
		server := tcpudp.NewServer(networkConfig, runtime, gamev1.Codec{})
		if err := server.Open(); err != nil {
			return nil, err
		}
		return &openedWorldNetwork{mode: config.Mode, tcp: server}, nil

	case worldNetworkBrowserWSDev:
		if config.TrustedAuthenticator != nil {
			return nil, errors.New("browserws-dev does not accept trusted character authentication")
		}
		if err := validateBrowserWSLoopbackAddress(config.BrowserWSAddress); err != nil {
			return nil, err
		}
		browserConfig := browserws.DefaultConfig()
		browserConfig.TickRateHz = config.TickRateHz
		browserConfig.SnapshotRateHz = config.SnapshotRateHz
		browserConfig.WorldIdentity = config.WorldIdentity
		browserConfig.PlayerFactory = adaptBrowserPlayerFactory(config.PlayerFactory)
		browserConfig.OriginPatterns = []string{
			"http://127.0.0.1:*",
			"http://localhost:*",
		}
		server, err := openBrowserWebSocketServer(config.BrowserWSAddress, browserConfig, runtime)
		if err != nil {
			return nil, err
		}
		return &openedWorldNetwork{mode: config.Mode, browser: server}, nil
	}
	panic("unreachable world network mode")
}

func adaptBrowserPlayerFactory(factory tcpudp.PlayerFactory) browserws.PlayerFactory {
	if factory == nil {
		return nil
	}
	return func(sessionID session.ID, entityID world.EntityID) browserws.PlayerSpec {
		spec := factory(sessionID, entityID)
		return browserws.PlayerSpec{
			Entity:        spec.Entity,
			Speed:         spec.Speed,
			Radius:        spec.Radius,
			MaxStepHeight: spec.MaxStepHeight,
			AOIRadius:     spec.AOIRadius,
		}
	}
}

func validateBrowserWSLoopbackAddress(address string) error {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return fmt.Errorf("invalid browser-ws address %q: %w", address, err)
	}
	if host == "localhost" {
		return nil
	}
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() {
		return fmt.Errorf("browserws-dev must listen on loopback, got %q", address)
	}
	return nil
}

func (n *openedWorldNetwork) Serve(ctx context.Context) error {
	if n.tcp != nil {
		return n.tcp.Serve(ctx)
	}
	if n.browser != nil {
		return n.browser.Serve(ctx)
	}
	return errors.New("world network is not open")
}

func (n *openedWorldNetwork) Close() error {
	if n.tcp != nil {
		return n.tcp.Close()
	}
	if n.browser != nil {
		return n.browser.Close()
	}
	return nil
}

func (n *openedWorldNetwork) Mode() string { return n.mode }

func (n *openedWorldNetwork) TCPAddr() string {
	if n.tcp == nil || n.tcp.TCPAddr() == nil {
		return "disabled"
	}
	return n.tcp.TCPAddr().String()
}

func (n *openedWorldNetwork) UDPAddr() string {
	if n.tcp == nil || n.tcp.UDPAddr() == nil {
		return "disabled"
	}
	return n.tcp.UDPAddr().String()
}

func (n *openedWorldNetwork) BrowserWSAddr() string {
	if n.browser == nil || n.browser.Addr() == nil {
		return "disabled"
	}
	return n.browser.Addr().String()
}

type browserWebSocketServer struct {
	listener net.Listener
	server   *http.Server
}

func openBrowserWebSocketServer(address string, config browserws.Config, runtime browserws.RuntimeSink) (*browserWebSocketServer, error) {
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return nil, err
	}
	return &browserWebSocketServer{
		listener: listener,
		server: &http.Server{
			Handler:           browserws.NewHandler(config, runtime, gamev1.Codec{}),
			ReadHeaderTimeout: 5 * time.Second,
		},
	}, nil
}

func (s *browserWebSocketServer) Addr() net.Addr {
	if s == nil || s.listener == nil {
		return nil
	}
	return s.listener.Addr()
}

func (s *browserWebSocketServer) Serve(ctx context.Context) error {
	shutdownDone := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			_ = s.server.Shutdown(shutdownCtx)
		case <-shutdownDone:
		}
	}()

	err := s.server.Serve(s.listener)
	close(shutdownDone)
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

func (s *browserWebSocketServer) Close() error {
	if s == nil || s.server == nil {
		return nil
	}
	return s.server.Close()
}
