package targetresource

import (
	"testing"

	"github.com/li41/astrahold-server/internal/world"
)

func TestTryGainIsSourceTargetScopedAndRespectsICD(t *testing.T) {
	s := NewStore()
	key := Key{SourceEntityID: 10, TargetEntityID: 20, ResourceID: Flaw}
	state, changed, err := s.TryGain(key, 1, 3, 5, 55)
	if err != nil || !changed || state.Current != 1 || state.ReadyTick != 55 { t.Fatalf("first gain state=%+v changed=%v err=%v", state, changed, err) }
	state, changed, err = s.TryGain(key, 1, 3, 54, 104)
	if err != nil || changed || state.Current != 1 { t.Fatalf("icd gain state=%+v changed=%v err=%v", state, changed, err) }
	state, changed, err = s.TryGain(key, 1, 3, 55, 105)
	if err != nil || !changed || state.Current != 2 { t.Fatalf("ready gain state=%+v changed=%v err=%v", state, changed, err) }

	other := Key{SourceEntityID: 11, TargetEntityID: 20, ResourceID: Flaw}
	otherState, changed, err := s.TryGain(other, 1, 3, 6, 56)
	if err != nil || !changed || otherState.Current != 1 { t.Fatalf("other source state=%+v changed=%v err=%v", otherState, changed, err) }
	state, _ = s.State(key)
	if state.Current != 2 { t.Fatalf("other source mutated original=%+v", state) }
}

func TestTryGainClampsAndClearEntityRemovesBothRoles(t *testing.T) {
	s := NewStore()
	first := Key{SourceEntityID: 10, TargetEntityID: 20, ResourceID: Flaw}
	second := Key{SourceEntityID: 30, TargetEntityID: 10, ResourceID: Flaw}
	if state, changed, err := s.TryGain(first, 9, 3, 1, 2); err != nil || !changed || state.Current != 3 { t.Fatalf("clamp state=%+v changed=%v err=%v", state, changed, err) }
	if _, _, err := s.TryGain(second, 1, 3, 1, 2); err != nil { t.Fatal(err) }
	removed := s.ClearEntity(world.EntityID(10))
	if len(removed) != 2 { t.Fatalf("removed=%#v", removed) }
	if _, ok := s.State(first); ok { t.Fatal("source-target state survived clear") }
	if _, ok := s.State(second); ok { t.Fatal("target-source state survived clear") }
}
