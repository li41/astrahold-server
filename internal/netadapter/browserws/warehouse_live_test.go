package browserws

import (
	"context"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/li41/astrahold-server/internal/characteridentity"
	"github.com/li41/astrahold-server/internal/characterstate"
	"github.com/li41/astrahold-server/internal/characterstats"
	"github.com/li41/astrahold-server/internal/codec/gamev1"
	"github.com/li41/astrahold-server/internal/gameplayworld"
	"github.com/li41/astrahold-server/internal/movement"
	"github.com/li41/astrahold-server/internal/navigation"
	"github.com/li41/astrahold-server/internal/protocol"
	"github.com/li41/astrahold-server/internal/session"
	"github.com/li41/astrahold-server/internal/simulation"
	"github.com/li41/astrahold-server/internal/spatial"
	"github.com/li41/astrahold-server/internal/transport"
	"github.com/li41/astrahold-server/internal/world"
	"github.com/li41/astrahold-server/internal/worldruntime"
)

const warehouseLivePersonalItem = "item_minor_healing_potion"

type warehouseLiveFixture struct {
	ctx   context.Context
	conn  *websocket.Conn
	codec gamev1.Codec
}

func TestWarehouseBrowserWSV32Map0GMRoundTrip(t *testing.T) {
	fixture := newWarehouseLiveFixture(t, "1", nil)

	sendWarehouseCommand(t, fixture, 1, protocol.ClientWarehouseCommand{Operation: protocol.WarehouseOperationOpenGM})
	result, snapshot := readWarehouseResultAndSnapshot(t, fixture, true)
	if result.ClientActionSequence != 1 || result.Operation != protocol.WarehouseOperationOpenGM || result.Outcome != protocol.WarehouseOutcomeOpened || result.Reason != "" {
		t.Fatalf("open_gm result=%#v", result)
	}
	if len(snapshot.Items) != 2 {
		t.Fatalf("GM snapshot items=%#v", snapshot.Items)
	}
	if warehouseSnapshotQuantity(snapshot, worldruntime.WeaponEnhancementScrollItemArchetypeID) != 0 ||
		warehouseSnapshotQuantity(snapshot, worldruntime.ArmorEnhancementScrollItemArchetypeID) != 0 {
		t.Fatalf("GM snapshot must expose both unlimited enhancement scrolls: %#v", snapshot.Items)
	}

	sendWarehouseCommand(t, fixture, 2, protocol.ClientWarehouseCommand{
		Operation:       protocol.WarehouseOperationWithdrawGM,
		ItemArchetypeID: worldruntime.WeaponEnhancementScrollItemArchetypeID,
		Quantity:        2,
	})
	result, snapshot = readWarehouseResultAndSnapshot(t, fixture, true)
	if result.ClientActionSequence != 2 || result.Operation != protocol.WarehouseOperationWithdrawGM ||
		result.Outcome != protocol.WarehouseOutcomeWithdrawn || result.ItemArchetypeID != worldruntime.WeaponEnhancementScrollItemArchetypeID ||
		result.Quantity != 2 {
		t.Fatalf("withdraw_gm result=%#v", result)
	}
	if warehouseSnapshotQuantity(snapshot, worldruntime.WeaponEnhancementScrollItemArchetypeID) != 0 {
		t.Fatalf("GM infinite source was decremented: %#v", snapshot.Items)
	}
}

