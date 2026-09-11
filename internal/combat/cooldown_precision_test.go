package combat

import (
	"testing"
	"time"
)

func TestCooldownTicksRespectsFloat32AuthoredTickBoundaries(t *testing.T) {
	tests := []struct {
		name    string
		seconds float32
		delta   time.Duration
		want    uint64
	}{
		{name: "one 20Hz tick", seconds: 0.05, delta: 50 * time.Millisecond, want: 1},
		{name: "two 20Hz ticks", seconds: 0.10, delta: 50 * time.Millisecond, want: 2},
		{name: "three 20Hz ticks", seconds: 0.15, delta: 50 * time.Millisecond, want: 3},
		{name: "half second at 20Hz", seconds: 0.5, delta: 50 * time.Millisecond, want: 10},
		{name: "non boundary still rounds up", seconds: 0.051, delta: 50 * time.Millisecond, want: 2},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := cooldownTicks(tc.seconds, tc.delta); got != tc.want {
				t.Fatalf("cooldownTicks(%v, %v)=%d want=%d", tc.seconds, tc.delta, got, tc.want)
			}
		})
	}
}

func TestCooldownReadyTickAllowsAuthoredFiftyMillisecondsOnNext20HzTick(t *testing.T) {
	action := ActionDefinition{CooldownSeconds: 0.05}
	if got := CooldownReadyTick(action, 100, 50*time.Millisecond); got != 101 {
		t.Fatalf("ready tick=%d want=101", got)
	}
}
