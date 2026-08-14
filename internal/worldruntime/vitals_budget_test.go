package worldruntime

import "testing"

func TestStaggeredInitialVitalsBudgetPreservesCycleCapacity(t *testing.T) {
	tests := []struct {
		name          string
		base          int
		snapshotEvery uint64
		want          []int
	}{
		{name: "every tick cannot stagger", base: 2500, snapshotEvery: 1, want: []int{2500, 2500}},
		{name: "20Hz world 10Hz snapshot", base: 2500, snapshotEvery: 2, want: []int{0, 5000, 0, 5000}},
		{name: "four tick cycle", base: 3000, snapshotEvery: 4, want: []int{0, 4000, 4000, 4000}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var cycleTotal int
			for i, want := range tt.want {
				got := staggeredInitialVitalsBudget(tt.base, uint64(i), tt.snapshotEvery)
				if got != want {
					t.Fatalf("tick=%d budget=%d want=%d", i, got, want)
				}
				if tt.snapshotEvery > 1 && i < int(tt.snapshotEvery) {
					cycleTotal += got
				}
			}
			if tt.snapshotEvery > 1 && cycleTotal < tt.base*int(tt.snapshotEvery) {
				t.Fatalf("cycle capacity=%d below original=%d", cycleTotal, tt.base*int(tt.snapshotEvery))
			}
		})
	}
}
