package inventory

import (
	"errors"
	"testing"
)

func TestExchangeConsumesAndGrantsWithSingleRevision(t *testing.T) {
	inv := New(4)
	if err := inv.Add("item_gray_wolf_pelt", 1); err != nil { t.Fatal(err) }
	if err := inv.Add("item_minor_healing_potion", 5); err != nil { t.Fatal(err) }
	before := inv.Revision()

	if err := inv.Exchange("item_gray_wolf_pelt", 1, "item_minor_healing_potion", 1); err != nil { t.Fatal(err) }
	if got := inv.Quantity("item_gray_wolf_pelt"); got != 0 { t.Fatalf("pelt quantity = %d, want 0", got) }
	if got := inv.Quantity("item_minor_healing_potion"); got != 6 { t.Fatalf("potion quantity = %d, want 6", got) }
	if got := inv.Revision(); got != before+1 { t.Fatalf("revision = %d, want %d", got, before+1) }
}

func TestExchangeInsufficientLeavesInventoryUnchanged(t *testing.T) {
	inv := New(4)
	if err := inv.Add("item_minor_healing_potion", 5); err != nil { t.Fatal(err) }
	before := inv.Revision()

	err := inv.Exchange("item_gray_wolf_pelt", 1, "item_minor_healing_potion", 1)
	if !errors.Is(err, ErrInsufficient) { t.Fatalf("error = %v, want insufficient", err) }
	if got := inv.Quantity("item_minor_healing_potion"); got != 5 { t.Fatalf("potion quantity = %d, want 5", got) }
	if got := inv.Revision(); got != before { t.Fatalf("revision = %d, want %d", got, before) }
}

func TestExchangeCanReuseStackSlotFreedByCost(t *testing.T) {
	inv := New(1)
	if err := inv.Add("item_gray_wolf_pelt", 1); err != nil { t.Fatal(err) }
	if err := inv.Exchange("item_gray_wolf_pelt", 1, "item_minor_healing_potion", 1); err != nil { t.Fatal(err) }
	if got := inv.Quantity("item_gray_wolf_pelt"); got != 0 { t.Fatalf("pelt quantity = %d, want 0", got) }
	if got := inv.Quantity("item_minor_healing_potion"); got != 1 { t.Fatalf("potion quantity = %d, want 1", got) }
}
