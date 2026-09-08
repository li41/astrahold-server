package targetresource

import (
	"errors"
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

func TestSpendRetainsReadyTickAtZeroAndRejectsInsufficientResource(t *testing.T) {
	s := NewStore()
	key := Key{SourceEntityID: 10, TargetEntityID: 20, ResourceID: Flaw}
	state, changed, err := s.TryGain(key, 3, 3, 5, 55)
	if err != nil || !changed || state.Current != 3 || state.ReadyTick != 55 { t.Fatalf("seed state=%+v changed=%v err=%v", state, changed, err) }

	state, err = s.Spend(key, 2)
	if err != nil || state.Current != 1 || state.ReadyTick != 55 { t.Fatalf("spend two state=%+v err=%v", state, err) }
	state, err = s.Spend(key, 1)
	if err != nil || state.Current != 0 || state.ReadyTick != 55 { t.Fatalf("spend to zero state=%+v err=%v", state, err) }
	stored, ok := s.State(key)
	if !ok || stored.Current != 0 || stored.ReadyTick != 55 { t.Fatalf("zero state=%+v ok=%v", stored, ok) }

	state, err = s.Spend(key, 1)
	if !errors.Is(err, ErrInsufficientResource) || state.Current != 0 || state.ReadyTick != 55 { t.Fatalf("insufficient spend state=%+v err=%v", state, err) }
	state, changed, err = s.TryGain(key, 1, 3, 54, 60)
	if err != nil || changed || state.Current != 0 || state.ReadyTick != 55 { t.Fatalf("spend bypassed build ICD state=%+v changed=%v err=%v", state, changed, err) }
	state, changed, err = s.TryGain(key, 1, 3, 55, 60)
	if err != nil || !changed || state.Current != 1 || state.ReadyTick != 60 { t.Fatalf("ready rebuild state=%+v changed=%v err=%v", state, changed, err) }
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
