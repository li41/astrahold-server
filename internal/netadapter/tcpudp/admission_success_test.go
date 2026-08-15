package tcpudp

import (
	"net"
	"testing"
	"time"

	"github.com/li41/astrahold-server/internal/characteridentity"
	"github.com/li41/astrahold-server/internal/codec/gamev1"
	"github.com/li41/astrahold-server/internal/protocol"
	"github.com/li41/astrahold-server/internal/session"
	"github.com/li41/astrahold-server/internal/transport"
	"github.com/li41/astrahold-server/internal/world"
)

func TestTrustedSuccessfulJoinCarriesAdmissionLease(t *testing.T) {
	runtime := newFakeRuntime()
	cfg := DefaultConfig()
	cfg.TCPAddress = "127.0.0.1:0"
	cfg.UDPAddress = "127.0.0.1:0"
	cfg.WorldIdentity = protocol.WorldIdentity{WorldID: "castle-sandbox", Revision: "s3d-001", GameplaySHA256: testGameplaySHA}
	identity, err := characteridentity.NewTrusted("character:lease-success")
	if err != nil {
		t.Fatal(err)
	}
	cfg.CharacterIdentityFactory = func(session.ID, world.EntityID) (characteridentity.Binding, error) {
		return identity, nil
	}

	codec := gamev1.Codec{}
	server, cancel, serveDone := startTCPUDPTestServer(t, cfg, runtime, codec)
	defer stopTCPUDPTestServer(t, server, cancel, serveDone)
	conn, err := net.Dial("tcp", server.TCPAddr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if err := conn.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if _, err := transport.ReadEnvelope(conn, codec); err != nil {
		t.Fatal(err)
	}

	select {
	case join := <-runtime.joins:
		if join.AdmissionLease == nil || !join.AdmissionLease.Valid() {
			t.Fatalf("join admission lease=%#v", join.AdmissionLease)
		}
		if join.AdmissionLease.CharacterID != identity.ID {
			t.Fatalf("lease character=%s want=%s", join.AdmissionLease.CharacterID, identity.ID)
		}
	case <-time.After(time.Second):
		t.Fatal("trusted join not observed")
	}
}