func TestWarehouseBrowserWSV32Map0PersonalRoundTrip(t *testing.T) {
	fixture := newWarehouseLiveFixture(t, "2", []characterstate.InventoryStack{{
		ItemArchetypeID: warehouseLivePersonalItem,
		Quantity:        5,
	}})

	sendWarehouseCommand(t, fixture, 1, protocol.ClientWarehouseCommand{Operation: protocol.WarehouseOperationOpenPersonal})
	result, snapshot := readWarehouseResultAndSnapshot(t, fixture, true)
	if result.ClientActionSequence != 1 || result.Operation != protocol.WarehouseOperationOpenPersonal || result.Outcome != protocol.WarehouseOutcomeOpened {
		t.Fatalf("open_personal result=%#v", result)
	}
	if len(snapshot.Items) != 0 {
		t.Fatalf("initial personal warehouse snapshot=%#v", snapshot.Items)
	}

	sendWarehouseCommand(t, fixture, 2, protocol.ClientWarehouseCommand{
		Operation:       protocol.WarehouseOperationDepositPersonal,
		ItemArchetypeID: warehouseLivePersonalItem,
		Quantity:        2,
	})
	result, snapshot = readWarehouseResultAndSnapshot(t, fixture, true)
	if result.ClientActionSequence != 2 || result.Operation != protocol.WarehouseOperationDepositPersonal || result.Outcome != protocol.WarehouseOutcomeDeposited {
		t.Fatalf("deposit_personal result=%#v", result)
	}
	if got := warehouseSnapshotQuantity(snapshot, warehouseLivePersonalItem); got != 2 {
		t.Fatalf("personal warehouse after deposit=%d want=2 snapshot=%#v", got, snapshot.Items)
	}

	sendWarehouseCommand(t, fixture, 3, protocol.ClientWarehouseCommand{
		Operation:       protocol.WarehouseOperationWithdrawPersonal,
		ItemArchetypeID: warehouseLivePersonalItem,
		Quantity:        1,
	})
	result, snapshot = readWarehouseResultAndSnapshot(t, fixture, true)
	if result.ClientActionSequence != 3 || result.Operation != protocol.WarehouseOperationWithdrawPersonal || result.Outcome != protocol.WarehouseOutcomeWithdrawn {
		t.Fatalf("withdraw_personal result=%#v", result)
	}
	if got := warehouseSnapshotQuantity(snapshot, warehouseLivePersonalItem); got != 1 {
		t.Fatalf("personal warehouse after withdraw=%d want=1 snapshot=%#v", got, snapshot.Items)
	}
}

func TestWarehouseBrowserWSV32Map0RejectsNonGMSubject(t *testing.T) {
	fixture := newWarehouseLiveFixture(t, "2", nil)

	sendWarehouseCommand(t, fixture, 1, protocol.ClientWarehouseCommand{Operation: protocol.WarehouseOperationOpenGM})
	result, _ := readWarehouseResultAndSnapshot(t, fixture, false)
	if result.ClientActionSequence != 1 || result.Operation != protocol.WarehouseOperationOpenGM ||
		result.Outcome != protocol.WarehouseOutcomeRejected || result.Reason != protocol.WarehouseRejectionNotAuthorized {
		t.Fatalf("non-GM open_gm result=%#v", result)
	}
}

