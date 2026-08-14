package tcpudp

import (
	"testing"

	"github.com/li41/astrahold-server/internal/protocol"
)

func TestReliableBacklogIncludesQueuedAndInFlight(t *testing.T) {
	conn := &clientConnection{reliable: make(chan protocol.Envelope, 2)}
	conn.reliable <- protocol.Envelope{Delivery: protocol.DeliveryReliableOrdered}
	conn.reliableInFlight.Store(true)
	p := &peer{conn: conn}
	p.ready.Store(true)
	server := &Server{peers: map[Token]*peer{{1}: p}}

	backlog := server.ReliableBacklog()
	if backlog.ReadyPeers != 1 || backlog.Queued != 1 || backlog.InFlight != 1 || backlog.MaxQueuedPerPeer != 1 {
		t.Fatalf("backlog=%+v", backlog)
	}
	if backlog.Drained(1) {
		t.Fatal("queued/in-flight work must not be considered drained")
	}

	<-conn.reliable
	conn.reliableInFlight.Store(false)
	backlog = server.ReliableBacklog()
	if !backlog.Drained(1) {
		t.Fatalf("drained backlog=%+v", backlog)
	}
}
