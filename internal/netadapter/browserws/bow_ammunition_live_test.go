package browserws

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/li41/astrahold-server/internal/ammunition"
	"github.com/li41/astrahold-server/internal/characteridentity"
	"github.com/li41/astrahold-server/internal/characterstate"
	"github.com/li41/astrahold-server/internal/characterstats"
	"github.com/li41/astrahold-server/internal/codec/gamev1"
	"github.com/li41/astrahold-server/internal/combat"
	"github.com/li41/astrahold-server/internal/equipmentcatalog"
	"github.com/li41/astrahold-server/internal/gameplayworld"
	"github.com/li41/astrahold-server/internal/movement"
	"github.com/li41/astrahold-server/internal/navigation"
	"github.com/li41/astrahold-server/internal/protocol"
	"github.com/li41/astrahold-server/internal/session"
	"github.com/li41/astrahold-server/internal/simulation"
	"github.com/li41/astrahold-server/internal/spatial"
	"github.com/li41/astrahold-server/internal/world"
	"github.com/li41/astrahold-server/internal/worldruntime"
)

const bowLiveTargetID world.EntityID = 9201

type bowAmmunitionLiveFixture struct {
	ctx            context.Context
	conn           *websocket.Conn
	codec          gamev1.Codec
	playerEntityID world.EntityID
}