func newWarehouseLiveFixture(t *testing.T, authenticationSubject string, stacks []characterstate.InventoryStack) warehouseLiveFixture {
	t.Helper()

	loaded, err := gameplayworld.LoadFile("../../../worlds/gm-room/gameplay.json")
	if err != nil {
		t.Fatalf("load GM room gameplay: %v", err)
	}
	navigator, err := navigation.NewGameplayNavigator(loaded.Definition)
	if err != nil {
		t.Fatalf("build GM room navigator: %v", err)
	}
	keeper, err := navigator.BlockerDefinition(gameplayworld.GMRoomStockKeeperBlockerID)
	if err != nil {
		t.Fatalf("resolve stock-keeper blocker: %v", err)
	}
	position := world.Position{
		X:     keeper.Bounds.MinX - 2,
		Y:     0,
		Z:     (keeper.Bounds.MinZ + keeper.Bounds.MaxZ) * 0.5,
		Layer: keeper.Layer,
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
	inventoryState, err := characterstate.NewInventoryStateWithEquipment(stacks, "", "")
	if err != nil {
		t.Fatalf("build warehouse E2E inventory: %v", err)
	}
	identity, err := characteridentity.NewTrusted("e2e-browserws-warehouse-" + warehouseSubjectLabel(authenticationSubject))
	if err != nil {
		t.Fatalf("build trusted warehouse identity: %v", err)
	}

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
		t.Fatalf("build warehouse world loop: %v", err)
	}
	loopCtx, loopCancel := context.WithCancel(context.Background())
	t.Cleanup(loopCancel)
	loopDone := make(chan error, 1)
	go func() {
		loopDone <- loop.Run(loopCtx)
	}()
	t.Cleanup(func() {
		loopCancel()
		select {
		case err := <-loopDone:
			if err != nil {
				t.Errorf("warehouse world loop: %v", err)
			}
		case <-time.After(time.Second):
			t.Errorf("warehouse world loop did not stop")
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
			Identity:              identity,
			AuthenticationSubject: authenticationSubject,
			Restore: worldruntime.CharacterRestore{
				SchemaVersion: characterstate.SchemaVersion,
				CharacterID:   identity.ID,
				Revision:      1,
				MapID:         string(gameplayworld.MapIDGMRoom),
				World:         worldIdentity,
				HP:            1000,
				MaxHP:         1000,
				MP:            100,
				MaxMP:         100,
				Transform:     world.Transform{Position: position},
				Inventory:     inventoryState,
				Warehouse:     characterstate.EmptyWarehouseState(),
				PrimaryStats:  characterstats.DefaultPrimary(),
			},
		}, nil
	}

	httpServer := httptest.NewServer(NewHandler(browserConfig, runtime, gamev1.Codec{}))
	t.Cleanup(httpServer.Close)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)
	conn, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(httpServer.URL, "http"), nil)
	if err != nil {
		t.Fatalf("dial warehouse BrowserWS: %v", err)
	}
	t.Cleanup(func() { _ = conn.CloseNow() })

	fixture := warehouseLiveFixture{ctx: ctx, conn: conn, codec: gamev1.Codec{}}
	welcome := readEnvelope(t, ctx, conn, fixture.codec)
	if _, ok := welcome.Message.(protocol.SessionWelcome); !ok {
		t.Fatalf("first BrowserWS message=%T want SessionWelcome", welcome.Message)
	}
	return fixture
}

func sendWarehouseCommand(t *testing.T, fixture warehouseLiveFixture, sequence uint32, command protocol.ClientWarehouseCommand) {
	t.Helper()
	frame, err := transport.EncodeEnvelope(protocol.Envelope{
		Delivery: protocol.DeliveryReliableOrdered,
		Sequence: sequence,
		Message:  command,
	}, fixture.codec)
	if err != nil {
		t.Fatalf("encode warehouse command: %v", err)
	}
	if err := fixture.conn.Write(fixture.ctx, websocket.MessageBinary, frame); err != nil {
		t.Fatalf("write warehouse command: %v", err)
	}
}

func readWarehouseResultAndSnapshot(t *testing.T, fixture warehouseLiveFixture, wantSnapshot bool) (protocol.WarehouseResult, protocol.WarehouseSnapshot) {
	t.Helper()
	var (
		result      protocol.WarehouseResult
		snapshot    protocol.WarehouseSnapshot
		haveResult  bool
		haveSnapshot bool
	)
	for i := 0; i < 128; i++ {
		envelope := readEnvelope(t, fixture.ctx, fixture.conn, fixture.codec)
		switch message := envelope.Message.(type) {
		case protocol.WarehouseResult:
			result = message
			haveResult = true
		case protocol.WarehouseSnapshot:
			snapshot = message
			haveSnapshot = true
		}
		if haveResult && (!wantSnapshot || haveSnapshot) {
			return result, snapshot
		}
	}
	t.Fatalf("warehouse response not observed result=%v snapshot=%v", haveResult, haveSnapshot)
	return protocol.WarehouseResult{}, protocol.WarehouseSnapshot{}
}

func warehouseSnapshotQuantity(snapshot protocol.WarehouseSnapshot, itemArchetypeID string) uint32 {
	for _, item := range snapshot.Items {
		if item.ItemArchetypeID == itemArchetypeID {
			return item.Quantity
		}
	}
	return 0
}

func warehouseSubjectLabel(subject string) string {
	if subject == "" {
		return "anonymous"
	}
	return fmt.Sprintf("subject-%s", subject)
}
