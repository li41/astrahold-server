package gateway

import (
	"errors"
	"testing"

	"github.com/li41/astrahold-server/internal/protocol"
	"github.com/li41/astrahold-server/internal/session"
)

type equipmentInstanceIngressSink struct {
	fakeSink
	command protocol.ClientEquipmentInstanceCommand
}

func (s *equipmentInstanceIngressSink) EnqueueEquipmentInstanceCommand(id session.ID, sequence uint32, command protocol.ClientEquipmentInstanceCommand) error {
	s.sessionID = id
	s.sequence = sequence
	s.command = command
	return s.err
}

func TestIngressRoutesReliableExactEquipmentInstance(t *testing.T) {
	sink := &equipmentInstanceIngressSink{}
	ingress := NewIngress(sink)
	command := protocol.ClientEquipmentInstanceCommand{
		Operation:      protocol.EquipmentOperationEquip,
		Slot:           protocol.EquipmentSlotMainHand,
		ItemInstanceID: "item-instance:mid-1",
	}
	if err := ingress.Handle(7, protocol.Envelope{Delivery: protocol.DeliveryReliableOrdered, Sequence: 11, Message: command}); err != nil {
		t.Fatal(err)
	}
	if sink.sessionID != 7 || sink.sequence != 11 || sink.command != command {
		t.Fatalf("routed=%#v", sink)
	}
}

func TestIngressAcceptsSlotOnlyEquipmentInstanceUnequip(t *testing.T) {
	sink := &equipmentInstanceIngressSink{}
	command := protocol.ClientEquipmentInstanceCommand{Operation: protocol.EquipmentOperationUnequip, Slot: protocol.EquipmentSlotOffHand}
	if err := NewIngress(sink).Handle(8, protocol.Envelope{Delivery: protocol.DeliveryReliableOrdered, Sequence: 12, Message: command}); err != nil {
		t.Fatal(err)
	}
	if sink.command != command {
		t.Fatalf("routed=%#v want=%#v", sink.command, command)
	}
}

func TestIngressRejectsInvalidEquipmentInstanceIntent(t *testing.T) {
	ingress := NewIngress(&equipmentInstanceIngressSink{})
	tests := []struct {
		name     string
		delivery protocol.Delivery
		command  protocol.ClientEquipmentInstanceCommand
		want     error
	}{
		{name: "realtime", delivery: protocol.DeliveryRealtimeSequenced, command: protocol.ClientEquipmentInstanceCommand{Operation: protocol.EquipmentOperationEquip, Slot: protocol.EquipmentSlotMainHand, ItemInstanceID: "item-instance:1"}, want: ErrInvalidClientDelivery},
		{name: "padded identity", delivery: protocol.DeliveryReliableOrdered, command: protocol.ClientEquipmentInstanceCommand{Operation: protocol.EquipmentOperationEquip, Slot: protocol.EquipmentSlotMainHand, ItemInstanceID: " item-instance:1 "}, want: ErrInvalidClientEnvelope},
		{name: "missing identity", delivery: protocol.DeliveryReliableOrdered, command: protocol.ClientEquipmentInstanceCommand{Operation: protocol.EquipmentOperationEquip, Slot: protocol.EquipmentSlotMainHand}, want: ErrInvalidClientEnvelope},
		{name: "unequip identity", delivery: protocol.DeliveryReliableOrdered, command: protocol.ClientEquipmentInstanceCommand{Operation: protocol.EquipmentOperationUnequip, Slot: protocol.EquipmentSlotMainHand, ItemInstanceID: "item-instance:1"}, want: ErrInvalidClientEnvelope},
		{name: "bad slot", delivery: protocol.DeliveryReliableOrdered, command: protocol.ClientEquipmentInstanceCommand{Operation: protocol.EquipmentOperationEquip, Slot: protocol.EquipmentSlot("unknown"), ItemInstanceID: "item-instance:1"}, want: ErrInvalidClientEnvelope},
	}
	for i, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := ingress.Handle(9, protocol.Envelope{Delivery: test.delivery, Sequence: uint32(i + 1), Message: test.command})
			if !errors.Is(err, test.want) {
				t.Fatalf("err=%v want=%v", err, test.want)
			}
		})
	}
}
