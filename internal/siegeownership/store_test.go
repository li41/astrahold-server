package siegeownership

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/li41/astrahold-server/internal/siege"
)

func TestStoreLoadOrCreateCommitReopenAndIdempotentReplay(t *testing.T) {
	root := filepath.Join(t.TempDir(), "ownership")
	store, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	initial, created, err := store.LoadOrCreate("castle-sandbox", "defenders")
	if err != nil || !created || initial.Revision != 1 || initial.OwnerID != "defenders" {
		t.Fatalf("initial=%+v created=%v err=%v", initial, created, err)
	}
	if loaded, ok, err := store.Load("castle-sandbox"); err != nil || !ok || loaded != initial {
		t.Fatalf("loaded=%+v ok=%v err=%v", loaded, ok, err)
	}

	transfer := siege.CastleOwnershipTransfer{
		ExpectedRevision: 1,
		PreviousOwnerID:  "defenders",
		OwnerID:          "attackers",
		MatchID:          "m1",
	}
	committed, err := store.Commit("castle-sandbox", transfer)
	if err != nil {
		t.Fatal(err)
	}
	if committed.Revision != 2 || committed.OwnerID != "attackers" || committed.PreviousOwnerID != "defenders" || committed.LastTransferMatchID != "m1" {
		t.Fatalf("committed=%+v", committed)
	}
	if replay, err := store.Commit("castle-sandbox", transfer); err != nil || replay != committed {
		t.Fatalf("replay=%+v err=%v", replay, err)
	}

	reopened, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	recovered, created, err := reopened.LoadOrCreate("castle-sandbox", "defenders")
	if err != nil || created || recovered != committed {
		t.Fatalf("recovered=%+v created=%v err=%v", recovered, created, err)
	}
}

func TestStoreRejectsStaleCASAndKeepsDurableTruth(t *testing.T) {
	store, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.LoadOrCreate("castle-sandbox", "defenders"); err != nil {
		t.Fatal(err)
	}
	first := siege.CastleOwnershipTransfer{ExpectedRevision: 1, PreviousOwnerID: "defenders", OwnerID: "attackers", MatchID: "m1"}
	committed, err := store.Commit("castle-sandbox", first)
	if err != nil {
		t.Fatal(err)
	}
	stale := siege.CastleOwnershipTransfer{ExpectedRevision: 1, PreviousOwnerID: "defenders", OwnerID: "third-side", MatchID: "m2"}
	if _, err := store.Commit("castle-sandbox", stale); !errors.Is(err, ErrRevisionConflict) {
		t.Fatalf("stale err=%v", err)
	}
	loaded, ok, err := store.Load("castle-sandbox")
	if err != nil || !ok || loaded != committed {
		t.Fatalf("loaded=%+v ok=%v err=%v", loaded, ok, err)
	}
}

func TestStoreNoOpOwnerDoesNotAdvanceRevision(t *testing.T) {
	store, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	initial, _, err := store.LoadOrCreate("castle-sandbox", "attackers")
	if err != nil {
		t.Fatal(err)
	}
	transfer := siege.CastleOwnershipTransfer{ExpectedRevision: 1, PreviousOwnerID: "attackers", OwnerID: "attackers", MatchID: "m2"}
	committed, err := store.Commit("castle-sandbox", transfer)
	if err != nil || committed != initial {
		t.Fatalf("committed=%+v err=%v", committed, err)
	}
}

func TestStoreFailsClosedOnUnknownTrailingAndMismatchedWorld(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(map[string]any) []byte
	}{
		{
			name: "unknown field",
			mutate: func(wire map[string]any) []byte {
				wire["unexpected"] = true
				data, _ := json.Marshal(wire)
				return append(data, '\n')
			},
		},
		{
			name: "trailing json",
			mutate: func(wire map[string]any) []byte {
				data, _ := json.Marshal(wire)
				return append(append(data, '\n'), []byte("{}\n")...)
			},
		},
		{
			name: "mismatched world",
			mutate: func(wire map[string]any) []byte {
				wire["world_id"] = "other-world"
				data, _ := json.Marshal(wire)
				return append(data, '\n')
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store, err := Open(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			if _, _, err := store.LoadOrCreate("castle-sandbox", "defenders"); err != nil {
				t.Fatal(err)
			}
			path := store.recordPath("castle-sandbox")
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			var wire map[string]any
			if err := json.Unmarshal(data, &wire); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, tc.mutate(wire), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, _, err := store.Load("castle-sandbox"); !errors.Is(err, ErrCorruptRecord) {
				t.Fatalf("load err=%v", err)
			}
		})
	}
}
