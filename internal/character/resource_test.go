package character

import (
	"errors"
	"testing"

	"github.com/li41/astrahold-server/internal/world"
)

func TestSpendMPIsAuthoritativeAndAtomic(t *testing.T) {
	service, err := NewServiceWithResources(200, 100)
	if err != nil { t.Fatal(err) }
	const id world.EntityID = 11
	if err := service.Register(id); err != nil { t.Fatal(err) }

	state, err := service.SpendMP(id, 60)
	if err != nil { t.Fatal(err) }
	if state.MP != 40 || state.MaxMP != 100 { t.Fatalf("after spend=%#v", state) }

	before, _ := service.State(id)
	state, err = service.SpendMP(id, 60)
	if !errors.Is(err, ErrInsufficientResource) { t.Fatalf("err=%v want ErrInsufficientResource", err) }
	after, _ := service.State(id)
	if state != before || after != before { t.Fatalf("insufficient spend mutated state: before=%#v returned=%#v after=%#v", before, state, after) }
}
