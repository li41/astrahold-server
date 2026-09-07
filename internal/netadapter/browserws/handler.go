// Package browserws adapts Browser WebSocket messages to the existing Astrahold ASTR transport frames.
// It is an ephemeral-identity development/E2E adapter; gameplay authority remains in worldruntime.
package browserws

import (
	"context"
	"errors"
	"net/http"
	"sync/atomic"

	"github.com/coder/websocket"
	"github.com/li41/astrahold-server/internal/gateway"
	"github.com/li41/astrahold-server/internal/protocol"
	"github.com/li41/astrahold-server/internal/session"
	"github.com/li41/astrahold-server/internal/transport"
	"github.com/li41/astrahold-server/internal/world"
	"github.com/li41/astrahold-server/internal/worldruntime"
)

var (
	ErrInvalidPlayerSpec    = errors.New("browserws: invalid player bootstrap spec")
	ErrInvalidWorldIdentity = errors.New("browserws: invalid world identity")
)

type RuntimeSink interface {
	gateway.MoveCommandSink
	gateway.ActionCommandSink
	AwaitJoinOwned(context.Context, worldruntime.JoinRequest) (worldruntime.SessionOwnershipFence, error)
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

type Config struct {
	TickRateHz            uint16
	SnapshotRateHz        uint16
	ReliableQueueCapacity int
	RealtimeQueueCapacity int
	PlayerFactory         PlayerFactory
	WorldIdentity         protocol.WorldIdentity
	OriginPatterns        []string
}

func DefaultConfig() Config {
	return Config{
		TickRateHz:            20,
		SnapshotRateHz:        10,
		ReliableQueueCapacity: 128,
		RealtimeQueueCapacity: 128,
		PlayerFactory:         defaultPlayerFactory,
	}
}

// Handler is deliberately transport-only. Do not mount it beside another adapter that
// independently allocates Session/Entity IDs against the same runtime until allocation is shared.
type Handler struct {
	config      Config
	runtime     RuntimeSink
	codec       transport.PayloadCodec
	ingress     *gateway.Ingress
	nextSession atomic.Uint64
	nextEntity  atomic.Uint64
}

func NewHandler(config Config, runtime RuntimeSink, codec transport.PayloadCodec) *Handler {
	if runtime == nil || codec == nil {
		panic("browserws: runtime and codec are required")
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
	if config.RealtimeQueueCapacity <= 0 {
		config.RealtimeQueueCapacity = 128
	}
	if config.PlayerFactory == nil {
		config.PlayerFactory = defaultPlayerFactory
	}
	return &Handler{config: config, runtime: runtime, codec: codec, ingress: gateway.NewIngress(runtime)}
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

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !h.config.WorldIdentity.Valid() {
		http.Error(w, ErrInvalidWorldIdentity.Error(), http.StatusServiceUnavailable)
		return
	}

	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		OriginPatterns: h.config.OriginPatterns,
	})
	if err != nil {
		return
	}
	defer conn.Close(websocket.StatusNormalClosure, "")
	conn.SetReadLimit(int64(transport.HeaderSize) + int64(transport.MaxPayloadSize))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sid := session.ID(h.nextSession.Add(1))
	entityID := world.EntityID(h.nextEntity.Add(1))
	spec := h.config.PlayerFactory(sid, entityID)
	if spec.Entity.ID != entityID || spec.AOIRadius <= 0 || spec.Speed <= 0 || spec.Radius <= 0 {
		_ = conn.Close(websocket.StatusInternalError, ErrInvalidPlayerSpec.Error())
		return
	}

	outbound := session.NewQueueConnection(h.config.ReliableQueueCapacity, h.config.RealtimeQueueCapacity)
	defer outbound.Close()
	sess, err := session.New(sid, entityID, spec.AOIRadius, outbound)
	if err != nil {
		_ = conn.Close(websocket.StatusInternalError, "session bootstrap failed")
		return
	}

	if _, err := h.runtime.AwaitJoinOwned(ctx, worldruntime.JoinRequest{
		Session:       sess,
		Entity:        spec.Entity,
		Speed:         spec.Speed,
		Radius:        spec.Radius,
		MaxStepHeight: spec.MaxStepHeight,
	}); err != nil {
		_ = conn.Close(websocket.StatusInternalError, "world join failed")
		return
	}
	joined := true
	defer func() {
		if joined {
			_ = h.runtime.EnqueueLeave(sid)
		}
	}()

	welcome := protocol.Envelope{
		Delivery: protocol.DeliveryReliableOrdered,
		Message: protocol.SessionWelcome{
			SessionID:      uint64(sid),
			EntityID:       entityID,
			RealtimePort:   0,
			RealtimeToken:  "",
			TickRateHz:     h.config.TickRateHz,
			SnapshotRateHz: h.config.SnapshotRateHz,
			World:          h.config.WorldIdentity,
		},
	}
	if err := writeEnvelope(ctx, conn, welcome, h.codec); err != nil {
		return
	}

	writerErr := make(chan error, 1)
	go func() {
		writerErr <- runWriter(ctx, conn, outbound, h.codec)
		cancel()
	}()

	for {
		messageType, data, err := conn.Read(ctx)
		if err != nil {
			return
		}
		if messageType != websocket.MessageBinary {
			_ = conn.Close(websocket.StatusUnsupportedData, "binary ASTR frames required")
			return
		}
		envelope, err := transport.DecodeEnvelope(data, h.codec)
		if err != nil {
			_ = conn.Close(websocket.StatusProtocolError, "invalid ASTR frame")
			return
		}
		if err := h.ingress.Handle(sid, envelope); err != nil {
			_ = conn.Close(websocket.StatusPolicyViolation, "invalid client intent")
			return
		}
		select {
		case <-writerErr:
			return
		default:
		}
	}
}

func runWriter(ctx context.Context, conn *websocket.Conn, outbound *session.QueueConnection, codec transport.PayloadCodec) error {
	for {
		var envelope protocol.Envelope
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-outbound.Done():
			return session.ErrConnectionClosed
		case envelope = <-outbound.Reliable():
		case envelope = <-outbound.Realtime():
		}
		if err := writeEnvelope(ctx, conn, envelope, codec); err != nil {
			return err
		}
	}
}

func writeEnvelope(ctx context.Context, conn *websocket.Conn, envelope protocol.Envelope, codec transport.PayloadCodec) error {
	data, err := transport.EncodeEnvelope(envelope, codec)
	if err != nil {
		return err
	}
	return conn.Write(ctx, websocket.MessageBinary, data)
}
