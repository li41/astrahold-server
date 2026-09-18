package browserws

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/li41/astrahold-server/internal/characteridentity"
	"github.com/li41/astrahold-server/internal/characterstate"
	"github.com/li41/astrahold-server/internal/characterstats"
	"github.com/li41/astrahold-server/internal/codec/gamev1"
	"github.com/li41/astrahold-server/internal/equipmentcatalog"
	"github.com/li41/astrahold-server/internal/gameplayworld"
	"github.com/li41/astrahold-server/internal/iteminstance"
	"github.com/li41/astrahold-server/internal/movement"
	"github.com/li41/astrahold-server/internal/navigation"
	"github.com/li41/astrahold-server/internal/protocol"
	"github.com/li41/astrahold-server/internal/session"
	"github.com/li41/astrahold-server/internal/simulation"
	"github.com/li41/astrahold-server/internal/spatial"
	"github.com/li41/astrahold-server/internal/world"
	"github.com/li41/astrahold-server/internal/worldruntime"
)

const (
	armorLiveLowChest  = "item_low_cloth_chest"
	armorLiveLowHelmet = "item_low_cloth_helmet"
	armorLiveMidChest  = "item_garrison_steel_cuirass"
	armorLiveHighChest = "item_starforged_bastion_cuirass"

	armorLiveMidInstanceID  = "item-instance:browserws-mid-chest"
	armorLiveHighInstanceID = "item-instance:browserws-high-chest"
)

type armorLiveRoller struct{}

func (*armorLiveRoller) Intn(int) int { return 0 }

type armorEquipmentLiveFixture struct {
	ctx   context.Context
	conn  *websocket.Conn
	codec gamev1.Codec
}

type armorOwnerState struct {
	inventory          protocol.InventorySnapshot
	inventoryInstances protocol.InventoryInstanceSnapshot
	equipment          protocol.EquipmentSnapshot
	equipmentInstances protocol.EquipmentInstanceSnapshot

	haveInventory          bool
	haveInventoryInstances bool
	haveEquipment          bool
	haveEquipmentInstances bool
}

func (s armorOwnerState) complete() bool {
	return s.haveInventory &&
		s.haveInventoryInstances &&
		s.haveEquipment &&
		s.haveEquipmentInstances
}

