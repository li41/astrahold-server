package worldruntime

import (
	"errors"
	"testing"
	"time"

	"github.com/li41/astrahold-server/internal/character"
	"github.com/li41/astrahold-server/internal/protocol"
	"github.com/li41/astrahold-server/internal/session"
	"github.com/li41/astrahold-server/internal/world"
)

func TestHealingPotionCooldownRejectsSpamAndAllowsExactReadyTick(t *testing.T) {
	runtime, _, s := newItemDropTestRuntime(t, world.Position{})
	connection := s.Connection().(*session.QueueConnection)
	drainReliable(connection)
	if _, err := runtime.characters.ApplyDamage(s.EntityID, 400); err != nil {
		t.Fatal(err)
	}
	inv := runtime.inventories[s.CharacterIdentity.ID]

	if err := runtime.EnqueueUseItem(s.ID, 1, protocol.ClientUseItem{ItemArchetypeID: "item_minor_healing_potion"}); err != nil {
		t.Fatal(err)
	}
	first := runtime.Step(2, 50*time.Millisecond)
	if len(first.CommandErrors) != 0 {
		t.Fatalf("first errors=%#v", first.CommandErrors)
	}
	firstResult := requireSingleItemUseResult(t, connection)
	if firstResult.Outcome != protocol.ItemUseOutcomeUsed || firstResult.AppliedAmount != 250 || firstResult.CooldownReadyTick != 42 {
		t.Fatalf("first result=%#v", firstResult)
	}
	if got := inv.Quantity("item_minor_healing_potion"); got != 4 {
		t.Fatalf("quantity after first use=%d", got)
	}

	if _, err := runtime.characters.ApplyDamage(s.EntityID, 300); err != nil {
		t.Fatal(err)
	}
	if err := runtime.EnqueueUseItem(s.ID, 2, protocol.ClientUseItem{ItemArchetypeID: "item_minor_healing_potion"}); err != nil {
		t.Fatal(err)
	}
	blocked := runtime.Step(3, 50*time.Millisecond)
	if len(blocked.CommandErrors) != 1 || !errors.Is(blocked.CommandErrors[0].Err, ErrItemUseCooldown) {
		t.Fatalf("blocked errors=%#v", blocked.CommandErrors)
	}
	blockedResult := requireSingleItemUseResult(t, connection)
	if blockedResult.Outcome != protocol.ItemUseOutcomeRejected || blockedResult.Reason != protocol.ItemUseRejectionCooldown || blockedResult.CooldownReadyTick != 42 {
		t.Fatalf("blocked result=%#v", blockedResult)
	}
	state, _ := runtime.characters.State(s.EntityID)
	if state.HP != 550 || inv.Quantity("item_minor_healing_potion") != 4 {
		t.Fatalf("blocked mutation hp=%d inventory=%#v", state.HP, inv.Snapshot())
	}

	if err := runtime.EnqueueUseItem(s.ID, 3, protocol.ClientUseItem{ItemArchetypeID: "item_minor_healing_potion"}); err != nil {
		t.Fatal(err)
	}
	ready := runtime.Step(42, 50*time.Millisecond)
	if len(ready.CommandErrors) != 0 {
		t.Fatalf("ready errors=%#v", ready.CommandErrors)
	}
	readyResult := requireSingleItemUseResult(t, connection)
	if readyResult.Outcome != protocol.ItemUseOutcomeUsed || readyResult.CooldownReadyTick != 82 {
		t.Fatalf("ready result=%#v", readyResult)
	}
	state, _ = runtime.characters.State(s.EntityID)
	if state.HP != 800 || inv.Quantity("item_minor_healing_potion") != 3 {
		t.Fatalf("ready mutation hp=%d inventory=%#v", state.HP, inv.Snapshot())
	}
}

