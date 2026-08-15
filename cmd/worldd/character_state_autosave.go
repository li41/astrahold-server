package main

import (
	"flag"
	"fmt"
	"math"

	"github.com/li41/astrahold-server/internal/worldruntime"
)

var (
	characterStateAutosaveSeconds = flag.Float64(
		"character-state-autosave-seconds",
		30.0,
		"Periodic trusted character autosave interval; 0 disables",
	)
	characterStateAutosavesPerTick = flag.Int(
		"character-state-autosaves-per-tick",
		32,
		"Maximum trusted character autosave captures per world tick",
	)
)

func characterStateAutosaveTicks(seconds float64, tickRate int) (uint64, error) {
	if tickRate <= 0 {
		return 0, fmt.Errorf("character-state autosave tick rate must be > 0")
	}
	if seconds < 0 || math.IsNaN(seconds) || math.IsInf(seconds, 0) {
		return 0, fmt.Errorf("character-state-autosave-seconds must be finite and >= 0")
	}
	if seconds == 0 {
		return 0, nil
	}
	ticks := math.Ceil(seconds * float64(tickRate))
	if ticks <= 0 || ticks > float64(^uint64(0)) {
		return 0, fmt.Errorf("character-state-autosave-seconds overflows tick duration")
	}
	return uint64(ticks), nil
}

func configureCharacterStateAutosave(config *worldruntime.Config, seconds float64, perTick int, tickRate int) (uint64, error) {
	if config == nil {
		return 0, fmt.Errorf("character-state autosave config is required")
	}
	if perTick < 0 {
		return 0, fmt.Errorf("character-state-autosaves-per-tick must be >= 0")
	}
	ticks, err := characterStateAutosaveTicks(seconds, tickRate)
	if err != nil {
		return 0, err
	}
	if ticks > 0 && perTick == 0 {
		return 0, fmt.Errorf("character-state-autosaves-per-tick must be > 0 when autosave is enabled")
	}
	config.CharacterStateAutosaveEveryTicks = ticks
	config.MaxCharacterStateAutosavesPerTick = perTick
	return ticks, nil
}
