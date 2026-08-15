package tcpudp

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/li41/astrahold-server/internal/characteridentity"
	"github.com/li41/astrahold-server/internal/gateway"
	"github.com/li41/astrahold-server/internal/protocol"
	"github.com/li41/astrahold-server/internal/session"
	"github.com/li41/astrahold-server/internal/transport"
	"github.com/li41/astrahold-server/internal/world"
	"github.com/li41/astrahold-server/internal/worldruntime"
)

var (
	ErrNotOpen                  = errors.New("tcpudp: server is not open")
	ErrAlreadyOpen              = errors.New("tcpudp: server already open")
	ErrInvalidPlayerSpec        = errors.New("tcpudp: invalid player bootstrap spec")
	ErrInvalidWorldIdentity     = errors.New("tcpudp: invalid world identity")
	ErrInvalidCharacterIdentity = errors.New("tcpudp: invalid character identity")
	ErrTCPChannelMismatch       = errors.New("tcpudp: realtime message received on TCP")
	ErrUDPChannelMismatch       = errors.New("tcpudp: reliable message received on UDP")
)

type RuntimeSink interface {
	gateway.MoveCommandSink
	AwaitCharacterAdmission(context.Context, characteridentity.Binding) (worldruntime.CharacterAdmissionLease, error)
	ReleaseCharacterAdmission(context.Context, worldruntime.CharacterAdmissionLease) error
	AwaitJoin(context.Context, worldruntime.JoinRequest) error
	EnqueueLeave(session.ID) error
}

type PlayerSpec struct {
	Entity        world.EntityState
	Speed         float32
	Radius        float32
	MaxStepHeight float32
	AOIRadius     float32
}

type PlayerFactory func(session.ID, world.EntityID) PlayerSpec

// CharacterIdentityFactory is a trusted server integration seam. Implementations may
// return AssuranceTrusted only after an upstream authenticated account/character resolver
// has selected the character. The default development transport issues a fresh ephemeral
// identity per connection and does not authenticate returning characters.
type CharacterIdentityFactory func(session.ID, world.EntityID) (characteridentity.Binding, error)

// CharacterRestoreFactory resolves already-durable state for a trusted identity after the
// world-owner admission lease is acquired and before SessionWelcome is sent. It is never
// called for the default ephemeral identity path. Implementations may perform storage I/O
// because handleTCP is not the world-owner tick.
type CharacterRestoreFactory func(characteridentity.Binding) (worldruntime.CharacterRestore, bool, error)

type Config struct {
	TCPAddress               string
	UDPAddress               string
	TickRateHz               uint16
	SnapshotRateHz           uint16
	ReliableQueueCapacity    int
	PlayerFactory            PlayerFactory
	CharacterIdentityFactory CharacterIdentityFactory
	CharacterRestoreFactory  CharacterRestoreFactory
	WorldIdentity            protocol.WorldIdentity
}

func DefaultConfig() Config {
	return Config{
		TCPAddress:               "127.0.0.1:7777",
		UDPAddress:               "127.0.0.1:7778",
		TickRateHz:               20,
		SnapshotRateHz:           10,
		ReliableQueueCapacity:    128,
		CharacterIdentityFactory: defaultCharacterIdentityFactory,
	}
}

type NetworkError struct {
	SessionID session.ID
	Operation string
	Err       error
}

type peer struct {
	sessionID session.ID
	entityID  world.EntityID
	token     Token
	conn      *clientConnection
	joined    atomic.Bool
	ready     atomic.Bool
	closeOnce sync.Once
	leaveOnce sync.Once
}

type Server struct {
	config  Config
	runtime RuntimeSink
	ingress *gateway.Ingress
	codec   transport.PayloadCodec

	tcp net.Listener
	udp *net.UDPConn

	mu          sync.RWMutex
	peers       map[Token]*peer
	nextSession atomic.Uint64
	nextEntity  atomic.Uint64
	errors      chan NetworkError
	metrics     networkCounters
	closeOnce   sync.Once
}