func TestBowAmmunitionBrowserWSV32LiveAuthority(t *testing.T) {
	warehouseState, err := characterstate.CanonicalWarehouseState(characterstate.WarehouseState{
		Initialized: true,
		Items: []characterstate.WarehouseStack{
			{ItemArchetypeID: ammunition.ItemWoodArrow, Quantity: 1},
			{ItemArchetypeID: ammunition.ItemSilverArrow, Quantity: 1},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	fixture := newBowAmmunitionLiveFixture(t, "item_hunter_shortbow", nil, warehouseState)

	writeBrowserIntent(t, fixture.ctx, fixture.conn, fixture.codec, 1, protocol.ClientUseAction{
		ActionID: "basic-attack", TargetKind: protocol.ActionTargetEntity, TargetID: "9201",
	})
	rejected := readBowActionRejection(t, fixture, 1)
	if rejected.Reason != protocol.ActionRejectionInsufficientResource || rejected.CooldownReadyTick != 0 {
		t.Fatalf("no-arrow rejection=%#v", rejected)
	}

	sendWarehouseCommand(t, warehouseFixtureView(fixture), 2, protocol.ClientWarehouseCommand{
		Operation: protocol.WarehouseOperationWithdrawPersonal, ItemArchetypeID: ammunition.ItemWoodArrow, Quantity: 1,
	})
	readBowWarehouseMutation(t, fixture, 2, ammunition.ItemWoodArrow, 1, ammunition.ItemSilverArrow, 0)

	sendWarehouseCommand(t, warehouseFixtureView(fixture), 3, protocol.ClientWarehouseCommand{
		Operation: protocol.WarehouseOperationWithdrawPersonal, ItemArchetypeID: ammunition.ItemSilverArrow, Quantity: 1,
	})
	readBowWarehouseMutation(t, fixture, 3, ammunition.ItemWoodArrow, 1, ammunition.ItemSilverArrow, 1)

	// If the rejected no-ammo attempt had committed the authored 1.2s bow cooldown, this immediate
	// shot would be rejected. A CombatEvent proves the rejection consumed neither ammo nor cooldown.
	writeBrowserIntent(t, fixture.ctx, fixture.conn, fixture.codec, 4, protocol.ClientUseAction{
		ActionID: "basic-attack", TargetKind: protocol.ActionTargetEntity, TargetID: "9201",
	})
	first := readBowCombatAndInventory(t, fixture, 4, ammunition.ItemWoodArrow, 0, ammunition.ItemSilverArrow, 1)
	if first.Result != protocol.CombatEventHit && first.Result != protocol.CombatEventMiss {
		t.Fatalf("wood-arrow combat event=%#v", first)
	}

	// Bow cadence is 1.2s. Let the authoritative cooldown expire, then the remaining silver arrow
	// must be consumed by the next established shot.
	time.Sleep(1300 * time.Millisecond)
	writeBrowserIntent(t, fixture.ctx, fixture.conn, fixture.codec, 5, protocol.ClientUseAction{
		ActionID: "basic-attack", TargetKind: protocol.ActionTargetEntity, TargetID: "9201",
	})
	second := readBowCombatAndInventory(t, fixture, 5, ammunition.ItemWoodArrow, 0, ammunition.ItemSilverArrow, 0)
	if second.Result != protocol.CombatEventHit && second.Result != protocol.CombatEventMiss {
		t.Fatalf("silver-arrow combat event=%#v", second)
	}
	if second.Result == protocol.CombatEventHit {
		// Base shortbow+arrow is 8..11. Agility 100 adds 30, then silver-vs-undead applies x1.20
		// before the optional critical. These ranges are disjoint from the no-silver outcomes.
		normalSilver := second.Damage >= 45 && second.Damage <= 49
		criticalSilver := second.Damage >= 68 && second.Damage <= 74
		if !normalSilver && !criticalSilver {
			t.Fatalf("silver-arrow undead damage=%d outside authoritative silver ranges", second.Damage)
		}
	}
}

func TestCrossbowBrowserWSV32DoesNotConsumeBowArrows(t *testing.T) {
	warehouseState := characterstate.EmptyWarehouseState()
	fixture := newBowAmmunitionLiveFixture(t, "item_hunter_light_crossbow", []characterstate.InventoryStack{
		{ItemArchetypeID: ammunition.ItemWoodArrow, Quantity: 2},
	}, warehouseState)

	readBowInventory(t, fixture, ammunition.ItemWoodArrow, 2, ammunition.ItemSilverArrow, 0)

	writeBrowserIntent(t, fixture.ctx, fixture.conn, fixture.codec, 1, protocol.ClientUseAction{
		ActionID: "basic-attack", TargetKind: protocol.ActionTargetEntity, TargetID: "9201",
	})
	event := readBowCombatEvent(t, fixture, 1)
	if event.Result != protocol.CombatEventHit && event.Result != protocol.CombatEventMiss {
		t.Fatalf("crossbow combat event=%#v", event)
	}

	// Deposit both arrows after the shot. This succeeds only if the crossbow did not consume one.
	sendWarehouseCommand(t, warehouseFixtureView(fixture), 2, protocol.ClientWarehouseCommand{
		Operation: protocol.WarehouseOperationDepositPersonal, ItemArchetypeID: ammunition.ItemWoodArrow, Quantity: 2,
	})
	result, snapshot := readWarehouseResultAndSnapshot(t, warehouseFixtureView(fixture), true)
	if result.ClientActionSequence != 2 || result.Operation != protocol.WarehouseOperationDepositPersonal ||
		result.Outcome != protocol.WarehouseOutcomeDeposited {
		t.Fatalf("crossbow post-shot deposit result=%#v", result)
	}
	if got := warehouseSnapshotQuantity(snapshot, ammunition.ItemWoodArrow); got != 2 {
		t.Fatalf("crossbow consumed bow ammo before deposit: warehouse=%d want=2 snapshot=%#v", got, snapshot.Items)
	}
}

func newBowAmmunitionLiveFixture(t *testing.T, weaponID string, stacks []characterstate.InventoryStack, warehouseState characterstate.WarehouseState) bowAmmunitionLiveFixture {
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
		WorldID: loaded.Definition.WorldID, Revision: loaded.Definition.Revision, GameplaySHA256: loaded.SHA256,
	}
	stateWorld := characterstate.WorldRef{
		MapID: string(gameplayworld.MapIDGMRoom), WorldID: worldIdentity.WorldID,
		Revision: worldIdentity.Revision, GameplaySHA256: worldIdentity.GameplaySHA256,
	}
	inventoryState, err := characterstate.NewInventoryStateWithEquipment(stacks, weaponID, "")
	if err != nil {
		t.Fatalf("build bow live inventory: %v", err)
	}
	identity, err := characteridentity.NewTrusted("e2e-browserws-ammunition-" + strings.TrimPrefix(weaponID, "item_"))
	if err != nil {
		t.Fatal(err)
	}
	primary := characterstats.DefaultPrimary()
	primary.Agility = 100

	sim := simulation.New(spatial.NewGrid(16), movement.NewService(navigator, 0.1))
	combatService, err := combat.NewService([]combat.ActionDefinition{{
		ID: "basic-attack", Targets: []combat.TargetKind{combat.TargetEntity}, Range: 4.5,
		BaseDamage: 100, DamageType: combat.DamagePhysical, CooldownSeconds: 0.5,
	}})
	if err != nil {
		t.Fatalf("build combat service: %v", err)
	}
	cfg := worldruntime.DefaultConfig()
	cfg.SnapshotEveryTicks = 1
	runtime := worldruntime.New(
		sim,
		cfg,
		worldruntime.WithDynamicWorld(navigator),
		worldruntime.WithCombatService(combatService),
		worldruntime.WithCharacterStateOutbox(nil, stateWorld),
	)
	if err := runtime.EnqueueSpawnEntity(worldruntime.SpawnEntityRequest{
		Entity: world.EntityState{
			ID: bowLiveTargetID,
			Kind: world.EntityMonster,
			ArchetypeID: "monster-browserws-ammunition-undead",
			Classification: world.EntityClassificationUndead,
			Transform: world.Transform{Position: world.Position{X: -3, Y: 0, Z: 0, Layer: 0}},
		},
		Speed: 4, Radius: 0.35, MaxStepHeight: 0.5,
		HP: 10000, MaxHP: 10000, BodySize: equipmentcatalog.BodySizeSmall,
	}); err != nil {
		t.Fatalf("enqueue bow target: %v", err)
	}

	loop, err := worldruntime.NewLoop(runtime, 40)
	if err != nil {
		t.Fatalf("build bow world loop: %v", err)
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
				t.Errorf("bow world loop: %v", err)
			}
		case <-time.After(time.Second):
			t.Errorf("bow world loop did not stop")
		}
	})

	browserConfig := DefaultConfig()
	browserConfig.TickRateHz = 40
	browserConfig.SnapshotRateHz = 40
	browserConfig.WorldIdentity = worldIdentity
	browserConfig.PlayerFactory = func(_ session.ID, entityID world.EntityID) PlayerSpec {
		return PlayerSpec{
			Entity: world.EntityState{
				ID: entityID, Kind: world.EntityPlayer,
				Transform: world.Transform{Position: world.Position{X: 0, Y: 0, Z: 0, Layer: 0}},
			},
			Speed: 6, Radius: loaded.Definition.Agent.Radius,
			MaxStepHeight: loaded.Definition.Agent.MaxStepHeight, AOIRadius: 64,
		}
	}
	browserConfig.TrustedE2EBootstrapFactory = func(_ session.ID, _ world.EntityID) (TrustedE2EBootstrap, error) {
		return TrustedE2EBootstrap{
			Identity: identity,
			Restore: worldruntime.CharacterRestore{
				SchemaVersion: characterstate.SchemaVersion,
				CharacterID: identity.ID,
				Revision: 1,
				MapID: gameplayworld.MapIDGMRoom,
				World: worldIdentity,
				HP: 1000, MaxHP: 1000, MP: 100, MaxMP: 100,
				Transform: world.Transform{Position: world.Position{X: 0, Y: 0, Z: 0, Layer: 0}},
				Inventory: inventoryState,
				Warehouse: warehouseState,
				PrimaryStats: primary,
			},
		}, nil
	}

	httpServer := httptest.NewServer(NewHandler(browserConfig, runtime, gamev1.Codec{}))
	t.Cleanup(httpServer.Close)
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	t.Cleanup(cancel)
	conn, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(httpServer.URL, "http"), nil)
	if err != nil {
		t.Fatalf("dial bow BrowserWS: %v", err)
	}
	t.Cleanup(func() { _ = conn.CloseNow() })

	fixture := bowAmmunitionLiveFixture{ctx: ctx, conn: conn, codec: gamev1.Codec{}}
	welcome := readEnvelope(t, ctx, conn, fixture.codec)
	message, ok := welcome.Message.(protocol.SessionWelcome)
	if !ok {
		t.Fatalf("first BrowserWS message=%T want SessionWelcome", welcome.Message)
	}
	fixture.playerEntityID = message.EntityID
	return fixture
}