func TestArmorEquipmentBrowserWSV33RoundTripAndRejections(t *testing.T) {
	mid, high := armorLiveInstances(t)
	inventoryState, err := characterstate.NewInventoryStateWithSlots(
		[]characterstate.InventoryStack{
			{ItemArchetypeID: armorLiveLowChest, Quantity: 1},
			{ItemArchetypeID: armorLiveLowHelmet, Quantity: 1},
		},
		[]iteminstance.Instance{mid, high},
		nil,
		nil,
	)
	if err != nil {
		t.Fatalf("build armor BrowserWS inventory: %v", err)
	}
	fixture := newArmorEquipmentLiveFixture(
		t,
		"round-trip",
		inventoryState,
		characterstats.DefaultPrimary(),
	)

	initial := readArmorOwnerStateUntil(t, fixture, "initial owner state", func(state armorOwnerState) bool {
		return state.complete() &&
			inventorySnapshotQuantity(state.inventory, armorLiveLowChest) == 1 &&
			inventorySnapshotQuantity(state.inventory, armorLiveLowHelmet) == 1 &&
			inventoryInstancePresent(state.inventoryInstances, armorLiveMidInstanceID) &&
			inventoryInstancePresent(state.inventoryInstances, armorLiveHighInstanceID) &&
			equipmentArchetype(state.equipment, protocol.EquipmentSlotChest) == "" &&
			equipmentInstanceID(state.equipmentInstances, protocol.EquipmentSlotChest) == ""
	})
	assertProtocolInstanceMatches(t, inventoryInstance(initial.inventoryInstances, armorLiveMidInstanceID), mid)
	assertProtocolInstanceMatches(t, inventoryInstance(initial.inventoryInstances, armorLiveHighInstanceID), high)

	// Type3 must traverse the real BrowserWS gateway for an armor slot, with the resulting
	// authoritative replacement snapshots moving the low-tier chest out of the stack inventory.
	writeBrowserIntent(t, fixture.ctx, fixture.conn, fixture.codec, 1, protocol.ClientEquipmentCommand{
		Operation:       protocol.EquipmentOperationEquip,
		Slot:            protocol.EquipmentSlotChest,
		ItemArchetypeID: armorLiveLowChest,
	})
	readArmorOwnerStateUntil(t, fixture, "low chest equipped", func(state armorOwnerState) bool {
		return state.complete() &&
			inventorySnapshotQuantity(state.inventory, armorLiveLowChest) == 0 &&
			equipmentArchetype(state.equipment, protocol.EquipmentSlotChest) == armorLiveLowChest &&
			inventoryInstancePresent(state.inventoryInstances, armorLiveMidInstanceID) &&
			inventoryInstancePresent(state.inventoryInstances, armorLiveHighInstanceID) &&
			equipmentInstanceID(state.equipmentInstances, protocol.EquipmentSlotChest) == ""
	})

	writeBrowserIntent(t, fixture.ctx, fixture.conn, fixture.codec, 2, protocol.ClientEquipmentCommand{
		Operation: protocol.EquipmentOperationUnequip,
		Slot:      protocol.EquipmentSlotChest,
	})
	readArmorOwnerStateUntil(t, fixture, "low chest unequipped", func(state armorOwnerState) bool {
		return state.complete() &&
			inventorySnapshotQuantity(state.inventory, armorLiveLowChest) == 1 &&
			equipmentArchetype(state.equipment, protocol.EquipmentSlotChest) == ""
	})

	// Rejected equipment commands intentionally have no optimistic/result packet. Each rejection is
	// followed by a valid helmet mutation; the complete owner batch proves the rejected command did
	// not mutate the chest or unique-instance inventory and also proves the connection stayed alive.
	writeBrowserIntent(t, fixture.ctx, fixture.conn, fixture.codec, 3, protocol.ClientEquipmentInstanceCommand{
		Operation:      protocol.EquipmentOperationEquip,
		Slot:           protocol.EquipmentSlotHelmet,
		ItemInstanceID: armorLiveMidInstanceID,
	})
	probeArmorRejectionWithHelmet(t, fixture, 4, "wrong-slot rejection", mid, high)
	resetArmorProbeHelmet(t, fixture, 5)

	writeBrowserIntent(t, fixture.ctx, fixture.conn, fixture.codec, 6, protocol.ClientEquipmentInstanceCommand{
		Operation:      protocol.EquipmentOperationEquip,
		Slot:           protocol.EquipmentSlotChest,
		ItemInstanceID: "item-instance:missing",
	})
	probeArmorRejectionWithHelmet(t, fixture, 7, "missing-instance rejection", mid, high)
	resetArmorProbeHelmet(t, fixture, 8)

	// Starforged Bastion requires durable base Constitution >= 18. The fixture deliberately uses
	// neutral current-schema stats (Constitution 10), so this exact instance must remain in the bag.
	writeBrowserIntent(t, fixture.ctx, fixture.conn, fixture.codec, 9, protocol.ClientEquipmentInstanceCommand{
		Operation:      protocol.EquipmentOperationEquip,
		Slot:           protocol.EquipmentSlotChest,
		ItemInstanceID: armorLiveHighInstanceID,
	})
	probeArmorRejectionWithHelmet(t, fixture, 10, "base-requirement rejection", mid, high)
	resetArmorProbeHelmet(t, fixture, 11)

	// A legal mid-tier exact-instance chest equip must then succeed through the same connection.
	writeBrowserIntent(t, fixture.ctx, fixture.conn, fixture.codec, 12, protocol.ClientEquipmentInstanceCommand{
		Operation:      protocol.EquipmentOperationEquip,
		Slot:           protocol.EquipmentSlotChest,
		ItemInstanceID: armorLiveMidInstanceID,
	})
	equipped := readArmorOwnerStateUntil(t, fixture, "mid chest equipped", func(state armorOwnerState) bool {
		return state.complete() &&
			!inventoryInstancePresent(state.inventoryInstances, armorLiveMidInstanceID) &&
			inventoryInstancePresent(state.inventoryInstances, armorLiveHighInstanceID) &&
			equipmentArchetype(state.equipment, protocol.EquipmentSlotChest) == armorLiveMidChest &&
			equipmentInstanceID(state.equipmentInstances, protocol.EquipmentSlotChest) == armorLiveMidInstanceID
	})
	assertProtocolInstanceMatches(
		t,
		equipmentInstance(equipped.equipmentInstances, protocol.EquipmentSlotChest),
		mid,
	)

	writeBrowserIntent(t, fixture.ctx, fixture.conn, fixture.codec, 13, protocol.ClientEquipmentInstanceCommand{
		Operation: protocol.EquipmentOperationUnequip,
		Slot:      protocol.EquipmentSlotChest,
	})
	final := readArmorOwnerStateUntil(t, fixture, "mid chest unequipped", func(state armorOwnerState) bool {
		return state.complete() &&
			inventoryInstancePresent(state.inventoryInstances, armorLiveMidInstanceID) &&
			inventoryInstancePresent(state.inventoryInstances, armorLiveHighInstanceID) &&
			equipmentInstanceID(state.equipmentInstances, protocol.EquipmentSlotChest) == ""
	})
	assertProtocolInstanceMatches(t, inventoryInstance(final.inventoryInstances, armorLiveMidInstanceID), mid)
	assertProtocolInstanceMatches(t, inventoryInstance(final.inventoryInstances, armorLiveHighInstanceID), high)
}

