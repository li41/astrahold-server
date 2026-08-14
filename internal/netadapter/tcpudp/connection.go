package tcpudp

import (
	"errors"
	"net"
	"sync"
	"time"

	"github.com/li41/astrahold-server/internal/protocol"
	"github.com/li41/astrahold-server/internal/session"
	"github.com/li41/astrahold-server/internal/transport"
)

var ErrRealtimeAddressMismatch = errors.New("tcpudp: realtime endpoint IP mismatch")

type clientConnection struct {
	tcp     net.Conn
	udp     *net.UDPConn
	token   Token
	codec   transport.PayloadCodec
	metrics *networkCounters

	reliable chan protocol.Envelope
	realtime *realtimeMailbox
	done     chan struct{}
	closeOnce sync.Once

	remoteMu   sync.RWMutex
	remote     *net.UDPAddr
	bindNotify chan struct{}
}

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
		return c.realtime.Put(envelope)
	default:
		return session.ErrInvalidSession
	}
}

func (c *clientConnection) Close() error {
	c.closeOnce.Do(func() {
		close(c.done)
		_ = c.tcp.Close()
	})
	return nil
}

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
	if c.remote == nil {
		return nil
	}
	copyAddr := *c.remote
	copyAddr.IP = append(net.IP(nil), c.remote.IP...)
	return &copyAddr
}

func (c *clientConnection) runReliableWriter() error {
	for {
		select {
		case <-c.done:
			return nil
		case envelope := <-c.reliable:
			if err := transport.WriteEnvelope(c.tcp, envelope, c.codec); err != nil {
				return err
			}
		}
	}
}

func (c *clientConnection) runRealtimeWriter(onDrop func(error)) error {
	// WriteToUDP 在回傳後不再持有 packet bytes，因此每個 writer 可以安全重用自己的 MTU-sized buffer。
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

		envelope, ok := c.realtime.Next(c.done)
		if !ok {
			return nil
		}
		started := time.Now()
		packet, err := AppendEncodeDatagram(packetBuffer[:0], c.token, envelope, c.codec)
		encodeDuration := time.Since(started)
		if err != nil {
			if onDrop != nil {
				onDrop(err)
			}
			continue
		}
		packetBuffer = packet[:0]
		c.metrics.recordRealtime(envelope.Message.Type(), len(packet), encodeDuration)
		if _, err := c.udp.WriteToUDP(packet, addr); err != nil {
			if onDrop != nil {
				onDrop(err)
			}
			continue
		}
	}
}
