package loadlab

import "testing"

func TestParseS3E9MixedDynamicUpdates(t *testing.T) {
	for _, test := range []struct {
		name  string
		value string
		want  uint64
	}{
		{name: "unset", value: "", want: 0},
		{name: "valid", value: "28", want: 28},
		{name: "zero", value: "0", want: 0},
		{name: "invalid", value: "not-a-number", want: 0},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := parseS3E9MixedDynamicUpdates(test.value); got != test.want {
				t.Fatalf("parseS3E9MixedDynamicUpdates(%q)=%d want=%d", test.value, got, test.want)
			}
		})
	}
}

func TestS3E9MixedMovementWindowOpenStartsAfterBootstrapAndStopsAfterFinalObjectiveFanout(t *testing.T) {
	oldEnabled := s3e9MixedMovementEnabled
	oldUpdates := s3e9MixedDynamicUpdates
	defer func() {
		s3e9MixedMovementEnabled = oldEnabled
		s3e9MixedDynamicUpdates = oldUpdates
	}()

	s3e9MixedMovementEnabled = true
	s3e9MixedDynamicUpdates = 28
	const ready uint64 = 500

	for _, test := range []struct {
		name          string
		dynamicStates uint64
		wantOpen      bool
	}{
		{name: "before bootstrap", dynamicStates: 499, wantOpen: false},
		{name: "bootstrap complete", dynamicStates: 500, wantOpen: false},
		{name: "first objective delivery", dynamicStates: 501, wantOpen: true},
		{name: "before final fanout completes", dynamicStates: 14499, wantOpen: true},
		{name: "final fanout complete", dynamicStates: 14500, wantOpen: false},
		{name: "after final fanout", dynamicStates: 14501, wantOpen: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := s3e9MixedMovementWindowOpen(test.dynamicStates, ready); got != test.wantOpen {
				t.Fatalf("open(dynamic=%d ready=%d)=%t want=%t", test.dynamicStates, ready, got, test.wantOpen)
			}
		})
	}
}

func TestS3E9MixedMovementWindowOpenRequiresEnabledFixtureAndReadyBots(t *testing.T) {
	oldEnabled := s3e9MixedMovementEnabled
	oldUpdates := s3e9MixedDynamicUpdates
	defer func() {
		s3e9MixedMovementEnabled = oldEnabled
		s3e9MixedDynamicUpdates = oldUpdates
	}()

	s3e9MixedDynamicUpdates = 28
	s3e9MixedMovementEnabled = false
	if s3e9MixedMovementWindowOpen(501, 500) {
		t.Fatal("movement window opened while fixture was disabled")
	}

	s3e9MixedMovementEnabled = true
	if s3e9MixedMovementWindowOpen(1, 0) {
		t.Fatal("movement window opened with zero ready bots")
	}
}