func NewServer(config Config, runtime RuntimeSink, codec transport.PayloadCodec) *Server {
	if runtime == nil || codec == nil {
		panic("tcpudp: runtime and codec are required")
	}
	if config.TCPAddress == "" {
		config.TCPAddress = "127.0.0.1:7777"
	}
	if config.UDPAddress == "" {
		config.UDPAddress = "127.0.0.1:7778"
	}
	if config.TickRateHz == 0 {
		config.TickRateHz = 20
	}
	if config.SnapshotRateHz == 0 {
		config.SnapshotRateHz = 10
	}
	if config.ReliableQueueCapacity <= 0 {
		config.ReliableQueueCapacity = 128
	}
	if config.PlayerFactory == nil {
		config.PlayerFactory = defaultPlayerFactory
	}
	if config.CharacterIdentityFactory == nil {
		config.CharacterIdentityFactory = defaultCharacterIdentityFactory
	}
	return &Server{
		config:  config,
		runtime: runtime,
		ingress: gateway.NewIngress(runtime),
		codec:   codec,
		peers:   make(map[Token]*peer),
		errors:  make(chan NetworkError, 256),
	}
}

func defaultPlayerFactory(_ session.ID, entityID world.EntityID) PlayerSpec {
	index := float32((uint64(entityID) - 1) % 8)
	return PlayerSpec{
		Entity: world.EntityState{
			ID:        entityID,
			Kind:      world.EntityPlayer,
			Transform: world.Transform{Position: world.Position{X: index * 1.5, Layer: 0}},
		},
		Speed:         6,
		Radius:        0.35,
		MaxStepHeight: 0.5,
		AOIRadius:     64,
	}
}

func defaultCharacterIdentityFactory(_ session.ID, _ world.EntityID) (characteridentity.Binding, error) {
	return characteridentity.NewEphemeral()
}

func (s *Server) Open() error {
	if s.tcp != nil || s.udp != nil {
		return ErrAlreadyOpen
	}
	if !s.config.WorldIdentity.Valid() {
		return ErrInvalidWorldIdentity
	}
	tcp, err := net.Listen("tcp", s.config.TCPAddress)
	if err != nil {
		return err
	}
	udpAddr, err := net.ResolveUDPAddr("udp", s.config.UDPAddress)
	if err != nil {
		_ = tcp.Close()
		return err
	}
	udp, err := net.ListenUDP("udp", udpAddr)
	if err != nil {
		_ = tcp.Close()
		return err
	}
	s.tcp = tcp
	s.udp = udp
	return nil
}

func (s *Server) TCPAddr() net.Addr {
	if s.tcp == nil {
		return nil
	}
	return s.tcp.Addr()
}

func (s *Server) UDPAddr() *net.UDPAddr {
	if s.udp == nil {
		return nil
	}
	if addr, ok := s.udp.LocalAddr().(*net.UDPAddr); ok {
		cp := *addr
		cp.IP = append(net.IP(nil), addr.IP...)
		return &cp
	}
	return nil
}

func (s *Server) Errors() <-chan NetworkError { return s.errors }

func (s *Server) Serve(ctx context.Context) error {
	if s.tcp == nil || s.udp == nil {
		return ErrNotOpen
	}
	go s.udpLoop(ctx)
	go func() { <-ctx.Done(); _ = s.Close() }()

	for {
		conn, err := s.tcp.Accept()
		if err != nil {
			select {
			case <-ctx.Done():
				return nil
			default:
				return err
			}
		}
		go s.handleTCP(ctx, conn)
	}
}

func (s *Server) Close() error {
	s.closeOnce.Do(func() {
		if s.tcp != nil {
			_ = s.tcp.Close()
		}
		if s.udp != nil {
			_ = s.udp.Close()
		}
		s.mu.RLock()
		peers := make([]*peer, 0, len(s.peers))
		for _, p := range s.peers {
			peers = append(peers, p)
		}
		s.mu.RUnlock()
		for _, p := range peers {
			s.closePeer(p, "server_close", nil)
		}
	})
	return nil
}

