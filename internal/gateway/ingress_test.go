package gateway

import (
	"errors"
	"math"
	"testing"

	"github.com/li41/astrahold-server/internal/protocol"
	"github.com/li41/astrahold-server/internal/session"
)

type fakeSink struct {
	sessionID         session.ID
	sequence          uint32
	input             protocol.ClientMoveInput
	action            protocol.ClientUseAction
	equipment         protocol.ClientEquipmentCommand
	equipmentInstance protocol.ClientEquipmentInstanceCommand
	err               error
}

func (f *fakeSink) EnqueueMove(id session.ID, sequence uint32, input protocol.ClientMoveInput) error {
	f.sessionID = id
	f.sequence = sequence
	f.input = input
	return f.err
}

func (f *fakeSink) EnqueueUseAction(id session.ID, sequence uint32, action protocol.ClientUseAction) error {
	f.sessionID = id
	f.sequence = sequence
	f.action = action
	return f.err
}

func (f *fakeSink) EnqueueEquipmentCommand(id session.ID, sequence uint32, command protocol.ClientEquipmentCommand) error {
	f.sessionID = id
	f.sequence = sequence
	f.equipment = command
	return f.err
}

func (f *fakeSink) EnqueueEquipmentInstanceCommand(id session.ID, sequence uint32, command protocol.ClientEquipmentInstanceCommand) error {
	f.sessionID = id
	f.sequence = sequence
	f.equipmentInstance = command
	return f.err
}

func TestIngressUsesEnvelopeSequenceForMove(t *testing.T) {
	sink := &fakeSink{}
	ingress := NewIngress(sink)
	err := ingress.Handle(7, protocol.Envelope{Delivery: protocol.DeliveryRealtimeSequenced, Sequence: 42, Message: protocol.ClientMoveInput{DirectionX: 1, DirectionZ: -0.25}})
	if err != nil {
		t.Fatal(err)
	}
	if sink.sessionID != 7 || sink.sequence != 42 {
		t.Fatalf("unexpected routing: session=%d sequence=%d", sink.sessionID, sink.sequence)
	}
}

func TestIngressRoutesReliableAction(t *testing.T) {
	sink := &fakeSink{}
	ingress := NewIngress(sink)
	action := protocol.ClientUseAction{ActionID: "basic-attack", TargetKind: protocol.ActionTargetGate, TargetID: "main-gate"}
	err := ingress.Handle(9, protocol.Envelope{Delivery: protocol.DeliveryReliableOrdered, Sequence: 5, Message: action})
	if err != nil {
		t.Fatal(err)
	}
	if sink.sessionID != 9 || sink.sequence != 5 || sink.action != action {
		t.Fatalf("unexpected action routing: %#v", sink)
	}
}

func TestIngressRoutesReliablePointActionWithoutTargetID(t *testing.T) {
	sink := &fakeSink{}
	ingress := NewIngress(sink)
	x, z := float32(4.5), float32(-12.25)
	action := protocol.ClientUseAction{
		ActionID:   "fireball",
		TargetKind: protocol.ActionTargetPoint,
		TargetX:    &x,
		TargetZ:    &z,
	}
	err := ingress.Handle(9, protocol.Envelope{Delivery: protocol.DeliveryReliableOrdered, Sequence: 6, Message: action})
	if err != nil {
		t.Fatal(err)
	}
	if sink.sessionID != 9 || sink.sequence != 6 || sink.action.ActionID != action.ActionID || sink.action.TargetKind != protocol.ActionTargetPoint || sink.action.TargetID != "" || sink.action.TargetX == nil || sink.action.TargetZ == nil || *sink.action.TargetX != x || *sink.action.TargetZ != z {
		t.Fatalf("unexpected point action routing: %#v", sink)
	}
}

func TestIngressRoutesGenericEquipmentAcrossFormalSlots(t *testing.T) {
	for i, slot := range protocol.EquipmentSlots() {
		sink := &fakeSink{}
		ingress := NewIngress(sink)
		command := protocol.ClientEquipmentCommand{
			Operation: protocol.EquipmentOperationUnequip,
			Slot:      slot,
		}
		err := ingress.Handle(7, protocol.Envelope{
			Delivery: protocol.DeliveryReliableOrdered,
			Sequence: uint32(i + 1),
			Message:  command,
		})
		if err != nil {
			t.Fatalf("slot %q: %v", slot, err)
		}
		if sink.sessionID != 7 || sink.sequence != uint32(i+1) || sink.equipment != command {
			t.Fatalf("slot %q unexpected routing: %#v", slot, sink)
		}
	}
}

func TestIngressRoutesExactEquipmentAcrossFormalSlots(t *testing.T) {
	for i, slot := range protocol.EquipmentSlots() {
		sink := &fakeSink{}
		ingress := NewIngress(sink)
		command := protocol.ClientEquipmentInstanceCommand{
			Operation:      protocol.EquipmentOperationEquip,
			Slot:           slot,
			ItemInstanceID: "instance-1",
		}
		err := ingress.Handle(8, protocol.Envelope{
			Delivery: protocol.DeliveryReliableOrdered,
			Sequence: uint32(i + 1),
			Message:  command,
		})
		if err != nil {
			t.Fatalf("slot %q: %v", slot, err)
		}
		if sink.sessionID != 8 || sink.sequence != uint32(i+1) || sink.equipmentInstance != command {
			t.Fatalf("slot %q unexpected routing: %#v", slot, sink)
		}
	}
}