func warehouseFixtureView(fixture bowAmmunitionLiveFixture) warehouseLiveFixture {
	return warehouseLiveFixture{ctx: fixture.ctx, conn: fixture.conn, codec: fixture.codec}
}

func readBowActionRejection(t *testing.T, fixture bowAmmunitionLiveFixture, sequence uint32) protocol.ActionRejected {
	t.Helper()
	for i := 0; i < 192; i++ {
		envelope := readEnvelope(t, fixture.ctx, fixture.conn, fixture.codec)
		switch message := envelope.Message.(type) {
		case protocol.ActionRejected:
			if message.ClientActionSequence == sequence {
				return message
			}
		case protocol.CombatEvent:
			t.Fatalf("unexpected CombatEvent while waiting for action rejection sequence=%d: %#v", sequence, message)
		}
	}
	t.Fatalf("action rejection sequence=%d not observed", sequence)
	return protocol.ActionRejected{}
}

func readBowWarehouseMutation(t *testing.T, fixture bowAmmunitionLiveFixture, sequence uint32, firstID string, firstQuantity uint32, secondID string, secondQuantity uint32) {
	t.Helper()
	var haveResult, haveInventory bool
	for i := 0; i < 256; i++ {
		envelope := readEnvelope(t, fixture.ctx, fixture.conn, fixture.codec)
		switch message := envelope.Message.(type) {
		case protocol.WarehouseResult:
			if message.ClientActionSequence == sequence {
				if message.Outcome != protocol.WarehouseOutcomeWithdrawn {
					t.Fatalf("warehouse sequence=%d result=%#v", sequence, message)
				}
				haveResult = true
			}
		case protocol.InventorySnapshot:
			if inventorySnapshotQuantity(message, firstID) == firstQuantity &&
				inventorySnapshotQuantity(message, secondID) == secondQuantity {
				haveInventory = true
			}
		}
		if haveResult && haveInventory {
			return
		}
	}
	t.Fatalf("warehouse mutation sequence=%d incomplete result=%v inventory=%v", sequence, haveResult, haveInventory)
}

