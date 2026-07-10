// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package validator

import (
	"context"
	"strings"
	"testing"

	frameworkvalidator "github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestJSONValidator(t *testing.T) {
	tests := []struct {
		name      string
		value     types.String
		wantError bool
	}{
		{name: "object", value: types.StringValue(`{"project":"production"}`)},
		{name: "array", value: types.StringValue(`["production"]`)},
		{name: "null", value: types.StringNull()},
		{name: "unknown", value: types.StringUnknown()},
		{name: "invalid", value: types.StringValue("credential-marker-not-json"), wantError: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var resp frameworkvalidator.StringResponse
			NewJSONValidator().ValidateString(context.Background(), frameworkvalidator.StringRequest{
				ConfigValue: tt.value,
			}, &resp)

			if got := resp.Diagnostics.HasError(); got != tt.wantError {
				t.Fatalf("HasError() = %v, want %v", got, tt.wantError)
			}
			for _, diagnostic := range resp.Diagnostics {
				if strings.Contains(diagnostic.Detail(), "credential-marker-not-json") {
					t.Fatal("diagnostic disclosed the configured value")
				}
			}
		})
	}
}