func TestIngressRejectsUnknownEquipmentSlot(t *testing.T) {
	ingress := NewIngress(&fakeSink{})
	tests := []any{
		protocol.ClientEquipmentCommand{
			Operation: protocol.EquipmentOperationUnequip,
			Slot:      protocol.EquipmentSlot("unknown"),
		},
		protocol.ClientEquipmentInstanceCommand{
			Operation:      protocol.EquipmentOperationEquip,
			Slot:           protocol.EquipmentSlot("unknown"),
			ItemInstanceID: "instance-1",
		},
	}
	for i, message := range tests {
		err := ingress.Handle(1, protocol.Envelope{
			Delivery: protocol.DeliveryReliableOrdered,
			Sequence: uint32(i + 1),
			Message:  message,
		})
		if !errors.Is(err, ErrInvalidClientEnvelope) {
			t.Fatalf("case %d err=%v, want ErrInvalidClientEnvelope", i, err)
		}
	}
}

func TestIngressRejectsMalformedPointAction(t *testing.T) {
	ingress := NewIngress(&fakeSink{})
	finite := float32(1)
	nan := float32(math.NaN())
	tests := []protocol.ClientUseAction{
		{ActionID: "fireball", TargetKind: protocol.ActionTargetPoint},
		{ActionID: "fireball", TargetKind: protocol.ActionTargetPoint, TargetID: "unexpected", TargetX: &finite, TargetZ: &finite},
		{ActionID: "fireball", TargetKind: protocol.ActionTargetPoint, TargetX: &finite},
		{ActionID: "fireball", TargetKind: protocol.ActionTargetPoint, TargetX: &nan, TargetZ: &finite},
	}
	for i, action := range tests {
		err := ingress.Handle(1, protocol.Envelope{Delivery: protocol.DeliveryReliableOrdered, Sequence: uint32(i + 1), Message: action})
		if !errors.Is(err, ErrInvalidClientEnvelope) {
			t.Fatalf("case %d err=%v, want ErrInvalidClientEnvelope", i, err)
		}
	}
}

func TestIngressRejectsCoordinatesOnEntityAction(t *testing.T) {
	ingress := NewIngress(&fakeSink{})
	x := float32(1)
	err := ingress.Handle(1, protocol.Envelope{Delivery: protocol.DeliveryReliableOrdered, Sequence: 1, Message: protocol.ClientUseAction{ActionID: "basic-attack", TargetKind: protocol.ActionTargetEntity, TargetID: "42", TargetX: &x}})
	if !errors.Is(err, ErrInvalidClientEnvelope) {
		t.Fatalf("err=%v, want ErrInvalidClientEnvelope", err)
	}
}

func TestIngressRejectsActionOnRealtime(t *testing.T) {
	ingress := NewIngress(&fakeSink{})
	err := ingress.Handle(1, protocol.Envelope{Delivery: protocol.DeliveryRealtimeSequenced, Sequence: 1, Message: protocol.ClientUseAction{ActionID: "basic-attack", TargetKind: protocol.ActionTargetGate, TargetID: "main-gate"}})
	if !errors.Is(err, ErrInvalidClientDelivery) {
		t.Fatalf("err=%v, want ErrInvalidClientDelivery", err)
	}
}

func TestIngressRejectsIncompleteAction(t *testing.T) {
	ingress := NewIngress(&fakeSink{})
	err := ingress.Handle(1, protocol.Envelope{Delivery: protocol.DeliveryReliableOrdered, Sequence: 1, Message: protocol.ClientUseAction{ActionID: "basic-attack"}})
	if !errors.Is(err, ErrInvalidClientEnvelope) {
		t.Fatalf("err=%v, want ErrInvalidClientEnvelope", err)
	}
}

func TestIngressRejectsReliableMove(t *testing.T) {
	ingress := NewIngress(&fakeSink{})
	err := ingress.Handle(1, protocol.Envelope{Delivery: protocol.DeliveryReliableOrdered, Sequence: 1, Message: protocol.ClientMoveInput{DirectionX: 1}})
	if !errors.Is(err, ErrInvalidClientDelivery) {
		t.Fatalf("err=%v, want ErrInvalidClientDelivery", err)
	}
}

func TestIngressRejectsServerOnlyMessage(t *testing.T) {
	ingress := NewIngress(&fakeSink{})
	err := ingress.Handle(1, protocol.Envelope{Delivery: protocol.DeliveryReliableOrdered, Sequence: 1, Message: protocol.EntityDespawn{EntityID: 99}})
	if !errors.Is(err, ErrUnsupportedClientMessage) {
		t.Fatalf("err=%v, want ErrUnsupportedClientMessage", err)
	}
}
