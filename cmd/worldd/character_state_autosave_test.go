package main

import (
	"math"
	"testing"

	"github.com/li41/astrahold-server/internal/worldruntime"
)

func TestCharacterStateAutosaveTicks(t *testing.T) {
	for _, tc := range []struct {
		name     string
		seconds  float64
		tickRate int
		want     uint64
	}{
		{name: "disabled", seconds: 0, tickRate: 20, want: 0},
		{name: "thirty seconds", seconds: 30, tickRate: 20, want: 600},
		{name: "ceil sub tick", seconds: 0.01, tickRate: 20, want: 1},
		{name: "ceil fractional", seconds: 0.051, tickRate: 20, want: 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := characterStateAutosaveTicks(tc.seconds, tc.tickRate)
			if err != nil {
				t.Fatal(err)
			}
			if got != tc.want {
				t.Fatalf("ticks=%d want=%d", got, tc.want)
			}
		})
	}
}

func TestCharacterStateAutosaveTicksRejectsInvalidInput(t *testing.T) {
	for _, seconds := range []float64{-1, math.NaN(), math.Inf(1)} {
		if _, err := characterStateAutosaveTicks(seconds, 20); err == nil {
			t.Fatalf("seconds=%v should fail", seconds)
		}
	}
	if _, err := characterStateAutosaveTicks(1, 0); err == nil {
		t.Fatal("zero tick rate should fail")
	}
}

func TestConfigureCharacterStateAutosave(t *testing.T) {
	cfg := worldruntime.DefaultConfig()
	ticks, err := configureCharacterStateAutosave(&cfg, 30, 32, 20)
	if err != nil {
		t.Fatal(err)
	}
	if ticks != 600 || cfg.CharacterStateAutosaveEveryTicks != 600 || cfg.MaxCharacterStateAutosavesPerTick != 32 {
		t.Fatalf("ticks=%d config=%#v", ticks, cfg)
	}

	if _, err := configureCharacterStateAutosave(&cfg, 0, 0, 20); err != nil {
		t.Fatal(err)
	}
	if cfg.CharacterStateAutosaveEveryTicks != 0 || cfg.MaxCharacterStateAutosavesPerTick != 0 {
		t.Fatalf("disabled config=%#v", cfg)
	}
}

func TestConfigureCharacterStateAutosaveRejectsInvalidBudget(t *testing.T) {
	cfg := worldruntime.DefaultConfig()
	if _, err := configureCharacterStateAutosave(&cfg, 30, 0, 20); err == nil {
		t.Fatal("enabled autosave accepted zero budget")
	}
	if _, err := configureCharacterStateAutosave(&cfg, 30, -1, 20); err == nil {
		t.Fatal("autosave accepted negative budget")
	}
	if _, err := configureCharacterStateAutosave(nil, 30, 32, 20); err == nil {
		t.Fatal("autosave accepted nil config")
	}
}