func TestArmorEquipmentBrowserWSV33RestoresGenericAndUniqueArmorOnJoin(t *testing.T) {
	mid, high := armorLiveInstances(t)
	midJSON, err := iteminstance.CanonicalShapeJSON(mid)
	if err != nil {
		t.Fatalf("encode equipped mid chest: %v", err)
	}
	inventoryState, err := characterstate.NewInventoryStateWithSlots(
		nil,
		[]iteminstance.Instance{high},
		[]characterstate.EquipmentSlotState{{
			Slot:            string(protocol.EquipmentSlotHelmet),
			ItemArchetypeID: armorLiveLowHelmet,
		}},
		[]characterstate.EquipmentInstanceSlotState{{
			Slot:             string(protocol.EquipmentSlotChest),
			ItemInstanceJSON: string(midJSON),
		}},
	)
	if err != nil {
		t.Fatalf("build restored armor inventory: %v", err)
	}

	fixture := newArmorEquipmentLiveFixture(
		t,
		"restore",
		inventoryState,
		characterstats.DefaultPrimary(),
	)
	restored := readArmorOwnerStateUntil(t, fixture, "restored owner batch", func(state armorOwnerState) bool {
		return state.complete() &&
			equipmentArchetype(state.equipment, protocol.EquipmentSlotHelmet) == armorLiveLowHelmet &&
			equipmentInstanceID(state.equipmentInstances, protocol.EquipmentSlotChest) == armorLiveMidInstanceID &&
			!inventoryInstancePresent(state.inventoryInstances, armorLiveMidInstanceID) &&
			inventoryInstancePresent(state.inventoryInstances, armorLiveHighInstanceID)
	})
	assertProtocolInstanceMatches(
		t,
		equipmentInstance(restored.equipmentInstances, protocol.EquipmentSlotChest),
		mid,
	)
	assertProtocolInstanceMatches(
		t,
		inventoryInstance(restored.inventoryInstances, armorLiveHighInstanceID),
		high,
	)
}

func probeArmorRejectionWithHelmet(
	t *testing.T,
	fixture armorEquipmentLiveFixture,
	sequence uint32,
	label string,
	mid iteminstance.Instance,
	high iteminstance.Instance,
) {
	t.Helper()
	writeBrowserIntent(t, fixture.ctx, fixture.conn, fixture.codec, sequence, protocol.ClientEquipmentCommand{
		Operation:       protocol.EquipmentOperationEquip,
		Slot:            protocol.EquipmentSlotHelmet,
		ItemArchetypeID: armorLiveLowHelmet,
	})
	state := readArmorOwnerStateUntil(t, fixture, label, func(state armorOwnerState) bool {
		return state.complete() &&
			inventorySnapshotQuantity(state.inventory, armorLiveLowHelmet) == 0 &&
			equipmentArchetype(state.equipment, protocol.EquipmentSlotHelmet) == armorLiveLowHelmet &&
			equipmentArchetype(state.equipment, protocol.EquipmentSlotChest) == "" &&
			equipmentInstanceID(state.equipmentInstances, protocol.EquipmentSlotChest) == "" &&
			inventoryInstancePresent(state.inventoryInstances, armorLiveMidInstanceID) &&
			inventoryInstancePresent(state.inventoryInstances, armorLiveHighInstanceID)
	})
	assertProtocolInstanceMatches(t, inventoryInstance(state.inventoryInstances, armorLiveMidInstanceID), mid)
	assertProtocolInstanceMatches(t, inventoryInstance(state.inventoryInstances, armorLiveHighInstanceID), high)
}

