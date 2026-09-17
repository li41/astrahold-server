package gateway

import (
	"errors"
	"testing"

	"github.com/li41/astrahold-server/internal/protocol"
	"github.com/li41/astrahold-server/internal/session"
)

type warehouseIngressSink struct {
	fakeSink
	command protocol.ClientWarehouseCommand
}

func (s *warehouseIngressSink) EnqueueWarehouseCommand(id session.ID, sequence uint32, command protocol.ClientWarehouseCommand) error {
	s.sessionID = id
	s.sequence = sequence
	s.command = command
	return s.err
}

func TestIngressRoutesReliableWarehouseCommand(t *testing.T) {
	sink := &warehouseIngressSink{}
	command := protocol.ClientWarehouseCommand{
		Operation:       protocol.WarehouseOperationWithdrawGM,
		ItemArchetypeID: "item_astrahold_weapon_enhancement_scroll",
		Quantity:        2,
	}
	if err := NewIngress(sink).Handle(7, protocol.Envelope{Delivery: protocol.DeliveryReliableOrdered, Sequence: 11, Message: command}); err != nil {
		t.Fatal(err)
	}
	if sink.sessionID != 7 || sink.sequence != 11 || sink.command != command {
		t.Fatalf("routed=%#v", sink)
	}
}

func TestIngressRejectsRealtimeWarehouseCommand(t *testing.T) {
	err := NewIngress(&warehouseIngressSink{}).Handle(7, protocol.Envelope{
		Delivery: protocol.DeliveryRealtimeSequenced,
		Sequence: 12,
		Message:  protocol.ClientWarehouseCommand{Operation: protocol.WarehouseOperationOpenPersonal},
	})
	if !errors.Is(err, ErrInvalidClientDelivery) {
		t.Fatalf("err=%v want=%v", err, ErrInvalidClientDelivery)
	}
}

func TestIngressLeavesWarehouseOperationLegalityToWorldOwner(t *testing.T) {
	sink := &warehouseIngressSink{}
	legacy := protocol.ClientWarehouseCommand{Operation: protocol.WarehouseOperation("open")}
	if err := NewIngress(sink).Handle(8, protocol.Envelope{Delivery: protocol.DeliveryReliableOrdered, Sequence: 13, Message: legacy}); err != nil {
		t.Fatal(err)
	}
	if sink.command != legacy {
		t.Fatalf("routed=%#v want=%#v", sink.command, legacy)
	}
}
