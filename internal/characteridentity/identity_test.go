package characteridentity

import (
	"errors"
	"testing"
)

func TestTrustedAndEphemeralBindings(t *testing.T) {
	trusted, err := NewTrusted("character:alpha-01")
	if err != nil || !trusted.Valid() || trusted.Assurance != AssuranceTrusted {
		t.Fatalf("trusted=%#v err=%v", trusted, err)
	}
	ephemeral, err := NewEphemeral()
	if err != nil || !ephemeral.Valid() || ephemeral.Assurance != AssuranceEphemeral || ephemeral.ID == trusted.ID {
		t.Fatalf("ephemeral=%#v err=%v", ephemeral, err)
	}
	second, err := NewEphemeral()
	if err != nil || second.ID == ephemeral.ID {
		t.Fatalf("second=%#v err=%v", second, err)
	}
}

func TestBindingRejectsReservedOrMalformedValues(t *testing.T) {
	if _, err := NewTrusted("ephemeral:0123456789abcdef0123456789abcdef"); !errors.Is(err, ErrInvalidID) {
		t.Fatalf("reserved err=%v", err)
	}
	for _, value := range []string{"", "space value", "slash/value"} {
		if _, err := NewTrusted(value); !errors.Is(err, ErrInvalidID) {
			t.Fatalf("value=%q err=%v", value, err)
		}
	}
	if _, err := New("ephemeral:gggggggggggggggggggggggggggggggg", AssuranceEphemeral); !errors.Is(err, ErrInvalidID) {
		t.Fatalf("malformed ephemeral err=%v", err)
	}
	if _, err := New("character:alpha", Assurance("unknown")); !errors.Is(err, ErrInvalidAssurance) {
		t.Fatalf("assurance err=%v", err)
	}
}
