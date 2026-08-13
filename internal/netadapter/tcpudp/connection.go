package tcpudp

import (
	"errors"
	"net"
	"sync"

	"github.com/li41/astrahold-server/internal/protocol"
	"github.com/li41/astrahold-server/internal/session"
	"github.com/li41/astrahold-server/internal/transport"
)

var ErrRealtimeAddressMismatch = errors.New("tcpudp: realtime endpoint IP mismatch")

type clientConnection struct {
	tcp   net.Conn
	udp   *net.UDPConn
	token Token
	codec transport.PayloadCodec

	reliable  chan protocol.Envelope
	realtime  chan protocol.Envelope
	done      chan struct{}
	closeOnce sync.Once

	remoteMu   sync.RWMutex
	remote     *net.UDPAddr
	bindNotify chan struct{}
}

func newClientConnection(tcp net.Conn, udp *net.UDPConn, token Token, codec transport.PayloadCodec, reliableCapacity int) *clientConnection {
	if reliableCapacity <= 0 {
		reliableCapacity = 128
	}
	return &clientConnection{
		tcp:        tcp,
		udp:        udp,
		token:      token,
		codec:      codec,
		reliable:   make(chan protocol.Envelope, reliableCapacity),
		realtime:   make(chan protocol.Envelope, 1),
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
		// Realtime 是 latest-state mailbox。舊 snapshot/correction 可以被較新的狀態取代。
		select {
		case c.realtime <- envelope:
			return nil
		default:
		}
		select {
		case <-c.realtime:
		default:
		}
		select {
		case c.realtime <- envelope:
			return nil
		case <-c.done:
			return session.ErrConnectionClosed
		default:
			return session.ErrBackpressure
		}
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
	var pending *protocol.Envelope
	for {
		if pending == nil {
			select {
			case <-c.done:
				return nil
			case envelope := <-c.realtime:
				pending = &envelope
			}
		}

		addr := c.realtimeAddr()
		if addr == nil {
			select {
			case <-c.done:
				return nil
			case envelope := <-c.realtime:
				pending = &envelope
			case <-c.bindNotify:
			}
			continue
		}

		packet, err := EncodeDatagram(c.token, *pending, c.codec)
		if err != nil {
			if onDrop != nil {
				onDrop(err)
			}
			pending = nil
			continue
		}
		if _, err := c.udp.WriteToUDP(packet, addr); err != nil {
			if onDrop != nil {
				onDrop(err)
			}
			pending = nil
			continue
		}
		pending = nil
	}
}
