// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

func TestCELStringValueSemanticEquals(t *testing.T) {
	tests := []struct {
		name, left, right string
		want              bool
	}{
		{"multiline", "resource.kind == 'service'\n && resource.name == 'api'", "resource.kind == 'service' && resource.name == 'api'", true},
		{"server parentheses", "resource.kind == 'service' && resource.name == 'api'", "(resource.kind == 'service') && (resource.name == 'api')", true},
		{"different", "resource.name == 'api'", "resource.name == 'worker'", false},
		{"unparsable fallback", "resource.  missing", "resource. missing", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, diags := celStringValue(tt.left).StringSemanticEquals(context.Background(), celStringValue(tt.right))
			if diags.HasError() {
				t.Fatalf("StringSemanticEquals() diagnostics: %v", diags)
			}
			if got != tt.want {
				t.Fatalf("StringSemanticEquals() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCELStringTypeValueConversion(t *testing.T) {
	typ := CELStringType{}
	value, err := typ.ValueFromTerraform(context.Background(), tftypes.NewValue(tftypes.String, "resource.name == 'api'"))
	if err != nil {
		t.Fatalf("ValueFromTerraform() error: %v", err)
	}
	celValue, ok := value.(CELStringValue)
	if !ok || celValue.ValueString() != "resource.name == 'api'" {
		t.Fatalf("ValueFromTerraform() = %#v (%T)", value, value)
	}
	nullValue, err := typ.ValueFromTerraform(context.Background(), tftypes.NewValue(tftypes.String, nil))
	if err != nil || !nullValue.(CELStringValue).IsNull() {
		t.Fatalf("null ValueFromTerraform() = %#v, %v", nullValue, err)
	}
	if _, ok := typ.ValueType(context.Background()).(CELStringValue); !ok {
		t.Fatalf("ValueType() = %T", typ.ValueType(context.Background()))
	}
	if !typ.Equal(CELStringType{}) || typ.Equal(basetypes.StringType{}) {
		t.Fatal("CELStringType.Equal() did not distinguish the custom type")
	}
	fromString, diags := typ.ValueFromString(context.Background(), basetypes.NewStringValue("true"))
	if diags.HasError() {
		t.Fatalf("ValueFromString() diagnostics: %v", diags)
	}
	directValue, ok := fromString.(CELStringValue)
	if !ok || !directValue.Equal(celStringValue("true")) {
		t.Fatalf("ValueFromString() = %#v (%T)", fromString, fromString)
	}
	if _, ok := directValue.Type(context.Background()).(CELStringType); !ok {
		t.Fatalf("CELStringValue.Type() = %T", directValue.Type(context.Background()))
	}
}

func TestCELStringHelpersPreserveRawText(t *testing.T) {
	raw := "resource.name == \"two  spaces\"\n && resource.kind == 'service'"
	value := celStringValue(raw)
	if value.ValueString() != raw {
		t.Fatalf("celStringValue() = %q, want %q", value.ValueString(), raw)
	}
	if got := celStringPointer(value); got == nil || *got != raw {
		t.Fatalf("celStringPointer() = %v, want %q", got, raw)
	}
	if got := celStringPointer(optionalCELStringValue("")); got != nil {
		t.Fatalf("celStringPointer(null) = %v, want nil", got)
	}
	unknown := CELStringValue{StringValue: basetypes.NewStringUnknown()}
	if got := celStringPointer(unknown); got != nil {
		t.Fatalf("celStringPointer(unknown) = %v, want nil", got)
	}
}
