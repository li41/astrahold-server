package character

import (
	"errors"
	"testing"

	"github.com/li41/astrahold-server/internal/world"
)

func TestReduceHPAndDefeat(t *testing.T) {
	service, err := NewService(250)
	if err != nil { t.Fatal(err) }
	const id world.EntityID = 7
	if err := service.Register(id); err != nil { t.Fatal(err) }

	state, err := service.ReduceHP(id, 100)
	if err != nil { t.Fatal(err) }
	if state.HP != 150 || state.Defeated { t.Fatalf("after damage=%#v", state) }

	state, err = service.ReduceHP(id, 200)
	if err != nil { t.Fatal(err) }
	if state.HP != 0 || !state.Defeated { t.Fatalf("after defeat=%#v", state) }

	if _, err := service.ReduceHP(id, 1); !errors.Is(err, ErrCharacterDefeated) {
		t.Fatalf("err=%v want ErrCharacterDefeated", err)
	}
}
