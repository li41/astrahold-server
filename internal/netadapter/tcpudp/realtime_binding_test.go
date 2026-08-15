package tcpudp

import (
	"errors"
	"net"
	"testing"
)

func TestFreshRealtimeSequenceGatesEndpointRebind(t *testing.T) {
	connection := &clientConnection{bindNotify: make(chan struct{}, 1)}
	original := &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 41000}
	rebound := &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 42000}

	if err := connection.bindFreshRealtime(original, 10); err != nil {
		t.Fatal(err)
	}
	if err := connection.bindFreshRealtime(rebound, 10); !errors.Is(err, ErrStaleRealtimeInput) {
		t.Fatalf("replay err=%v", err)
	}
	if got := connection.realtimeAddr(); got == nil || got.Port != original.Port {
		t.Fatalf("replayed packet rebound endpoint: %+v", got)
	}
	if err := connection.bindFreshRealtime(rebound, 9); !errors.Is(err, ErrStaleRealtimeInput) {
		t.Fatalf("older replay err=%v", err)
	}
	if got := connection.realtimeAddr(); got == nil || got.Port != original.Port {
		t.Fatalf("older replay rebound endpoint: %+v", got)
	}

	if err := connection.bindFreshRealtime(rebound, 11); err != nil {
		t.Fatal(err)
	}
	if got := connection.realtimeAddr(); got == nil || got.Port != rebound.Port {
		t.Fatalf("fresh authenticated sequence did not rebind endpoint: %+v", got)
	}
}

func TestRejectedIPRebindDoesNotConsumeSequence(t *testing.T) {
	connection := &clientConnection{bindNotify: make(chan struct{}, 1)}
	original := &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 41000}
	foreign := &net.UDPAddr{IP: net.ParseIP("192.0.2.10"), Port: 43000}
	sameIP := &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 44000}

	if err := connection.bindFreshRealtime(original, 1); err != nil {
		t.Fatal(err)
	}
	if err := connection.bindFreshRealtime(foreign, 2); !errors.Is(err, ErrRealtimeAddressMismatch) {
		t.Fatalf("foreign bind err=%v", err)
	}
	if err := connection.bindFreshRealtime(sameIP, 2); err != nil {
		t.Fatalf("rejected foreign IP consumed sequence: %v", err)
	}
	if got := connection.realtimeAddr(); got == nil || got.Port != sameIP.Port {
		t.Fatalf("same-IP fresh rebind failed: %+v", got)
	}
}
