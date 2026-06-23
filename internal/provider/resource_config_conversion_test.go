// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"reflect"
	"testing"
)

// TestResourceConfigRoundTrip checks the resource config dynamic<->map bridge
// used to feed *structpb.Struct. Numbers settle to float64 (Terraform Number
// semantics), so the fixture uses float64 to assert a stable round-trip.
func TestResourceConfigRoundTrip(t *testing.T) {
	t.Parallel()
	in := map[string]interface{}{
		"str":  "value",
		"flag": true,
		"num":  float64(3),
		"nested": map[string]interface{}{
			"inner": "x",
		},
	}

	dyn := goMapToDynamic(in)
	if dyn.IsNull() {
		t.Fatal("goMapToDynamic returned null for non-empty map")
	}

	out, err := resourceConfigFromDynamic(dyn)
	if err != nil {
		t.Fatalf("resourceConfigFromDynamic: %v", err)
	}
	if !reflect.DeepEqual(out, in) {
		t.Errorf("round-trip mismatch:\n got  %#v\n want %#v", out, in)
	}
}

func TestResourceConfigEmpty(t *testing.T) {
	t.Parallel()
	// Empty map -> null dynamic -> empty map.
	dyn := goMapToDynamic(map[string]interface{}{})
	if !dyn.IsNull() {
		t.Errorf("empty map should produce null dynamic, got %v", dyn)
	}
	out, err := resourceConfigFromDynamic(dyn)
	if err != nil {
		t.Fatalf("resourceConfigFromDynamic: %v", err)
	}
	if len(out) != 0 {
		t.Errorf("null dynamic should produce empty map, got %#v", out)
	}
}
