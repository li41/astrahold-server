package combat

import "testing"

func TestPrepareIntentAssignsStableMonotonicActionInstanceIDs(t *testing.T) {
	svc, err := NewService([]ActionDefinition{testAction()})
	if err != nil { t.Fatal(err) }
	intent := Intent{ActorEntityID: 42, ActionID: "basic-attack", Target: Target{Kind: TargetGate, ID: "main-gate"}}
	first, err := svc.PrepareIntent(intent, 10)
	if err != nil { t.Fatal(err) }
	second, err := svc.PrepareIntent(intent, 10)
	if err != nil { t.Fatal(err) }
	if first.ActionInstanceID == 0 || second.ActionInstanceID != first.ActionInstanceID+1 {
		t.Fatalf("instance ids first=%d second=%d", first.ActionInstanceID, second.ActionInstanceID)
	}
	if first.ActorEntityID != 42 || first.Damage.Source.ActorEntityID != 42 {
		t.Fatalf("actor ownership not preserved: %+v", first)
	}
}
