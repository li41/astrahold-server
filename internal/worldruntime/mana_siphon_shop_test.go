package worldruntime

import (
	"testing"
	"time"

	"github.com/li41/astrahold-server/internal/protocol"
	"github.com/li41/astrahold-server/internal/world"
)

func TestDefaultShopTradesThreeGrayPeltsForManaSiphonStaff(t *testing.T) {
	rt, sim, s, _ := newNPCTestRuntime(t, world.Position{})
	npcID := spawnTestNPC(t, sim, world.Position{X: 2})
	inv := rt.inventories[s.CharacterIdentity.ID]
	if inv == nil {
		t.Fatal("inventory missing")
	}
	if err := inv.Add(testShopCostArchetypeID, 3); err != nil {
		t.Fatal(err)
	}
	beforeRevision := inv.Revision()

	if err := rt.EnqueueShopCommand(s.ID, 1, protocol.ClientShopCommand{
		Operation:   protocol.ShopOperationBuy,
		NPCEntityID: npcID,
		OfferID:     "trade_gray_pelts_for_mana_siphon_staff",
	}); err != nil {
		t.Fatal(err)
	}
	report := rt.Step(2, 50*time.Millisecond)
	if len(report.CommandErrors) != 0 {
		t.Fatalf("buy errors = %#v", report.CommandErrors)
	}
	if got := inv.Quantity(testShopCostArchetypeID); got != 0 {
		t.Fatalf("gray pelt quantity = %d, want 0", got)
	}
	if got := inv.Quantity(manaSiphonStaffArchetypeID); got != 1 {
		t.Fatalf("mana siphon staff quantity = %d, want 1", got)
	}
	if got := inv.Revision(); got != beforeRevision+1 {
		t.Fatalf("inventory revision = %d, want %d", got, beforeRevision+1)
	}
}
