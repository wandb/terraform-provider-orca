// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"
	structpb "google.golang.org/protobuf/types/known/structpb"
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

func TestLiteralCollectionRoundTrip(t *testing.T) {
	ctx := context.Background()
	strings := []attr.Value{types.StringValue("10.0.0.0/8"), types.StringValue("192.168.0.0/16")}
	list := types.ListValueMust(types.StringType, strings)
	cases := map[string]attr.Value{
		"list":         list,
		"empty_list":   types.ListValueMust(types.StringType, nil),
		"set":          types.SetValueMust(types.StringType, strings),
		"tuple":        types.TupleValueMust([]attr.Type{types.StringType, types.BoolType}, []attr.Value{strings[0], types.BoolValue(true)}),
		"map":          types.MapValueMust(types.ListType{ElemType: types.StringType}, map[string]attr.Value{"ips": list}),
		"object":       types.ObjectValueMust(map[string]attr.Type{"ips": types.ListType{ElemType: types.StringType}}, map[string]attr.Value{"ips": list}),
		"null_element": types.ListValueMust(types.StringType, []attr.Value{types.StringNull()}),
	}
	for name, literal := range cases {
		t.Run(name, func(t *testing.T) {
			model := DeploymentVariableValueResourceModel{LiteralValue: types.DynamicValue(literal), ReferenceValue: types.ObjectNull(referenceValueAttrTypes)}
			wire, err := structpbValueFromModel(model)
			if err != nil {
				t.Fatal(err)
			}
			// Create/update start with the plan; subsequent refreshes start with state.
			for i := 0; i < 2; i++ {
				if d := setValueOnModel(ctx, &model, wire); d.HasError() {
					t.Fatal(d)
				}
				if !model.LiteralValue.Equal(types.DynamicValue(literal)) {
					t.Fatalf("round trip changed value/type: got %v, want %v", model.LiteralValue, literal)
				}
			}
		})
	}
}

func TestLiteralCollectionRead(t *testing.T) {
	ctx := context.Background()
	wire, err := structpb.NewValue([]any{"new"})
	if err != nil {
		t.Fatal(err)
	}
	t.Run("refresh_reads_remote_changes", func(t *testing.T) {
		model := DeploymentVariableValueResourceModel{LiteralValue: types.DynamicValue(types.ListValueMust(types.StringType, []attr.Value{types.StringValue("old")}))}
		if d := setValueOnModel(ctx, &model, wire); d.HasError() {
			t.Fatal(d)
		}
		want := types.DynamicValue(types.ListValueMust(types.StringType, []attr.Value{types.StringValue("new")}))
		if !model.LiteralValue.Equal(want) {
			t.Fatalf("got %v, want %v", model.LiteralValue, want)
		}
	})
	t.Run("import_infers_tuple", func(t *testing.T) {
		var model DeploymentVariableValueResourceModel
		if d := setValueOnModel(ctx, &model, wire); d.HasError() {
			t.Fatal(d)
		}
		want := types.DynamicValue(types.TupleValueMust([]attr.Type{types.StringType}, []attr.Value{types.StringValue("new")}))
		if !model.LiteralValue.Equal(want) {
			t.Fatalf("got %v, want %v", model.LiteralValue, want)
		}
	})
	t.Run("conversion_error_preserves_state", func(t *testing.T) {
		before := types.DynamicValue(types.ListValueMust(types.StringType, nil))
		model := DeploymentVariableValueResourceModel{LiteralValue: before}
		if d := setValueOnModel(ctx, &model, structpb.NewStringValue("invalid")); !d.HasError() {
			t.Fatal("expected diagnostic")
		}
		if !model.LiteralValue.Equal(before) {
			t.Fatal("conversion error overwrote literal state")
		}
	})
}
