// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"strings"
	"testing"

	frameworkresource "github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestWorkflowJobAgentConfigSchemaSupportsLegacyAndJSON(t *testing.T) {
	t.Parallel()

	var resp frameworkresource.SchemaResponse
	(&WorkflowResource{}).Schema(context.Background(), frameworkresource.SchemaRequest{}, &resp)

	jobAgentBlock, ok := resp.Schema.Blocks["job_agent"].(schema.ListNestedBlock)
	if !ok {
		t.Fatalf("job_agent block = %T, want schema.ListNestedBlock", resp.Schema.Blocks["job_agent"])
	}
	configAttribute := jobAgentBlock.NestedObject.Attributes["config"]
	config, ok := configAttribute.(schema.MapAttribute)
	if !ok {
		t.Fatalf("job_agent.config = %T, want schema.MapAttribute", configAttribute)
	}
	if !config.Optional || config.Required {
		t.Fatalf("job_agent.config optional=%t required=%t, want optional only", config.Optional, config.Required)
	}
	if !config.ElementType.Equal(types.StringType) {
		t.Fatalf("job_agent.config element type = %s, want %s", config.ElementType, types.StringType)
	}

	configJSONAttribute := jobAgentBlock.NestedObject.Attributes["config_json"]
	configJSON, ok := configJSONAttribute.(schema.StringAttribute)
	if !ok {
		t.Fatalf("job_agent.config_json = %T, want schema.StringAttribute", configJSONAttribute)
	}
	if !configJSON.Optional || configJSON.Required {
		t.Fatalf("job_agent.config_json optional=%t required=%t, want optional only", configJSON.Optional, configJSON.Required)
	}
}

func TestWorkflowInputsRoundTrip(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		in   types.String
		want string
	}{
		{"null", types.StringNull(), "[]"},
		{"empty_string", types.StringValue(""), "[]"},
		{"empty_array", types.StringValue("[]"), "[]"},
		{"single_object", types.StringValue(`[{"name":"env"}]`), `[{"name":"env"}]`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			val, err := workflowInputsToValue(tc.in)
			if err != nil {
				t.Fatalf("workflowInputsToValue: %v", err)
			}
			got := workflowInputsFromValue(val)
			if got.ValueString() != tc.want {
				t.Errorf("round-trip = %q, want %q", got.ValueString(), tc.want)
			}
		})
	}
}

func TestWorkflowInputsInvalidJSON(t *testing.T) {
	t.Parallel()
	if _, err := workflowInputsToValue(types.StringValue("{not json")); err == nil {
		t.Fatal("expected error on invalid inputs JSON")
	}
}

func TestWorkflowJobAgentsRoundTrip(t *testing.T) {
	t.Parallel()

	in := []WorkflowJobAgentModel{{
		Name:       types.StringValue("agent-a"),
		Ref:        types.StringValue("ref-1"),
		Config:     mustStringMap(t, map[string]string{"k": "v"}),
		ConfigJSON: types.StringNull(),
		Selector:   types.StringValue("kind == \"k8s\""),
	}}

	val, err := workflowJobAgentsToValue(in)
	if err != nil {
		t.Fatalf("workflowJobAgentsToValue: %v", err)
	}
	out := workflowJobAgentsFromValue(val, in)
	if len(out) != 1 {
		t.Fatalf("got %d agents, want 1", len(out))
	}
	got, want := out[0], in[0]
	if !got.Name.Equal(want.Name) || !got.Ref.Equal(want.Ref) || !got.Selector.Equal(want.Selector) {
		t.Errorf("scalar fields mismatch: got %+v want %+v", got, want)
	}
	if !got.Config.Equal(want.Config) {
		t.Errorf("config mismatch: got %v want %v", got.Config, want.Config)
	}
	if !got.ConfigJSON.IsNull() {
		t.Errorf("config_json = %q, want null", got.ConfigJSON.ValueString())
	}
}

func TestWorkflowJobAgentConfigPreservesNumericWorkflowID(t *testing.T) {
	t.Parallel()

	val, err := workflowJobAgentsToValue([]WorkflowJobAgentModel{{
		Name:       types.StringValue("managed-spec"),
		Ref:        types.StringValue("agent-id"),
		Config:     types.MapNull(types.StringType),
		ConfigJSON: types.StringValue(`{"repo":"deployments","workflowId":321561144}`),
		Selector:   types.StringValue("true"),
	}})
	if err != nil {
		t.Fatalf("workflowJobAgentsToValue: %v", err)
	}

	agents := val.AsInterface().([]any)
	config := agents[0].(map[string]any)["config"].(map[string]any)
	workflowID, ok := config["workflowId"].(float64)
	if !ok {
		t.Fatalf("workflowId = %T(%v), want numeric protobuf value", config["workflowId"], config["workflowId"])
	}
	if workflowID != 321561144 {
		t.Fatalf("workflowId = %v, want 321561144", workflowID)
	}
}

func TestWorkflowJobAgentConfigJSONRoundTrip(t *testing.T) {
	t.Parallel()

	in := []WorkflowJobAgentModel{{
		Name:       types.StringValue("managed-spec"),
		Ref:        types.StringValue("agent-id"),
		Config:     types.MapNull(types.StringType),
		ConfigJSON: types.StringValue(`{"workflowId":321561144,"enabled":true,"repos":["deployments"],"nested":{"retries":3}}`),
		Selector:   types.StringValue("true"),
	}}

	val, err := workflowJobAgentsToValue(in)
	if err != nil {
		t.Fatalf("workflowJobAgentsToValue: %v", err)
	}
	out := workflowJobAgentsFromValue(val, in)
	if len(out) != 1 {
		t.Fatalf("got %d agents, want 1", len(out))
	}
	if !out[0].Config.IsNull() {
		t.Fatalf("config = %v, want null", out[0].Config)
	}
	const want = `{"enabled":true,"nested":{"retries":3},"repos":["deployments"],"workflowId":321561144}`
	if got := out[0].ConfigJSON.ValueString(); got != want {
		t.Fatalf("config_json = %q, want %q", got, want)
	}
}