func (s *Server) handleTCP(ctx context.Context, raw net.Conn) {
	if tcp, ok := raw.(*net.TCPConn); ok {
		_ = tcp.SetNoDelay(true)
		_ = tcp.SetKeepAlive(true)
		_ = tcp.SetKeepAlivePeriod(30 * time.Second)
	}

	token, err := NewToken()
	if err != nil {
		_ = raw.Close()
		return
	}
	sid := session.ID(s.nextSession.Add(1))
	eid := world.EntityID(s.nextEntity.Add(1))
	spec := s.config.PlayerFactory(sid, eid)
	if spec.Entity.ID != eid || spec.AOIRadius <= 0 || spec.Speed <= 0 || spec.Radius <= 0 {
		s.emit(sid, "player_factory", ErrInvalidPlayerSpec)
		_ = raw.Close()
		return
	}
	identity, err := s.config.CharacterIdentityFactory(sid, eid)
	if err != nil {
		s.emit(sid, "character_identity_factory", err)
		_ = raw.Close()
		return
	}
	if !identity.Valid() {
		s.emit(sid, "character_identity_factory", ErrInvalidCharacterIdentity)
		_ = raw.Close()
		return
	}

	var admissionLease *worldruntime.CharacterAdmissionLease
	if identity.Assurance == characteridentity.AssuranceTrusted {
		lease, err := s.runtime.AwaitCharacterAdmission(ctx, identity)
		if err != nil {
			s.emit(sid, "character_admission", err)
			_ = raw.Close()
			return
		}
		admissionLease = &lease
		defer func() {
			if admissionLease == nil {
				return
			}
			if err := s.runtime.ReleaseCharacterAdmission(context.Background(), *admissionLease); err != nil {
				s.emit(sid, "character_admission_release", err)
			}
		}()
	}

	var restore *worldruntime.CharacterRestore
	if identity.Assurance == characteridentity.AssuranceTrusted && s.config.CharacterRestoreFactory != nil {
		candidate, exists, err := s.config.CharacterRestoreFactory(identity)
		if err != nil {
			s.emit(sid, "character_restore_factory", err)
			_ = raw.Close()
			return
		}
		if exists {
			if err := worldruntime.ValidateCharacterRestore(identity, candidate, s.config.WorldIdentity); err != nil {
				s.emit(sid, "character_restore_factory", err)
				_ = raw.Close()
				return
			}
			candidateCopy := candidate
			restore = &candidateCopy
		}
	}

	connection := newClientConnection(raw, s.udp, token, s.codec, s.config.ReliableQueueCapacity, &s.metrics)
	sess, err := session.NewWithCharacterIdentity(sid, eid, identity, spec.AOIRadius, connection)
	if err != nil {
		_ = raw.Close()
		return
	}
	p := &peer{sessionID: sid, entityID: eid, token: token, conn: connection}
	s.mu.Lock()
	s.peers[token] = p
	s.mu.Unlock()

	if err := s.runtime.AwaitJoin(ctx, worldruntime.JoinRequest{
		Session:        sess,
		Entity:         spec.Entity,
		Speed:          spec.Speed,
		Radius:         spec.Radius,
		MaxStepHeight:  spec.MaxStepHeight,
		Restore:        restore,
		AdmissionLease: admissionLease,
	}); err != nil {
		s.closePeer(p, "join_world", err)
		return
	}
	// The world-owner join consumed the matching reservation atomically with active ownership.
	admissionLease = nil
	p.joined.Store(true)

	udpPort := uint16(0)
	if addr := s.UDPAddr(); addr != nil && addr.Port >= 0 && addr.Port <= 65535 {
		udpPort = uint16(addr.Port)
	}
	welcome := protocol.Envelope{
		Delivery:   protocol.DeliveryReliableOrdered,
		Sequence:   0,
		ServerTick: 0,
		Message: protocol.SessionWelcome{
			SessionID:      uint64(sid),
			EntityID:       eid,
			RealtimePort:   udpPort,
			RealtimeToken:  token.String(),
			TickRateHz:     s.config.TickRateHz,
			SnapshotRateHz: s.config.SnapshotRateHz,
			World:          s.config.WorldIdentity,
		},
	}
	if err := transport.WriteEnvelope(raw, welcome, s.codec); err != nil {
		s.closePeer(p, "welcome_write", err)
		return
	}
	p.ready.Store(true)

	go func() {
		if err := connection.runReliableWriter(); err != nil {
			s.closePeer(p, "tcp_write", err)
		}
	}()
	go func() {
		_ = connection.runRealtimeWriter(func(err error) { s.emit(sid, "udp_write", err) })
	}()

	for {
		envelope, err := transport.ReadEnvelope(raw, s.codec)
		if err != nil {
			s.closePeer(p, "tcp_read", err)
			return
		}
		if envelope.Delivery != protocol.DeliveryReliableOrdered {
			s.closePeer(p, "tcp_channel", ErrTCPChannelMismatch)
			return
		}
		if err := s.ingress.Handle(sid, envelope); err != nil {
			s.closePeer(p, "tcp_ingress", err)
			return
		}
		select {
		case <-ctx.Done():
			s.closePeer(p, "context_done", nil)
			return
		default:
		}
	}
}

