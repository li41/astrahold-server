package character

import (
	"testing"

	"github.com/li41/astrahold-server/internal/classid"
	"github.com/li41/astrahold-server/internal/world"
)

func TestRegisterStateAcceptsUnassignedAndCanonicalClassIDs(t *testing.T) {
	service, err := NewServiceWithResources(1000, 100)
	if err != nil {
		t.Fatal(err)
	}
	classIDs := append([]classid.ID{""}, classid.All()...)
	for index, id := range classIDs {
		entityID := world.EntityID(index + 1)
		state := State{EntityID: entityID, ClassID: id, HP: 1000, MaxHP: 1000, MP: 100, MaxMP: 100}
		if err := service.RegisterState(state); err != nil {
			t.Fatalf("ClassID %q rejected: %v", id, err)
		}
		got, ok := service.State(entityID)
		if !ok || got.ClassID != id {
			t.Fatalf("ClassID %q not retained: state=%#v ok=%v", id, got, ok)
		}
	}
}

func TestRegisterStateRejectsUnknownOrNonCanonicalClassID(t *testing.T) {
	for _, id := range []classid.ID{"class_guard", " class_oathguard", "class_oathguard "} {
		service, err := NewServiceWithResources(1000, 100)
		if err != nil {
			t.Fatal(err)
		}
		state := State{EntityID: 1, ClassID: id, HP: 1000, MaxHP: 1000, MP: 100, MaxMP: 100}
		if err := service.RegisterState(state); err != ErrInvalidState {
			t.Fatalf("ClassID %q err=%v want=%v", id, err, ErrInvalidState)
		}
	}
}
