// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// TestDeploymentVariableValueRoundTrip exercises the structpb.Value encoding
// that replaced the old api.Value union: literal scalars and the reference
// sentinel ({reference, path}) must survive a model -> *structpb.Value -> model
// round-trip unchanged. This locks the wire shape the engine is expected to
// accept, without needing a live engine.
func TestDeploymentVariableValueRoundTrip(t *testing.T) {
	t.Parallel()

	refObj := func(reference string, path ...string) types.Object {
		elems := make([]attr.Value, len(path))
		for i, p := range path {
			elems[i] = types.StringValue(p)
		}
		list, d := types.ListValue(types.StringType, elems)
		if d.HasError() {
			t.Fatalf("build path list: %v", d)
		}
		obj, d := types.ObjectValue(referenceValueAttrTypes, map[string]attr.Value{
			"reference": types.StringValue(reference),
			"path":      list,
		})
		if d.HasError() {
			t.Fatalf("build reference object: %v", d)
		}
		return obj
	}

	cases := []struct {
		name string
		in   DeploymentVariableValueResourceModel
	}{
		{
			name: "literal_string",
			in: DeploymentVariableValueResourceModel{
				LiteralValue:   types.DynamicValue(types.StringValue("hello")),
				ReferenceValue: types.ObjectNull(referenceValueAttrTypes),
			},
		},
		{
			name: "literal_int",
			in: DeploymentVariableValueResourceModel{
				LiteralValue:   types.DynamicValue(types.Int64Value(42)),
				ReferenceValue: types.ObjectNull(referenceValueAttrTypes),
			},
		},
		{
			name: "literal_bool",
			in: DeploymentVariableValueResourceModel{
				LiteralValue:   types.DynamicValue(types.BoolValue(true)),
				ReferenceValue: types.ObjectNull(referenceValueAttrTypes),
			},
		},
		{
			name: "reference",
			in: DeploymentVariableValueResourceModel{
				LiteralValue:   types.DynamicNull(),
				ReferenceValue: refObj("other_resource", "a", "b"),
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			val, err := structpbValueFromModel(tc.in)
			if err != nil {
				t.Fatalf("structpbValueFromModel: %v", err)
			}

			var out DeploymentVariableValueResourceModel
			if d := setValueOnModel(context.Background(), &out, val); d.HasError() {
				t.Fatalf("setValueOnModel: %v", d)
			}

			if !out.LiteralValue.Equal(tc.in.LiteralValue) {
				t.Errorf("LiteralValue round-trip mismatch: got %v want %v", out.LiteralValue, tc.in.LiteralValue)
			}
			if !out.ReferenceValue.Equal(tc.in.ReferenceValue) {
				t.Errorf("ReferenceValue round-trip mismatch: got %v want %v", out.ReferenceValue, tc.in.ReferenceValue)
			}
		})
	}
}

func TestStructpbValueFromModelRequiresOneOf(t *testing.T) {
	t.Parallel()
	_, err := structpbValueFromModel(DeploymentVariableValueResourceModel{
		LiteralValue:   types.DynamicNull(),
		ReferenceValue: types.ObjectNull(referenceValueAttrTypes),
	})
	if err == nil {
		t.Fatal("expected error when neither literal nor reference value is set")
	}
}
