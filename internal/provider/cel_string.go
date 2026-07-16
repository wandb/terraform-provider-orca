// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

var (
	_ basetypes.StringTypable                    = CELStringType{}
	_ basetypes.StringValuableWithSemanticEquals = CELStringValue{}
)

type CELStringType struct{ basetypes.StringType }

func (CELStringType) Equal(other attr.Type) bool {
	_, ok := other.(CELStringType)
	return ok
}

func (CELStringType) String() string { return "provider.CELStringType" }

func (CELStringType) ValueFromString(_ context.Context, value basetypes.StringValue) (basetypes.StringValuable, diag.Diagnostics) {
	return CELStringValue{StringValue: value}, nil
}

func (typ CELStringType) ValueFromTerraform(ctx context.Context, value tftypes.Value) (attr.Value, error) {
	baseValue, err := typ.StringType.ValueFromTerraform(ctx, value)
	if err != nil {
		return nil, err
	}
	stringValue, ok := baseValue.(basetypes.StringValue)
	if !ok {
		return nil, fmt.Errorf("unexpected value type %T", baseValue)
	}
	return CELStringValue{StringValue: stringValue}, nil
}

func (CELStringType) ValueType(context.Context) attr.Value { return CELStringValue{} }

type CELStringValue struct{ basetypes.StringValue }

func (value CELStringValue) Equal(other attr.Value) bool {
	otherValue, ok := other.(CELStringValue)
	return ok && value.StringValue.Equal(otherValue.StringValue)
}

func (value CELStringValue) StringSemanticEquals(ctx context.Context, other basetypes.StringValuable) (bool, diag.Diagnostics) {
	otherValue, diags := other.ToStringValue(ctx)
	if diags.HasError() {
		return false, diags
	}
	return celEquivalent(value.ValueString(), otherValue.ValueString()), diags
}

func (CELStringValue) Type(context.Context) attr.Type { return CELStringType{} }

func celStringValue(value string) CELStringValue {
	return CELStringValue{StringValue: basetypes.NewStringValue(value)}
}

func optionalCELStringValue(value string) CELStringValue {
	if value == "" {
		return CELStringValue{StringValue: basetypes.NewStringNull()}
	}
	return celStringValue(value)
}

func celStringPointer(value CELStringValue) *string {
	if value.IsNull() || value.IsUnknown() {
		return nil
	}
	raw := value.ValueString()
	return &raw
}
