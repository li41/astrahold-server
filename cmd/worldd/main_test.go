package main

import (
	"math"
	"testing"
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
