package worldruntime

import "testing"

func TestS3E8ReplicationBudgetDefaults(t *testing.T) {
	config := DefaultConfig()
	if config.MaxLifecycleMessagesPerSnapshot != 16000 {
		t.Fatalf("pure lifecycle budget=%d want=16000", config.MaxLifecycleMessagesPerSnapshot)
	}
	if config.MaxChurnLifecycleMessagesPerSnapshot != 6000 {
		t.Fatalf("churn lifecycle budget=%d want=6000", config.MaxChurnLifecycleMessagesPerSnapshot)
	}
	if config.MaxInitialVitalsPerTick != 8000 {
		t.Fatalf("pure initial vitals budget=%d want=8000", config.MaxInitialVitalsPerTick)
	}
	if config.MaxChurnInitialVitalsPerTick != 2500 {
		t.Fatalf("churn initial vitals budget=%d want=2500", config.MaxChurnInitialVitalsPerTick)
	}
	if config.MaxDirtyVitalsPerTick != 4000 {
		t.Fatalf("dirty vitals budget=%d want=4000", config.MaxDirtyVitalsPerTick)
	}
}