func readBowCombatAndInventory(t *testing.T, fixture bowAmmunitionLiveFixture, sequence uint32, firstID string, firstQuantity uint32, secondID string, secondQuantity uint32) protocol.CombatEvent {
	t.Helper()
	var event protocol.CombatEvent
	var haveEvent, haveInventory bool
	for i := 0; i < 256; i++ {
		envelope := readEnvelope(t, fixture.ctx, fixture.conn, fixture.codec)
		switch message := envelope.Message.(type) {
		case protocol.ActionRejected:
			if message.ClientActionSequence == sequence {
				t.Fatalf("action sequence=%d rejected: %#v", sequence, message)
			}
		case protocol.CombatEvent:
			if message.ActorEntityID == fixture.playerEntityID && message.TargetEntityID == bowLiveTargetID && message.ActionID == "basic-attack" {
				event = message
				haveEvent = true
			}
		case protocol.InventorySnapshot:
			if inventorySnapshotQuantity(message, firstID) == firstQuantity &&
				inventorySnapshotQuantity(message, secondID) == secondQuantity {
				haveInventory = true
			}
		}
		if haveEvent && haveInventory {
			return event
		}
	}
	t.Fatalf("combat/inventory sequence=%d incomplete event=%v inventory=%v", sequence, haveEvent, haveInventory)
	return protocol.CombatEvent{}
}

func readBowCombatEvent(t *testing.T, fixture bowAmmunitionLiveFixture, sequence uint32) protocol.CombatEvent {
	t.Helper()
	for i := 0; i < 256; i++ {
		envelope := readEnvelope(t, fixture.ctx, fixture.conn, fixture.codec)
		switch message := envelope.Message.(type) {
		case protocol.ActionRejected:
			if message.ClientActionSequence == sequence {
				t.Fatalf("action sequence=%d rejected: %#v", sequence, message)
			}
		case protocol.CombatEvent:
			if message.ActorEntityID == fixture.playerEntityID && message.TargetEntityID == bowLiveTargetID && message.ActionID == "basic-attack" {
				return message
			}
		}
	}
	t.Fatalf("combat event sequence=%d not observed", sequence)
	return protocol.CombatEvent{}
}

func readBowInventory(t *testing.T, fixture bowAmmunitionLiveFixture, firstID string, firstQuantity uint32, secondID string, secondQuantity uint32) protocol.InventorySnapshot {
	t.Helper()
	for i := 0; i < 256; i++ {
		envelope := readEnvelope(t, fixture.ctx, fixture.conn, fixture.codec)
		if message, ok := envelope.Message.(protocol.InventorySnapshot); ok {
			if inventorySnapshotQuantity(message, firstID) == firstQuantity &&
				inventorySnapshotQuantity(message, secondID) == secondQuantity {
				return message
			}
		}
	}
	t.Fatalf("inventory snapshot not observed %s=%d %s=%d", firstID, firstQuantity, secondID, secondQuantity)
	return protocol.InventorySnapshot{}
}
