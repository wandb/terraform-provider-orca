// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package validator

import (
	"context"
	"encoding/json"

	frameworkvalidator "github.com/hashicorp/terraform-plugin-framework/schema/validator"
)

var _ frameworkvalidator.String = &JSONValidator{}

type JSONValidator struct{}

func NewJSONValidator() frameworkvalidator.String {
	return &JSONValidator{}
}

func (v *JSONValidator) Description(context.Context) string {
	return "must be valid JSON"
}

func (v *JSONValidator) MarkdownDescription(context.Context) string {
	return "must be valid JSON"
}

func (v *JSONValidator) ValidateString(_ context.Context, req frameworkvalidator.StringRequest, resp *frameworkvalidator.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}

	if !json.Valid([]byte(req.ConfigValue.ValueString())) {
		resp.Diagnostics.AddError("Invalid JSON", "The configured value must be valid JSON.")
	}
}
