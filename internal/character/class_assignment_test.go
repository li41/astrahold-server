package character

import (
	"errors"
	"testing"

	"github.com/li41/astrahold-server/internal/classid"
)

func TestAssignInitialClassAllowsOnlyUnassignedToCanonical(t *testing.T) {
	service, err := NewService(1000)
	if err != nil {
		t.Fatal(err)
	}
	if err := service.Register(1); err != nil {
		t.Fatal(err)
	}
	state, err := service.AssignInitialClass(1, classid.Oathguard)
	if err != nil {
		t.Fatal(err)
	}
	if state.ClassID != classid.Oathguard {
		t.Fatalf("class=%q", state.ClassID)
	}
	if _, err := service.AssignInitialClass(1, classid.Oathguard); !errors.Is(err, ErrClassAlreadyAssigned) {
		t.Fatalf("same class second assignment err=%v", err)
	}
	if _, err := service.AssignInitialClass(1, classid.Breaker); !errors.Is(err, ErrClassAlreadyAssigned) {
		t.Fatalf("different class transfer err=%v", err)
	}
}

func TestAssignInitialClassRejectsInvalidAndMissingCharacter(t *testing.T) {
	service, err := NewService(1000)
	if err != nil {
		t.Fatal(err)
	}
	if err := service.Register(1); err != nil {
		t.Fatal(err)
	}
	for _, target := range []classid.ID{"", "class_guard", " class_oathguard"} {
		if _, err := service.AssignInitialClass(1, target); !errors.Is(err, ErrInvalidClassAssignment) {
			t.Fatalf("target=%q err=%v", target, err)
		}
	}
	if _, err := service.AssignInitialClass(99, classid.Ranger); !errors.Is(err, ErrCharacterNotFound) {
		t.Fatalf("missing character err=%v", err)
	}
}
