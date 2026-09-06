package tcpudp

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/li41/astrahold-server/internal/characteridentity"
	"github.com/li41/astrahold-server/internal/codec/gamev1"
	"github.com/li41/astrahold-server/internal/protocol"
	"github.com/li41/astrahold-server/internal/session"
	"github.com/li41/astrahold-server/internal/transport"
	"github.com/li41/astrahold-server/internal/world"
	"github.com/li41/astrahold-server/internal/worldruntime"
)

func TestTrustedCharacterRestoreFactoryPopulatesJoin(t *testing.T) {
	runtime := newFakeRuntime()
	cfg := DefaultConfig()
	cfg.TCPAddress = "127.0.0.1:0"
	cfg.UDPAddress = "127.0.0.1:0"
	cfg.WorldIdentity = protocol.WorldIdentity{WorldID: "castle-sandbox", Revision: "s3d-001", GameplaySHA256: testGameplaySHA}
	identity, _ := characteridentity.NewTrusted("character:restore-transport")
	cfg.CharacterIdentityFactory = func(session.ID, world.EntityID) (characteridentity.Binding, error) { return identity, nil }
	want := worldruntime.CharacterRestore{
		CharacterID: identity.ID,
		Revision:    5,
		World:       cfg.WorldIdentity,
		HP:          720,
		MaxHP:       1100,
		MP:          100,
		MaxMP:       100,
		Transform: world.Transform{
			Position: world.Position{X: 14, Z: -2, Layer: 3},
			Yaw:      0.75,
		},
	}
	cfg.CharacterRestoreFactory = func(got characteridentity.Binding) (worldruntime.CharacterRestore, bool, error) {
		if got != identity {
			t.Fatalf("identity=%#v", got)
		}
		return want, true, nil
	}

	codec := gamev1.Codec{}
	server := NewServer(cfg, runtime, codec)
	if err := server.Open(); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	serveDone := make(chan error, 1)
	go func() { serveDone <- server.Serve(ctx) }()

	conn, err := net.Dial("tcp", server.TCPAddr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if _, err := transport.ReadEnvelope(conn, codec); err != nil {
		t.Fatal(err)
	}
	select {
	case join := <-runtime.joins:
		if join.Restore == nil || *join.Restore != want {
			t.Fatalf("restore=%#v want=%#v", join.Restore, want)
		}
	case <-time.After(time.Second):
		t.Fatal("join not enqueued")
	}
	cancel()
	_ = server.Close()
	select {
	case err := <-serveDone:
		if err != nil {
			t.Fatalf("serve error: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("serve did not stop")
	}
}

func TestEphemeralIdentitySkipsCharacterRestoreFactory(t *testing.T) {
	runtime := newFakeRuntime()
	cfg := DefaultConfig()
	cfg.TCPAddress = "127.0.0.1:0"
	cfg.UDPAddress = "127.0.0.1:0"
	cfg.WorldIdentity = protocol.WorldIdentity{WorldID: "castle-sandbox", Revision: "s3d-001", GameplaySHA256: testGameplaySHA}
	called := false
	cfg.CharacterRestoreFactory = func(characteridentity.Binding) (worldruntime.CharacterRestore, bool, error) {
		called = true
		return worldruntime.CharacterRestore{}, false, nil
	}

	codec := gamev1.Codec{}
	server := NewServer(cfg, runtime, codec)
	if err := server.Open(); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	serveDone := make(chan error, 1)
	go func() { serveDone <- server.Serve(ctx) }()

	conn, err := net.Dial("tcp", server.TCPAddr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if _, err := transport.ReadEnvelope(conn, codec); err != nil {
		t.Fatal(err)
	}
	select {
	case join := <-runtime.joins:
		if join.Restore != nil {
			t.Fatalf("unexpected restore=%#v", join.Restore)
		}
		if join.Session.CharacterIdentity.Assurance != characteridentity.AssuranceEphemeral {
			t.Fatalf("identity=%#v", join.Session.CharacterIdentity)
		}
		if called {
			t.Fatal("ephemeral path called durable restore factory")
		}
	case <-time.After(time.Second):
		t.Fatal("join not enqueued")
	}
	cancel()
	_ = server.Close()
	select {
	case err := <-serveDone:
		if err != nil {
			t.Fatalf("serve error: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("serve did not stop")
	}
}