func TestHealingAndManaPotionsShareCooldownGroup(t *testing.T) {
	runtime, _, s := newItemDropTestRuntime(t, world.Position{})
	connection := s.Connection().(*session.QueueConnection)
	drainReliable(connection)
	if _, err := runtime.characters.ApplyDamage(s.EntityID, 300); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.characters.SpendMP(s.EntityID, 50); err != nil {
		t.Fatal(err)
	}

	if err := runtime.EnqueueUseItem(s.ID, 1, protocol.ClientUseItem{ItemArchetypeID: "item_minor_healing_potion"}); err != nil {
		t.Fatal(err)
	}
	if report := runtime.Step(2, 50*time.Millisecond); len(report.CommandErrors) != 0 {
		t.Fatalf("heal errors=%#v", report.CommandErrors)
	}
	healResult := requireSingleItemUseResult(t, connection)
	if healResult.CooldownReadyTick != 42 {
		t.Fatalf("heal result=%#v", healResult)
	}

	if err := runtime.EnqueueUseItem(s.ID, 2, protocol.ClientUseItem{ItemArchetypeID: "item_minor_mana_potion"}); err != nil {
		t.Fatal(err)
	}
	blocked := runtime.Step(3, 50*time.Millisecond)
	if len(blocked.CommandErrors) != 1 || !errors.Is(blocked.CommandErrors[0].Err, ErrItemUseCooldown) {
		t.Fatalf("mana errors=%#v", blocked.CommandErrors)
	}
	manaResult := requireSingleItemUseResult(t, connection)
	if manaResult.ItemArchetypeID != "item_minor_mana_potion" || manaResult.Reason != protocol.ItemUseRejectionCooldown || manaResult.CooldownReadyTick != 42 {
		t.Fatalf("mana result=%#v", manaResult)
	}
	state, _ := runtime.characters.State(s.EntityID)
	if state.MP != 50 || runtime.inventories[s.CharacterIdentity.ID].Quantity("item_minor_mana_potion") != 3 {
		t.Fatalf("mana cooldown mutated state=%#v inventory=%#v", state, runtime.inventories[s.CharacterIdentity.ID].Snapshot())
	}
}

func TestFullResourceRejectionDoesNotStartPotionCooldown(t *testing.T) {
	runtime, _, s := newItemDropTestRuntime(t, world.Position{})
	connection := s.Connection().(*session.QueueConnection)
	drainReliable(connection)

	if err := runtime.EnqueueUseItem(s.ID, 1, protocol.ClientUseItem{ItemArchetypeID: "item_minor_healing_potion"}); err != nil {
		t.Fatal(err)
	}
	full := runtime.Step(2, 50*time.Millisecond)
	if len(full.CommandErrors) != 1 || !errors.Is(full.CommandErrors[0].Err, character.ErrResourceFull) {
		t.Fatalf("full errors=%#v", full.CommandErrors)
	}
	fullResult := requireSingleItemUseResult(t, connection)
	if fullResult.Reason != protocol.ItemUseRejectionResourceFull || fullResult.CooldownReadyTick != 0 {
		t.Fatalf("full result=%#v", fullResult)
	}
	if len(runtime.itemUseCooldownReadyTick) != 0 {
		t.Fatalf("full-resource rejection started cooldown=%#v", runtime.itemUseCooldownReadyTick)
	}

	if _, err := runtime.characters.ApplyDamage(s.EntityID, 500); err != nil {
		t.Fatal(err)
	}
	if err := runtime.EnqueueUseItem(s.ID, 2, protocol.ClientUseItem{ItemArchetypeID: "item_minor_healing_potion"}); err != nil {
		t.Fatal(err)
	}
	allowed := runtime.Step(3, 50*time.Millisecond)
	if len(allowed.CommandErrors) != 0 {
		t.Fatalf("allowed errors=%#v", allowed.CommandErrors)
	}
	allowedResult := requireSingleItemUseResult(t, connection)
	if allowedResult.Outcome != protocol.ItemUseOutcomeUsed || allowedResult.CooldownReadyTick != 43 {
		t.Fatalf("allowed result=%#v", allowedResult)
	}
}

