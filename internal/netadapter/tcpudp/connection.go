package tcpudp

import (
	"errors"
	"net"
	"sync"
	"sync/atomic"

	"github.com/li41/astrahold-server/internal/protocol"
	"github.com/li41/astrahold-server/internal/session"
	"github.com/li41/astrahold-server/internal/transport"
)

var (
	ErrRealtimeAddressMismatch = errors.New("tcpudp: realtime endpoint IP mismatch")
	ErrStaleRealtimeInput      = errors.New("tcpudp: stale realtime input sequence")
)

type clientConnection struct {
	tcp     net.Conn
	udp     *net.UDPConn
	token   Token
	codec   transport.PayloadCodec
	metrics *networkCounters

	reliable         chan protocol.Envelope
	reliableInFlight atomic.Bool
	realtime         *realtimeMailbox
	done             chan struct{}
	closeOnce        sync.Once

	remoteMu                  sync.RWMutex
	remote                    *net.UDPAddr
	lastRealtimeInputSequence uint32
	bindNotify                chan struct{}
}

var _ session.ImmediateRealtimeConnection = (*clientConnection)(nil)

func newClientConnection(tcp net.Conn, udp *net.UDPConn, token Token, codec transport.PayloadCodec, reliableCapacity int, metrics *networkCounters) *clientConnection {
	if reliableCapacity <= 0 {
		reliableCapacity = 128
	}
	return &clientConnection{
		tcp:        tcp,
		udp:        udp,
		token:      token,
		codec:      codec,
		metrics:    metrics,
		reliable:   make(chan protocol.Envelope, reliableCapacity),
		realtime:   newRealtimeMailbox(),
		done:       make(chan struct{}),
		bindNotify: make(chan struct{}, 1),
	}
}

func (c *clientConnection) TrySend(envelope protocol.Envelope) error {
	select {
	case <-c.done:
		return session.ErrConnectionClosed
	default:
	}

	switch envelope.Delivery {
	case protocol.DeliveryReliableOrdered:
		select {
		case c.reliable <- envelope:
			return nil
		case <-c.done:
			return session.ErrConnectionClosed
		default:
			return session.ErrBackpressure
		}
	case protocol.DeliveryRealtimeSequenced:
		// PutEncoded 在返回前即把 realtime message materialize 到 connection-owned packet slot。
		// 因此 caller 可安全重用 snapshot backing storage；writer 之後只讀 mailbox-owned bytes。
		return c.realtime.PutEncoded(c.token, envelope, c.codec)
	default:
		return session.ErrInvalidSession
	}
}

// RealtimeConsumedBeforeReturn 宣告 TrySend(Realtime) 的同步 ownership capability。
func (*clientConnection) RealtimeConsumedBeforeReturn() {}

func (c *clientConnection) Close() error {
	c.closeOnce.Do(func() {
		close(c.done)
		_ = c.tcp.Close()
	})
	return nil
}

// bindFreshRealtime atomically applies the C2S anti-replay gate and same-IP NAT port rebind.
// A stale/replayed authenticated datagram therefore cannot redirect S2C realtime traffic before
// the world owner later rejects the same stale input sequence.
func (c *clientConnection) bindFreshRealtime(addr *net.UDPAddr, sequence uint32) error {
	if addr == nil {
		return ErrRealtimeAddressMismatch
	}
	c.remoteMu.Lock()
	defer c.remoteMu.Unlock()
	if c.remote != nil && !c.remote.IP.Equal(addr.IP) {
		return ErrRealtimeAddressMismatch
	}
	if sequence == 0 || sequence <= c.lastRealtimeInputSequence {
		return ErrStaleRealtimeInput
	}
	copyAddr := *addr
	copyAddr.IP = append(net.IP(nil), addr.IP...)
	c.remote = &copyAddr
	c.lastRealtimeInputSequence = sequence
	select {
	case c.bindNotify <- struct{}{}:
	default:
	}
	return nil
}

// bindRealtime remains a narrow transport helper for tests that do not model inbound sequence.
// Production udpLoop uses bindFreshRealtime so endpoint mutation is replay-gated.
func (c *clientConnection) bindRealtime(addr *net.UDPAddr) error {
	if addr == nil {
		return ErrRealtimeAddressMismatch
	}
	c.remoteMu.Lock()
	defer c.remoteMu.Unlock()
	if c.remote != nil && !c.remote.IP.Equal(addr.IP) {
		return ErrRealtimeAddressMismatch
	}
	copyAddr := *addr
	copyAddr.IP = append(net.IP(nil), addr.IP...)
	c.remote = &copyAddr
	select {
	case c.bindNotify <- struct{}{}:
	default:
	}
	return nil
}

func (c *clientConnection) realtimeAddr() *net.UDPAddr {
	c.remoteMu.RLock()
	defer c.remoteMu.RUnlock()
	// bindRealtime / bindFreshRealtime 永遠建立新的 immutable address object，不會原地修改既有 c.remote。
	// 因此 writer 可在 lock 外安全持有這個 pointer，且不需要每 datagram 複製 IP slice。
	return c.remote
}

func (c *clientConnection) runReliableWriter() error {
	for {
		select {
		case <-c.done:
			return nil
		case envelope := <-c.reliable:
			c.reliableInFlight.Store(true)
			err := transport.WriteEnvelope(c.tcp, envelope, c.codec)
			c.reliableInFlight.Store(false)
			if err != nil {
				return err
			}
		}
	}
}

func (c *clientConnection) runRealtimeWriter(onDrop func(error)) error {
	// NextPacket 只在 lock 內把 mailbox-owned packet 複製進此 writer-owned buffer；
	// WriteToUDP 返回後即可安全覆寫。
	packetBuffer := make([]byte, 0, MaxDatagramSize)
	for {
		addr := c.realtimeAddr()
		if addr == nil {
			// 尚未收到第一個 UDP input 時不從 mailbox 取資料；
			// producer 可以持續 coalesce，綁定完成後只送最新 correction / snapshot set。
			select {
			case <-c.done:
				return nil
			case <-c.bindNotify:
				continue
			}
		}

		packet, messageType, encodeDuration, ok := c.realtime.NextPacket(packetBuffer[:0], c.done)
		if !ok {
			return nil
		}
		packetBuffer = packet[:0]
		c.metrics.recordRealtime(messageType, len(packet), encodeDuration)
		if _, err := c.udp.WriteToUDP(packet, addr); err != nil {
			if onDrop != nil {
				onDrop(err)
			}
			continue
		}
	}
}
