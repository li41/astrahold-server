package character

import (
	"errors"
	"testing"

	"github.com/li41/astrahold-server/internal/world"
)

func TestRegisterStateRestoresAuthoritativeHealth(t *testing.T) {
	service, err := NewService(1000)
	if err != nil {
		t.Fatal(err)
	}
	want := State{EntityID: 7, HP: 640, MaxHP: 1200}
	if err := service.RegisterState(want); err != nil {
		t.Fatal(err)
	}
	got, ok := service.State(7)
	if !ok || got != want {
		t.Fatalf("state=%#v ok=%v", got, ok)
	}
	if err := service.RegisterState(want); !errors.Is(err, ErrCharacterExists) {
		t.Fatalf("duplicate err=%v", err)
	}
}

func TestRegisterStateValidatesHealthInvariants(t *testing.T) {
	service, _ := NewService(1000)
	invalid := []State{
		{EntityID: 0, HP: 1, MaxHP: 1},
		{EntityID: 1, HP: 0, MaxHP: 1000},
		{EntityID: 2, HP: 1, MaxHP: 0},
		{EntityID: 3, HP: 1001, MaxHP: 1000},
		{EntityID: 4, HP: 1, MaxHP: 1000, Defeated: true},
	}
	for _, state := range invalid {
		if err := service.RegisterState(state); err == nil {
			t.Fatalf("state %#v unexpectedly accepted", state)
		}
	}
	if err := service.RegisterState(State{EntityID: world.EntityID(5), HP: 0, MaxHP: 1000, Defeated: true}); err != nil {
		t.Fatalf("valid defeated state rejected: %v", err)
	}
}
