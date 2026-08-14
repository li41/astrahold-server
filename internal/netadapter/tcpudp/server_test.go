package tcpudp

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/li41/astrahold-server/internal/codec/gamev1"
	"github.com/li41/astrahold-server/internal/protocol"
	"github.com/li41/astrahold-server/internal/session"
	"github.com/li41/astrahold-server/internal/transport"
	"github.com/li41/astrahold-server/internal/worldruntime"
)

const testGameplaySHA = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

type moveCall struct { id session.ID; seq uint32; input protocol.ClientMoveInput }
type actionCall struct { id session.ID; seq uint32; action protocol.ClientUseAction }

type fakeRuntime struct {
	joins chan worldruntime.JoinRequest
	leaves chan session.ID
	moves chan moveCall
	actions chan actionCall
}

func newFakeRuntime() *fakeRuntime {
	return &fakeRuntime{joins: make(chan worldruntime.JoinRequest,4), leaves: make(chan session.ID,4), moves: make(chan moveCall,4), actions: make(chan actionCall,4)}
}
func (f *fakeRuntime) EnqueueJoin(r worldruntime.JoinRequest) error { f.joins <- r; return nil }
func (f *fakeRuntime) EnqueueLeave(id session.ID) error { f.leaves <- id; return nil }
func (f *fakeRuntime) EnqueueMove(id session.ID, seq uint32, in protocol.ClientMoveInput) error { f.moves <- moveCall{id:id,seq:seq,input:in}; return nil }
func (f *fakeRuntime) EnqueueUseAction(id session.ID, seq uint32, action protocol.ClientUseAction) error { f.actions <- actionCall{id:id,seq:seq,action:action}; return nil }

func TestOpenRejectsMissingWorldIdentity(t *testing.T) {
	cfg := DefaultConfig(); cfg.TCPAddress="127.0.0.1:0"; cfg.UDPAddress="127.0.0.1:0"
	server := NewServer(cfg, newFakeRuntime(), gamev1.Codec{})
	if err := server.Open(); err != ErrInvalidWorldIdentity { t.Fatalf("Open() error = %v, want ErrInvalidWorldIdentity", err) }
}

func TestTCPUDPHandshakeAndRouting(t *testing.T) {
	runtime := newFakeRuntime()
	cfg := DefaultConfig(); cfg.TCPAddress="127.0.0.1:0"; cfg.UDPAddress="127.0.0.1:0"
	cfg.WorldIdentity = protocol.WorldIdentity{WorldID:"castle-sandbox", Revision:"s3d-001", GameplaySHA256:testGameplaySHA}
	codec := gamev1.Codec{}
	server := NewServer(cfg,runtime,codec); if err := server.Open(); err != nil { t.Fatal(err) }
	ctx,cancel := context.WithCancel(context.Background()); defer cancel()
	serveDone := make(chan error,1); go func(){ serveDone <- server.Serve(ctx) }()

	tcpConn,err := net.Dial("tcp",server.TCPAddr().String()); if err != nil { t.Fatal(err) }; defer tcpConn.Close()
	welcomeEnvelope,err := transport.ReadEnvelope(tcpConn,codec); if err != nil { t.Fatal(err) }
	welcome,ok := welcomeEnvelope.Message.(protocol.SessionWelcome); if !ok { t.Fatalf("unexpected welcome: %#v",welcomeEnvelope.Message) }
	if welcome.SessionID==0 || welcome.EntityID==0 || welcome.RealtimePort==0 || len(welcome.RealtimeToken)!=32 { t.Fatalf("invalid welcome: %#v",welcome) }
	if welcome.World != cfg.WorldIdentity { t.Fatalf("world identity mismatch: got=%+v want=%+v",welcome.World,cfg.WorldIdentity) }

	var join worldruntime.JoinRequest
	select { case join=<-runtime.joins: case <-time.After(time.Second): t.Fatal("join not enqueued") }
	if uint64(join.Session.ID)!=welcome.SessionID || join.Entity.ID!=welcome.EntityID { t.Fatal("join/welcome mismatch") }

	if err := join.Session.Connection().TrySend(protocol.Envelope{Delivery:protocol.DeliveryReliableOrdered,Sequence:1,Message:protocol.EntityDespawn{EntityID:999}}); err != nil { t.Fatal(err) }
	reliable,err := transport.ReadEnvelope(tcpConn,codec); if err != nil { t.Fatal(err) }
	if reliable.Delivery!=protocol.DeliveryReliableOrdered || reliable.Sequence!=1 { t.Fatalf("reliable route mismatch: %#v",reliable) }

	actionMessage := protocol.ClientUseAction{ActionID:"basic-attack",TargetKind:protocol.ActionTargetGate,TargetID:"main-gate"}
	action := protocol.Envelope{Delivery:protocol.DeliveryReliableOrdered,Sequence:7,Message:actionMessage}
	if err := transport.WriteEnvelope(tcpConn,action,codec); err != nil { t.Fatal(err) }
	select {
	case call := <-runtime.actions:
		if uint64(call.id)!=welcome.SessionID || call.seq!=7 || call.action!=actionMessage { t.Fatalf("action route mismatch: %#v",call) }
	case <-time.After(time.Second): t.Fatal("action not routed")
	}

	token,err := ParseToken(welcome.RealtimeToken); if err != nil { t.Fatal(err) }
	udpConn,err := net.DialUDP("udp",nil,server.UDPAddr()); if err != nil { t.Fatal(err) }; defer udpConn.Close()
	moveEnvelope := protocol.Envelope{Delivery:protocol.DeliveryRealtimeSequenced,Sequence:3,Message:protocol.ClientMoveInput{DirectionX:1}}
	packet,err := EncodeDatagram(token,moveEnvelope,codec); if err != nil { t.Fatal(err) }
	if _,err := udpConn.Write(packet); err != nil { t.Fatal(err) }
	select { case move:=<-runtime.moves: if uint64(move.id)!=welcome.SessionID || move.seq!=3 || move.input.DirectionX!=1 { t.Fatalf("move mismatch: %#v",move) }; case <-time.After(time.Second): t.Fatal("move not routed") }

	correction := protocol.Envelope{Delivery:protocol.DeliveryRealtimeSequenced,Sequence:4,ServerTick:10,Message:protocol.PositionCorrection{Tick:10,EntityID:welcome.EntityID}}
	if err := join.Session.Connection().TrySend(correction); err != nil { t.Fatal(err) }
	if err := udpConn.SetReadDeadline(time.Now().Add(time.Second)); err != nil { t.Fatal(err) }
	buffer := make([]byte,MaxDatagramSize); n,err := udpConn.Read(buffer); if err != nil { t.Fatal(err) }
	gotToken,got,err := DecodeDatagram(buffer[:n],codec); if err != nil { t.Fatal(err) }
	if gotToken!=token || got.Sequence!=4 || got.ServerTick!=10 { t.Fatal("realtime route mismatch") }
	if _,ok := got.Message.(protocol.PositionCorrection); !ok { t.Fatalf("unexpected realtime message: %#v",got.Message) }

	_ = tcpConn.Close()
	select { case id:=<-runtime.leaves: if uint64(id)!=welcome.SessionID { t.Fatal("leave mismatch") }; case <-time.After(time.Second): t.Fatal("leave not enqueued") }
	cancel(); _ = server.Close()
	select { case err:=<-serveDone: if err!=nil { t.Fatalf("serve error: %v",err) }; case <-time.After(time.Second): t.Fatal("serve did not stop") }
}
