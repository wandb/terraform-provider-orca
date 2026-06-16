// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"reflect"
	"testing"
)

// TestJobAgentConfigRoundTrip locks the *structpb.Struct bridge for job agent
// config. structpb stores all JSON numbers as float64, so an int64 input comes
// back as float64 — downstream code uses toInt64 to absorb this. The test
// asserts that documented behavior explicitly so a future change is caught.
func TestJobAgentConfigRoundTrip(t *testing.T) {
	t.Parallel()
	in := map[string]interface{}{
		"url":     "https://example.com",
		"enabled": true,
		"retries": int64(5),
	}

	st, err := jobAgentConfigStruct(&in)
	if err != nil {
		t.Fatalf("jobAgentConfigStruct: %v", err)
	}
	out := jobAgentConfigMap(st)

	want := map[string]interface{}{
		"url":     "https://example.com",
		"enabled": true,
		"retries": float64(5), // structpb coerces numbers to float64
	}
	if !reflect.DeepEqual(out, want) {
		t.Errorf("round-trip mismatch:\n got  %#v\n want %#v", out, want)
	}
}

func TestJobAgentConfigNil(t *testing.T) {
	t.Parallel()
	st, err := jobAgentConfigStruct(nil)
	if err != nil {
		t.Fatalf("jobAgentConfigStruct(nil): %v", err)
	}
	if st != nil {
		t.Errorf("nil config should yield nil struct, got %v", st)
	}
	if got := jobAgentConfigMap(nil); len(got) != 0 {
		t.Errorf("nil struct should yield empty map, got %#v", got)
	}
}
