package browserws

import (
	"testing"

	"github.com/li41/astrahold-server/internal/characteridentity"
	"github.com/li41/astrahold-server/internal/protocol"
	"github.com/li41/astrahold-server/internal/session"
	"github.com/li41/astrahold-server/internal/world"
	"github.com/li41/astrahold-server/internal/worldruntime"
)

type fencedInstanceBrowserRuntime struct {
	*fakeRuntime
	gotFence   worldruntime.SessionOwnershipFence
	gotCommand protocol.ClientEquipmentInstanceCommand
}

func (r *fencedInstanceBrowserRuntime) EnqueueFencedEquipmentInstanceCommand(fence worldruntime.SessionOwnershipFence, _ uint32, command protocol.ClientEquipmentInstanceCommand) error {
	r.gotFence = fence
	r.gotCommand = command
	return nil
}

func TestOwnedCommandSinkFencesV28EquipmentInstanceIntent(t *testing.T) {
	runtime := &fencedInstanceBrowserRuntime{fakeRuntime: newFakeRuntime()}
	fence := worldruntime.SessionOwnershipFence{
		SessionID: session.ID(7),
		EntityID: world.EntityID(70),
		CharacterID: characteridentity.ID("character:browser-instance"),
		Epoch: 3,
	}
	sink := ownedCommandSink{runtime: runtime, ownership: fence}
	command := protocol.ClientEquipmentInstanceCommand{
		Operation: protocol.EquipmentOperationEquip,
		Slot: protocol.EquipmentSlotMainHand,
		ItemInstanceID: "item-instance:mid-sword-1",
	}
	if err := sink.EnqueueEquipmentInstanceCommand(fence.SessionID, 11, command); err != nil { t.Fatal(err) }
	if runtime.gotFence != fence { t.Fatalf("fence=%#v want=%#v", runtime.gotFence, fence) }
	if runtime.gotCommand != command { t.Fatalf("command=%#v want=%#v", runtime.gotCommand, command) }
}

func TestOwnedCommandSinkRejectsEquipmentInstanceForWrongSession(t *testing.T) {
	runtime := &fencedInstanceBrowserRuntime{fakeRuntime: newFakeRuntime()}
	fence := worldruntime.SessionOwnershipFence{SessionID: 7, EntityID: 70, CharacterID: characteridentity.ID("character:browser-instance"), Epoch: 3}
	sink := ownedCommandSink{runtime: runtime, ownership: fence}
	err := sink.EnqueueEquipmentInstanceCommand(8, 11, protocol.ClientEquipmentInstanceCommand{Operation: protocol.EquipmentOperationUnequip, Slot: protocol.EquipmentSlotMainHand})
	if err != worldruntime.ErrCharacterOwnershipFenceInvalid { t.Fatalf("err=%v want=%v", err, worldruntime.ErrCharacterOwnershipFenceInvalid) }
	if runtime.gotFence.Valid() { t.Fatalf("runtime received rejected fence=%#v", runtime.gotFence) }
}