func resetArmorProbeHelmet(t *testing.T, fixture armorEquipmentLiveFixture, sequence uint32) {
	t.Helper()
	writeBrowserIntent(t, fixture.ctx, fixture.conn, fixture.codec, sequence, protocol.ClientEquipmentCommand{
		Operation: protocol.EquipmentOperationUnequip,
		Slot:      protocol.EquipmentSlotHelmet,
	})
	readArmorOwnerStateUntil(t, fixture, "probe helmet reset", func(state armorOwnerState) bool {
		return state.complete() &&
			inventorySnapshotQuantity(state.inventory, armorLiveLowHelmet) == 1 &&
			equipmentArchetype(state.equipment, protocol.EquipmentSlotHelmet) == ""
	})
}

func newArmorEquipmentLiveFixture(
	t *testing.T,
	identitySuffix string,
	inventoryState characterstate.InventoryState,
	primary characterstats.Primary,
) armorEquipmentLiveFixture {
	t.Helper()

	loaded, err := gameplayworld.LoadFile("../../../worlds/gm-room/gameplay.json")
	if err != nil {
		t.Fatalf("load GM room gameplay: %v", err)
	}
	navigator, err := navigation.NewGameplayNavigator(loaded.Definition)
	if err != nil {
		t.Fatalf("build GM room navigator: %v", err)
	}
	worldIdentity := protocol.WorldIdentity{
		WorldID:        loaded.Definition.WorldID,
		Revision:       loaded.Definition.Revision,
		GameplaySHA256: loaded.SHA256,
	}
	stateWorld := characterstate.WorldRef{
		MapID:          string(gameplayworld.MapIDGMRoom),
		WorldID:        worldIdentity.WorldID,
		Revision:       worldIdentity.Revision,
		GameplaySHA256: worldIdentity.GameplaySHA256,
	}
	identity, err := characteridentity.NewTrusted("e2e-browserws-armor-" + identitySuffix)
	if err != nil {
		t.Fatalf("build armor trusted identity: %v", err)
	}
	position := world.Position{X: 0, Y: 0, Z: 0, Layer: 0}

	sim := simulation.New(spatial.NewGrid(16), movement.NewService(navigator, 0.1))
	config := worldruntime.DefaultConfig()
	config.SnapshotEveryTicks = 1
	runtime := worldruntime.New(
		sim,
		config,
		worldruntime.WithDynamicWorld(navigator),
		worldruntime.WithCharacterStateOutbox(nil, stateWorld),
	)
	loop, err := worldruntime.NewLoop(runtime, 40)
	if err != nil {
		t.Fatalf("build armor world loop: %v", err)
	}
	loopCtx, loopCancel := context.WithCancel(context.Background())
	t.Cleanup(loopCancel)
	loopDone := make(chan error, 1)
	go func() { loopDone <- loop.Run(loopCtx) }()
	t.Cleanup(func() {
		loopCancel()
		select {
		case err := <-loopDone:
			if err != nil {
				t.Errorf("armor world loop: %v", err)
			}
		case <-time.After(time.Second):
			t.Errorf("armor world loop did not stop")
		}
	})

	browserConfig := DefaultConfig()
	browserConfig.TickRateHz = 40
	browserConfig.SnapshotRateHz = 40
	browserConfig.WorldIdentity = worldIdentity
	browserConfig.PlayerFactory = func(_ session.ID, entityID world.EntityID) PlayerSpec {
		return PlayerSpec{
			Entity: world.EntityState{
				ID:        entityID,
				Kind:      world.EntityPlayer,
				Transform: world.Transform{Position: position},
			},
			Speed:         6,
			Radius:        loaded.Definition.Agent.Radius,
			MaxStepHeight: loaded.Definition.Agent.MaxStepHeight,
			AOIRadius:     64,
		}
	}
	browserConfig.TrustedE2EBootstrapFactory = func(_ session.ID, _ world.EntityID) (TrustedE2EBootstrap, error) {
		return TrustedE2EBootstrap{
			Identity: identity,
			Restore: worldruntime.CharacterRestore{
				SchemaVersion: characterstate.SchemaVersion,
				CharacterID:   identity.ID,
				Revision:      1,
				MapID:         gameplayworld.MapIDGMRoom,
				World:         worldIdentity,
				HP:            1000,
				MaxHP:         1000,
				MP:            100,
				MaxMP:         100,
				Transform:     world.Transform{Position: position},
				Inventory:     inventoryState,
				Warehouse:     characterstate.EmptyWarehouseState(),
				PrimaryStats:  primary,
			},
		}, nil
	}

	httpServer := httptest.NewServer(NewHandler(browserConfig, runtime, gamev1.Codec{}))
	t.Cleanup(httpServer.Close)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	conn, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(httpServer.URL, "http"), nil)
	if err != nil {
		t.Fatalf("dial armor BrowserWS: %v", err)
	}
	t.Cleanup(func() { _ = conn.CloseNow() })

	fixture := armorEquipmentLiveFixture{ctx: ctx, conn: conn, codec: gamev1.Codec{}}
	welcome := readEnvelope(t, ctx, conn, fixture.codec)
	if _, ok := welcome.Message.(protocol.SessionWelcome); !ok {
		t.Fatalf("first BrowserWS message=%T want SessionWelcome", welcome.Message)
	}
	return fixture
}

