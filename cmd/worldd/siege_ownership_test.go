package main

import (
	"path/filepath"
	"testing"

	"github.com/li41/astrahold-server/internal/siege"
)

func TestSiegeOwnershipPersistenceRecoversCommittedOwnerAcrossReopen(t *testing.T) {
	root := filepath.Join(t.TempDir(), "siege-ownership")
	first, initial, created, err := openSiegeOwnershipPersistence(root, "castle-sandbox", "defenders")
	if err != nil || !created || initial.Revision != 1 || initial.OwnerID != "defenders" {
		t.Fatalf("initial=%+v created=%v err=%v", initial, created, err)
	}
	committed, err := first.Commit(siege.CastleOwnershipTransfer{
		ExpectedRevision: 1,
		PreviousOwnerID:  "defenders",
		OwnerID:          "attackers",
		MatchID:          "m1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if committed.Revision != 2 || committed.OwnerID != "attackers" {
		t.Fatalf("committed=%+v", committed)
	}

	second, recovered, created, err := openSiegeOwnershipPersistence(root, "castle-sandbox", "defenders")
	if err != nil || created || recovered != committed {
		t.Fatalf("recovered=%+v created=%v err=%v", recovered, created, err)
	}
	if second.Path() != first.Path() {
		t.Fatalf("first path=%s second path=%s", first.Path(), second.Path())
	}
}
