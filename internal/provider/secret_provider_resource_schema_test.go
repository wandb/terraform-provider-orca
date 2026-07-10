// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/defaults"
	frameworkvalidator "github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestSecretProviderResourceSchemaWriteOnlyConfig(t *testing.T) {
	var resp resource.SchemaResponse
	(&SecretProviderResource{}).Schema(context.Background(), resource.SchemaRequest{}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Schema() diagnostics: %v", resp.Diagnostics)
	}
	if diagnostics := resp.Schema.ValidateImplementation(context.Background()); diagnostics.HasError() {
		t.Fatalf("schema implementation diagnostics: %v", diagnostics)
	}

	if _, ok := resp.Schema.Attributes["config"]; ok {
		t.Fatal("legacy config attribute is still present")
	}

	config, ok := resp.Schema.Attributes["config_wo"].(schema.StringAttribute)
	if !ok {
		t.Fatal("config_wo is not a string attribute")
	}
	if !config.Required || !config.Sensitive || !config.WriteOnly || config.Computed || config.Optional {
		t.Fatalf("unexpected config_wo schema: %#v", config)
	}
	if len(config.Validators) != 1 {
		t.Fatalf("config_wo validators = %d, want 1", len(config.Validators))
	}

	const invalidConfig = "credential-marker-not-json"
	var validationResp frameworkvalidator.StringResponse
	config.Validators[0].ValidateString(context.Background(), frameworkvalidator.StringRequest{
		ConfigValue: types.StringValue(invalidConfig),
	}, &validationResp)
	if !validationResp.Diagnostics.HasError() {
		t.Fatal("config_wo accepted invalid JSON")
	}
	for _, diagnostic := range validationResp.Diagnostics {
		if strings.Contains(diagnostic.Detail(), invalidConfig) {
			t.Fatal("config_wo validation diagnostic disclosed the configured value")
		}
	}

	version, ok := resp.Schema.Attributes["config_wo_version"].(schema.Int64Attribute)
	if !ok {
		t.Fatal("config_wo_version is not an int64 attribute")
	}
	if !version.Optional || !version.Computed || version.Required || version.WriteOnly {
		t.Fatalf("unexpected config_wo_version schema: %#v", version)
	}
	if version.Default == nil {
		t.Fatal("config_wo_version has no default")
	}

	var defaultResp defaults.Int64Response
	version.Default.DefaultInt64(context.Background(), defaults.Int64Request{}, &defaultResp)
	if defaultResp.Diagnostics.HasError() {
		t.Fatalf("config_wo_version default diagnostics: %v", defaultResp.Diagnostics)
	}
	if !defaultResp.PlanValue.Equal(types.Int64Value(1)) {
		t.Fatalf("config_wo_version default = %s, want 1", defaultResp.PlanValue)
	}
}
