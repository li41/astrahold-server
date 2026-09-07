package worldruntime

import (
	"testing"

	"github.com/li41/astrahold-server/internal/world"
)

func TestMonsterLootTicketWithSecretIsStableAndBounded(t *testing.T) {
	var secret monsterLootRollSecret
	secret[0] = 0x41
	secret[31] = 0x9d

	const total = uint64(100)
	first := monsterLootTicketWithSecret(secret, world.EntityID(77), 3, 2, total)
	second := monsterLootTicketWithSecret(secret, world.EntityID(77), 3, 2, total)
	if first != second {
		t.Fatalf("same Server secret/input produced different tickets: first=%d second=%d", first, second)
	}
	if first >= total {
		t.Fatalf("ticket=%d outside [0,%d)", first, total)
	}
}

func TestMonsterLootTicketDependsOnServerPrivateSecret(t *testing.T) {
	var firstSecret monsterLootRollSecret
	var secondSecret monsterLootRollSecret
	firstSecret[0] = 1
	secondSecret[0] = 2

	// A client knows EntityID/incarnation/drop index, but not this secret. Verify changing only the
	// Server-private key changes at least one ticket across a deterministic sample of public inputs.
	changed := false
	for monsterID := world.EntityID(1); monsterID <= 64; monsterID++ {
		first := monsterLootTicketWithSecret(firstSecret, monsterID, 4, 0, 1_000_003)
		second := monsterLootTicketWithSecret(secondSecret, monsterID, 4, 0, 1_000_003)
		if first != second {
			changed = true
			break
		}
	}
	if !changed {
		t.Fatal("loot tickets did not depend on Server-private secret")
	}
}

func TestMonsterLootTicketZeroTotalIsSafe(t *testing.T) {
	var secret monsterLootRollSecret
	if got := monsterLootTicketWithSecret(secret, world.EntityID(1), 1, 0, 0); got != 0 {
		t.Fatalf("zero-total ticket=%d want 0", got)
	}
}
