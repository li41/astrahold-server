package worldruntime

import "testing"

func TestConvergenceSnapshotRequiresAllBootstrapState(t *testing.T) {
	base := ConvergenceSnapshot{
		ReplicationViews:     2,
		BuiltViews:           2,
		DesiredRelationships: 4,
		KnownDesired:         4,
	}
	if !base.Converged(2) {
		t.Fatalf("fully settled snapshot should converge: %+v", base)
	}

	cases := []struct {
		name string
		mutate func(*ConvergenceSnapshot)
	}{
		{"missing build", func(s *ConvergenceSnapshot) { s.BuiltViews-- }},
		{"pending spawn", func(s *ConvergenceSnapshot) { s.PendingSpawns = 1 }},
		{"pending despawn", func(s *ConvergenceSnapshot) { s.PendingDespawns = 1 }},
		{"pending vitals", func(s *ConvergenceSnapshot) { s.PendingVitalsEntities = 1 }},
		{"dirty vitals", func(s *ConvergenceSnapshot) { s.DirtyVitalsEntities = 1 }},
		{"pending dynamic", func(s *ConvergenceSnapshot) { s.PendingDynamicSessions = 1 }},
		{"known mismatch", func(s *ConvergenceSnapshot) { s.KnownDesired-- }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			snapshot := base
			tc.mutate(&snapshot)
			if snapshot.Converged(2) {
				t.Fatalf("snapshot unexpectedly converged: %+v", snapshot)
			}
		})
	}
}