func armorLiveInstances(t *testing.T) (iteminstance.Instance, iteminstance.Instance) {
	t.Helper()
	catalog, err := equipmentcatalog.Default()
	if err != nil {
		t.Fatalf("load equipment catalog: %v", err)
	}
	create := func(id, archetype string) iteminstance.Instance {
		definition, ok := catalog.Resolve(archetype)
		if !ok {
			t.Fatalf("missing equipment definition %q", archetype)
		}
		instance, err := iteminstance.Create(iteminstance.ID(id), definition, &armorLiveRoller{})
		if err != nil {
			t.Fatalf("create %q instance: %v", archetype, err)
		}
		return instance
	}
	return create(armorLiveMidInstanceID, armorLiveMidChest),
		create(armorLiveHighInstanceID, armorLiveHighChest)
}

func readArmorOwnerStateUntil(
	t *testing.T,
	fixture armorEquipmentLiveFixture,
	label string,
	accept func(armorOwnerState) bool,
) armorOwnerState {
	t.Helper()
	var state armorOwnerState
	for i := 0; i < 320; i++ {
		envelope := readEnvelope(t, fixture.ctx, fixture.conn, fixture.codec)
		switch message := envelope.Message.(type) {
		case protocol.InventorySnapshot:
			state.inventory = message
			state.haveInventory = true
		case protocol.InventoryInstanceSnapshot:
			state.inventoryInstances = message
			state.haveInventoryInstances = true
		case protocol.EquipmentSnapshot:
			state.equipment = message
			state.haveEquipment = true
		case protocol.EquipmentInstanceSnapshot:
			state.equipmentInstances = message
			state.haveEquipmentInstances = true
		}
		if accept(state) {
			return state
		}
	}
	t.Fatalf("%s not observed: %#v", label, state)
	return armorOwnerState{}
}

func inventoryInstancePresent(snapshot protocol.InventoryInstanceSnapshot, itemInstanceID string) bool {
	for _, item := range snapshot.Items {
		if item.ItemInstanceID == itemInstanceID {
			return true
		}
	}
	return false
}

func inventoryInstance(snapshot protocol.InventoryInstanceSnapshot, itemInstanceID string) protocol.ItemInstanceState {
	for _, item := range snapshot.Items {
		if item.ItemInstanceID == itemInstanceID {
			return item
		}
	}
	return protocol.ItemInstanceState{}
}

func equipmentArchetype(snapshot protocol.EquipmentSnapshot, slot protocol.EquipmentSlot) string {
	for _, item := range snapshot.Slots {
		if item.Slot == slot {
			return item.ItemArchetypeID
		}
	}
	return ""
}

func equipmentInstanceID(snapshot protocol.EquipmentInstanceSnapshot, slot protocol.EquipmentSlot) string {
	return equipmentInstance(snapshot, slot).ItemInstanceID
}

func equipmentInstance(snapshot protocol.EquipmentInstanceSnapshot, slot protocol.EquipmentSlot) protocol.ItemInstanceState {
	for _, item := range snapshot.Slots {
		if item.Slot == slot {
			return item.Item
		}
	}
	return protocol.ItemInstanceState{}
}

func assertProtocolInstanceMatches(t *testing.T, got protocol.ItemInstanceState, want iteminstance.Instance) {
	t.Helper()
	if got.ItemInstanceID != string(want.ID) ||
		got.ItemArchetypeID != want.ItemArchetypeID ||
		got.EnhancementLevel != want.EnhancementLevel ||
		len(got.Affixes) != len(want.Affixes) {
		t.Fatalf("instance=%#v want=%#v", got, want)
	}
	for index, affix := range want.Affixes {
		gotAffix := got.Affixes[index]
		if gotAffix.AffixID != string(affix.ID) ||
			gotAffix.Strength != affix.Strength ||
			gotAffix.Value != affix.Value {
			t.Fatalf("instance %q affix[%d]=%#v want=%#v", got.ItemInstanceID, index, gotAffix, affix)
		}
	}
}
