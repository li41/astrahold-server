package playerarchetype

import "testing"

func TestDefaultIDIsCanonical(t *testing.T) {
	if DefaultID != "player_default" {
		t.Fatalf("DefaultID = %q", DefaultID)
	}
	if DefaultID == "" {
		t.Fatal("DefaultID must not be empty")
	}
}
