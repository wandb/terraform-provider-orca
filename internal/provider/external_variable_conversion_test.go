// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestStatsigExternalVariableProviderConfigRoundTrip(t *testing.T) {
	t.Parallel()

	input := ExternalVariableProviderResourceModel{
		Statsig: []ExternalVariableProviderStatsigModel{{
			Endpoint:   types.StringValue("https://api.statsig.com"),
			Token:      types.StringValue(`{{ secret "statsig-server-key" }}`),
			ConsoleURL: types.StringValue("https://console.statsig.com/project-id"),
		}},
	}

	providerType, config, err := externalVariableProviderConfigFromModel(input)
	if err != nil {
		t.Fatalf("externalVariableProviderConfigFromModel: %v", err)
	}
	if providerType != "statsig" {
		t.Fatalf("provider type = %q, want statsig", providerType)
	}

	wantConfig := map[string]any{
		"endpoint":   "https://api.statsig.com",
		"token":      `{{ secret "statsig-server-key" }}`,
		"consoleUrl": "https://console.statsig.com/project-id",
	}
	if !reflect.DeepEqual(config, wantConfig) {
		t.Fatalf("config = %#v, want %#v", config, wantConfig)
	}

	var output ExternalVariableProviderResourceModel
	if err := setExternalVariableProviderBlocksFromAPI(&output, providerType, config); err != nil {
		t.Fatalf("setExternalVariableProviderBlocksFromAPI: %v", err)
	}
	if len(output.Statsig) != 1 {
		t.Fatalf("statsig block count = %d, want 1", len(output.Statsig))
	}
	if output.Statsig[0] != input.Statsig[0] {
		t.Fatalf("statsig block = %#v, want %#v", output.Statsig[0], input.Statsig[0])
	}
}

func TestCustomExternalVariableProviderConfigRoundTrip(t *testing.T) {
	t.Parallel()

	input := ExternalVariableProviderResourceModel{
		Custom: []ExternalVariableProviderCustomModel{{
			Type:   types.StringValue("launchdarkly"),
			Config: types.StringValue(`{"endpoint":"https://example.test","nested":{"enabled":true}}`),
		}},
	}

	providerType, config, err := externalVariableProviderConfigFromModel(input)
	if err != nil {
		t.Fatalf("externalVariableProviderConfigFromModel: %v", err)
	}
	if providerType != "launchdarkly" {
		t.Fatalf("provider type = %q, want launchdarkly", providerType)
	}

	var output ExternalVariableProviderResourceModel
	if err := setExternalVariableProviderBlocksFromAPI(&output, providerType, config); err != nil {
		t.Fatalf("setExternalVariableProviderBlocksFromAPI: %v", err)
	}
	if len(output.Custom) != 1 {
		t.Fatalf("custom block count = %d, want 1", len(output.Custom))
	}
	if output.Custom[0].Type.ValueString() != "launchdarkly" {
		t.Fatalf("custom type = %q, want launchdarkly", output.Custom[0].Type.ValueString())
	}
	assertJSONEqual(t, output.Custom[0].Config.ValueString(), input.Custom[0].Config.ValueString())
}

func TestStatsigDeploymentVariableValueConfigRoundTrip(t *testing.T) {
	t.Parallel()

	input := DeploymentVariableValueResourceModel{
		Statsig: []DeploymentVariableValueStatsigModel{{
			ProviderID:   types.StringValue("provider-id"),
			GateKey:      types.StringValue("enable_new_checkout"),
			UserTemplate: types.StringValue(`{"userID":"{{ .resource.id }}","custom":{"region":"{{ .resource.metadata.region }}"}}`),
		}},
	}

	external, err := externalVariableValueFromModel(input)
	if err != nil {
		t.Fatalf("externalVariableValueFromModel: %v", err)
	}
	if external.GetProviderId() != "provider-id" {
		t.Fatalf("provider id = %q, want provider-id", external.GetProviderId())
	}
	if external.GetConfig().AsMap()["key"] != "enable_new_checkout" {
		t.Fatalf("gate key = %#v, want enable_new_checkout", external.GetConfig().AsMap()["key"])
	}

	var output DeploymentVariableValueResourceModel
	if err := setExternalValueOnModel(&output, external, "statsig"); err != nil {
		t.Fatalf("setExternalValueOnModel: %v", err)
	}
	if len(output.Statsig) != 1 {
		t.Fatalf("statsig block count = %d, want 1", len(output.Statsig))
	}
	if output.Statsig[0].ProviderID.ValueString() != "provider-id" {
		t.Fatalf("provider id = %q, want provider-id", output.Statsig[0].ProviderID.ValueString())
	}
	if output.Statsig[0].GateKey.ValueString() != "enable_new_checkout" {
		t.Fatalf("gate key = %q, want enable_new_checkout", output.Statsig[0].GateKey.ValueString())
	}
	assertJSONEqual(t, output.Statsig[0].UserTemplate.ValueString(), input.Statsig[0].UserTemplate.ValueString())
}

func assertJSONEqual(t *testing.T, got, want string) {
	t.Helper()

	var gotValue, wantValue any
	if err := json.Unmarshal([]byte(got), &gotValue); err != nil {
		t.Fatalf("decode got JSON: %v", err)
	}
	if err := json.Unmarshal([]byte(want), &wantValue); err != nil {
		t.Fatalf("decode want JSON: %v", err)
	}
	if !reflect.DeepEqual(gotValue, wantValue) {
		t.Fatalf("JSON = %#v, want %#v", gotValue, wantValue)
	}
}
