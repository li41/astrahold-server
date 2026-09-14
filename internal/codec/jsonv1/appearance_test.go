package jsonv1

import (
	"reflect"
	"testing"

	"github.com/li41/astrahold-server/internal/appearance"
	"github.com/li41/astrahold-server/internal/protocol"
)

func TestAppearanceSnapshotRoundTrip(t *testing.T) {
	codec := Codec{}
	want := protocol.AppearanceSnapshot{
		SkinID:                   appearance.PeasantGirl,
		BasicAttackAffinityBonus: 1,
	}

	payload, err := codec.Marshal(want)
	if err != nil {
		t.Fatal(err)
	}
	if got := string(payload); got != `{"skin_id":"skin_peasant_girl","basic_attack_affinity_bonus":1}` {
		t.Fatalf("payload=%s", got)
	}

	message, err := codec.Unmarshal(protocol.MessageAppearanceSnapshot, payload)
	if err != nil {
		t.Fatal(err)
	}
	got, ok := message.(protocol.AppearanceSnapshot)
	if !ok {
		t.Fatalf("message type=%T", message)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("round trip=%#v want=%#v", got, want)
	}
}

func TestAppearanceSnapshotEmptySelectionStillCarriesAuthoritativeZeroBonus(t *testing.T) {
	codec := Codec{}
	payload, err := codec.Marshal(protocol.AppearanceSnapshot{})
	if err != nil {
		t.Fatal(err)
	}
	if got := string(payload); got != `{"basic_attack_affinity_bonus":0}` {
		t.Fatalf("payload=%s", got)
	}
}
