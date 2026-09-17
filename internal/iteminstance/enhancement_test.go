package iteminstance

import "testing"

func TestEnhancementLevelCanonicalShapeRoundTrip(t *testing.T) {
	original := Instance{ID: ID("instance:enhanced"), ItemArchetypeID: "item_test_weapon", EnhancementLevel: 7}
	data, err := CanonicalShapeJSON(original)
	if err != nil { t.Fatal(err) }
	restored, err := DecodeCanonicalShapeJSON(data)
	if err != nil { t.Fatal(err) }
	if restored.ID != original.ID || restored.ItemArchetypeID != original.ItemArchetypeID || restored.EnhancementLevel != 7 {
		t.Fatalf("restored=%#v", restored)
	}
}

func TestEnhancementLevelZeroPreservesLegacyCanonicalShape(t *testing.T) {
	legacy := []byte(`{"item_instance_id":"instance:legacy","item_archetype_id":"item_test_weapon"}`)
	restored, err := DecodeCanonicalShapeJSON(legacy)
	if err != nil { t.Fatal(err) }
	if restored.EnhancementLevel != 0 { t.Fatalf("enhancement=%d", restored.EnhancementLevel) }
}