func (s *Server) udpLoop(ctx context.Context) {
	buffer := make([]byte, 65535)
	for {
		n, addr, err := s.udp.ReadFromUDP(buffer)
		if err != nil {
			select {
			case <-ctx.Done():
				return
			default:
				s.emit(0, "udp_read", err)
				return
			}
		}
		token, envelope, err := DecodeDatagram(buffer[:n], s.codec)
		if err != nil {
			s.emit(0, "udp_decode", err)
			continue
		}
		s.mu.RLock()
		p := s.peers[token]
		s.mu.RUnlock()
		if p == nil || !p.ready.Load() {
			continue
		}
		if envelope.Delivery != protocol.DeliveryRealtimeSequenced {
			s.emit(p.sessionID, "udp_channel", ErrUDPChannelMismatch)
			continue
		}
		if err := p.conn.bindRealtime(addr); err != nil {
			s.emit(p.sessionID, "udp_bind", err)
			continue
		}
		if err := s.ingress.Handle(p.sessionID, envelope); err != nil {
			s.emit(p.sessionID, "udp_ingress", err)
			continue
		}
	}
}

func (s *Server) closePeer(p *peer, operation string, cause error) {
	if p == nil {
		return
	}
	p.closeOnce.Do(func() {
		p.ready.Store(false)
		s.mu.Lock()
		if current := s.peers[p.token]; current == p {
			delete(s.peers, p.token)
		}
		s.mu.Unlock()
		_ = p.conn.Close()
		if cause != nil && !errors.Is(cause, net.ErrClosed) && !errors.Is(cause, io.EOF) {
			s.emit(p.sessionID, operation, cause)
		}
	})
	if p.joined.Load() {
		p.leaveOnce.Do(func() {
			if err := s.runtime.EnqueueLeave(p.sessionID); err != nil {
				s.emit(p.sessionID, "enqueue_leave", err)
			}
		})
	}
}

func (s *Server) emit(id session.ID, operation string, err error) {
	if err == nil {
		return
	}
	event := NetworkError{SessionID: id, Operation: operation, Err: err}
	select {
	case s.errors <- event:
	default:
	}
}

func HostPort(addr net.Addr) (string, uint16, error) {
	if addr == nil {
		return "", 0, ErrNotOpen
	}
	host, portString, err := net.SplitHostPort(addr.String())
	if err != nil {
		return "", 0, err
	}
	port, err := strconv.Atoi(portString)
	if err != nil || port < 0 || port > 65535 {
		return "", 0, fmt.Errorf("tcpudp: invalid port %q", portString)
	}
	return host, uint16(port), nil
}
