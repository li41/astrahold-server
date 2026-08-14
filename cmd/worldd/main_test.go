package main

import (
	"math"
	"testing"

	"github.com/li41/astrahold-server/internal/deathoutcome"
	"github.com/li41/astrahold-server/internal/respawnpolicy"
)

func TestReviveProtectionTicks(t *testing.T) {
	for _, tc := range []struct {
		name     string
		seconds  float64
		tickRate int
		want     uint64
	}{
		{name: "disabled", seconds: 0, tickRate: 20, want: 0},
		{name: "three seconds", seconds: 3, tickRate: 20, want: 60},
		{name: "ceil fractional tick", seconds: 0.051, tickRate: 20, want: 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := reviveProtectionTicks(tc.seconds, tc.tickRate)
			if err != nil {
				t.Fatal(err)
			}
			if got != tc.want {
				t.Fatalf("ticks=%d want=%d", got, tc.want)
			}
		})
	}
}

func TestReviveProtectionTicksRejectsInvalidSeconds(t *testing.T) {
	for _, value := range []float64{-1, math.NaN(), math.Inf(1)} {
		if _, err := reviveProtectionTicks(value, 20); err == nil {
			t.Fatalf("value=%v should fail", value)
		}
	}
}

func TestDrainDeathOutcomeBatchLogsAndConfirmsOldestEvents(t *testing.T) {
	outbox, err := deathoutcome.NewOutbox(4)
	if err != nil {
		t.Fatal(err)
	}
	for entity := uint64(1); entity <= 2; entity++ {
		if _, created, err := outbox.Enqueue(deathoutcome.Event{
			EntityID:       worldEntityID(entity),
			DefeatRevision: 1,
			Context:        respawnpolicy.DeathContextPvP,
			DefeatedTick:   entity,
		}); err != nil || !created {
			t.Fatalf("enqueue entity=%d created=%v err=%v", entity, created, err)
		}
	}
	if got := drainDeathOutcomeBatch(outbox); got != 2 {
		t.Fatalf("drained=%d", got)
	}
	if outbox.Depth() != 0 {
		t.Fatalf("depth=%d", outbox.Depth())
	}
}

// world.EntityID 的 underlying type是 uint64；小 helper讓此測試不必額外耦合 world package API。
func worldEntityID(value uint64) uint64 { return value }