func TestWorkflowJobAgentConfigJSONRejectsInvalidObjects(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		raw  string
	}{
		{name: "malformed", raw: `{not json`},
		{name: "array", raw: `["not","an","object"]`},
		{name: "scalar", raw: `"not-an-object"`},
		{name: "null", raw: `null`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := workflowJobAgentsToValue([]WorkflowJobAgentModel{{
				Name:       types.StringValue("agent"),
				Ref:        types.StringValue("agent-id"),
				Config:     types.MapNull(types.StringType),
				ConfigJSON: types.StringValue(tc.raw),
				Selector:   types.StringValue("true"),
			}})
			if err == nil {
				t.Fatal("expected config_json to return an error")
			}
			if !strings.Contains(err.Error(), "JSON object") {
				t.Fatalf("error = %q, want JSON object guidance", err)
			}
		})
	}
}

func TestWorkflowJobAgentConfigRequiresExactlyOneRepresentation(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name       string
		config     types.Map
		configJSON types.String
	}{
		{
			name:       "neither",
			config:     types.MapNull(types.StringType),
			configJSON: types.StringNull(),
		},
		{
			name:       "both",
			config:     mustStringMap(t, map[string]string{"repo": "deployments"}),
			configJSON: types.StringValue(`{"workflowId":321561144}`),
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := workflowJobAgentsToValue([]WorkflowJobAgentModel{{
				Name:       types.StringValue("agent"),
				Ref:        types.StringValue("agent-id"),
				Config:     tc.config,
				ConfigJSON: tc.configJSON,
				Selector:   types.StringValue("true"),
			}})
			if err == nil {
				t.Fatal("expected exactly-one validation error")
			}
			if !strings.Contains(err.Error(), "exactly one") {
				t.Fatalf("error = %q, want exactly-one guidance", err)
			}
		})
	}
}

func TestWorkflowJobAgentEmptyConfigRoundTrip(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		in   WorkflowJobAgentModel
	}{
		{
			name: "legacy map",
			in: WorkflowJobAgentModel{
				Name:       types.StringValue("legacy"),
				Ref:        types.StringValue("agent-id"),
				Config:     mustStringMap(t, map[string]string{}),
				ConfigJSON: types.StringNull(),
				Selector:   types.StringValue("true"),
			},
		},
		{
			name: "json object",
			in: WorkflowJobAgentModel{
				Name:       types.StringValue("json"),
				Ref:        types.StringValue("agent-id"),
				Config:     types.MapNull(types.StringType),
				ConfigJSON: types.StringValue(`{}`),
				Selector:   types.StringValue("true"),
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			val, err := workflowJobAgentsToValue([]WorkflowJobAgentModel{tc.in})
			if err != nil {
				t.Fatalf("workflowJobAgentsToValue: %v", err)
			}
			out := workflowJobAgentsFromValue(val, []WorkflowJobAgentModel{tc.in})
			if len(out) != 1 {
				t.Fatalf("got %d agents, want 1", len(out))
			}
			if !out[0].Config.Equal(tc.in.Config) {
				t.Fatalf("config = %v, want %v", out[0].Config, tc.in.Config)
			}
			if !out[0].ConfigJSON.Equal(tc.in.ConfigJSON) {
				t.Fatalf("config_json = %v, want %v", out[0].ConfigJSON, tc.in.ConfigJSON)
			}
		})
	}
}

func TestWorkflowJobAgentImportChoosesRepresentationByValueType(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name       string
		configJSON types.String
		config     types.Map
	}{
		{
			name:       "typed values use config_json",
			configJSON: types.StringValue(`{"workflowId":321561144}`),
			config:     types.MapNull(types.StringType),
		},
		{
			name:       "strings use legacy config",
			configJSON: types.StringNull(),
			config:     mustStringMap(t, map[string]string{"repo": "deployments"}),
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			input := []WorkflowJobAgentModel{{
				Name:       types.StringValue("agent"),
				Ref:        types.StringValue("agent-id"),
				Config:     tc.config,
				ConfigJSON: tc.configJSON,
				Selector:   types.StringValue("true"),
			}}
			val, err := workflowJobAgentsToValue(input)
			if err != nil {
				t.Fatalf("workflowJobAgentsToValue: %v", err)
			}
			out := workflowJobAgentsFromValue(val, nil)
			if len(out) != 1 {
				t.Fatalf("got %d agents, want 1", len(out))
			}
			if !out[0].Config.Equal(tc.config) {
				t.Fatalf("config = %v, want %v", out[0].Config, tc.config)
			}
			if !out[0].ConfigJSON.Equal(tc.configJSON) {
				t.Fatalf("config_json = %v, want %v", out[0].ConfigJSON, tc.configJSON)
			}
		})
	}
}

func TestWorkflowJobAgentsEmpty(t *testing.T) {
	t.Parallel()
	if out := workflowJobAgentsFromValue(nil, nil); len(out) != 0 {
		t.Errorf("nil value should yield empty slice, got %d", len(out))
	}
}

func mustStringMap(t *testing.T, value map[string]string) types.Map {
	t.Helper()
	result, diags := types.MapValueFrom(context.Background(), types.StringType, value)
	if diags.HasError() {
		t.Fatalf("build string map: %v", diags)
	}
	return result
}