func TestPotionCooldownSurvivesSameRuntimeReconnect(t *testing.T) {
	runtime, _, s := newItemDropTestRuntime(t, world.Position{})
	firstConnection := s.Connection().(*session.QueueConnection)
	drainReliable(firstConnection)
	if _, err := runtime.characters.ApplyDamage(s.EntityID, 400); err != nil {
		t.Fatal(err)
	}
	if err := runtime.EnqueueUseItem(s.ID, 1, protocol.ClientUseItem{ItemArchetypeID: "item_minor_healing_potion"}); err != nil {
		t.Fatal(err)
	}
	if report := runtime.Step(2, 50*time.Millisecond); len(report.CommandErrors) != 0 {
		t.Fatalf("first use errors=%#v", report.CommandErrors)
	}
	firstResult := requireSingleItemUseResult(t, firstConnection)
	if firstResult.CooldownReadyTick != 42 {
		t.Fatalf("first result=%#v", firstResult)
	}

	if err := runtime.EnqueueLeave(s.ID); err != nil {
		t.Fatal(err)
	}
	if report := runtime.Step(3, 50*time.Millisecond); len(report.CommandErrors) != 0 {
		t.Fatalf("leave errors=%#v", report.CommandErrors)
	}

	secondConnection := session.NewQueueConnection(32, 8)
	secondSession, err := session.NewWithCharacterIdentity(2, 11, s.CharacterIdentity, 32, secondConnection)
	if err != nil {
		t.Fatal(err)
	}
	if err := runtime.EnqueueJoin(JoinRequest{
		Session: secondSession,
		Entity:  world.EntityState{ID: 11, Kind: world.EntityPlayer},
		Speed: 6, Radius: 0.35, MaxStepHeight: 0.5,
	}); err != nil {
		t.Fatal(err)
	}
	if report := runtime.Step(4, 50*time.Millisecond); len(report.CommandErrors) != 0 {
		t.Fatalf("rejoin errors=%#v", report.CommandErrors)
	}
	drainReliable(secondConnection)
	if _, err := runtime.characters.ApplyDamage(secondSession.EntityID, 100); err != nil {
		t.Fatal(err)
	}
	if err := runtime.EnqueueUseItem(secondSession.ID, 1, protocol.ClientUseItem{ItemArchetypeID: "item_minor_healing_potion"}); err != nil {
		t.Fatal(err)
	}
	blocked := runtime.Step(5, 50*time.Millisecond)
	if len(blocked.CommandErrors) != 1 || !errors.Is(blocked.CommandErrors[0].Err, ErrItemUseCooldown) {
		t.Fatalf("reconnect errors=%#v", blocked.CommandErrors)
	}
	result := requireSingleItemUseResult(t, secondConnection)
	if result.Reason != protocol.ItemUseRejectionCooldown || result.CooldownReadyTick != 42 {
		t.Fatalf("reconnect result=%#v", result)
	}
}

func TestExpiredPotionCooldownIsPruned(t *testing.T) {
	runtime, _, s := newItemDropTestRuntime(t, world.Position{})
	drainReliable(s.Connection().(*session.QueueConnection))
	if _, err := runtime.characters.ApplyDamage(s.EntityID, 400); err != nil {
		t.Fatal(err)
	}
	if err := runtime.EnqueueUseItem(s.ID, 1, protocol.ClientUseItem{ItemArchetypeID: "item_minor_healing_potion"}); err != nil {
		t.Fatal(err)
	}
	if report := runtime.Step(2, 50*time.Millisecond); len(report.CommandErrors) != 0 {
		t.Fatalf("use errors=%#v", report.CommandErrors)
	}
	if len(runtime.itemUseCooldownReadyTick) != 1 {
		t.Fatalf("cooldowns=%#v", runtime.itemUseCooldownReadyTick)
	}
	runtime.Step(42, 50*time.Millisecond)
	if len(runtime.itemUseCooldownReadyTick) != 0 {
		t.Fatalf("expired cooldowns=%#v", runtime.itemUseCooldownReadyTick)
	}
}

func drainReliable(connection *session.QueueConnection) []protocol.Envelope {
	var envelopes []protocol.Envelope
	for {
		select {
		case envelope := <-connection.Reliable():
			envelopes = append(envelopes, envelope)
		default:
			return envelopes
		}
	}
}

func requireSingleItemUseResult(t *testing.T, connection *session.QueueConnection) protocol.ItemUseResult {
	t.Helper()
	var results []protocol.ItemUseResult
	for _, envelope := range drainReliable(connection) {
		if result, ok := envelope.Message.(protocol.ItemUseResult); ok {
			results = append(results, result)
		}
	}
	if len(results) != 1 {
		t.Fatalf("item-use results=%#v", results)
	}
	return results[0]
}
