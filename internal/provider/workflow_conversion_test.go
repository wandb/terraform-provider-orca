// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

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
	cfg, d := types.MapValue(types.StringType, map[string]attr.Value{"k": types.StringValue("v")})
	if d.HasError() {
		t.Fatalf("build config map: %v", d)
	}
	in := []WorkflowJobAgentModel{{
		Name:     types.StringValue("agent-a"),
		Ref:      types.StringValue("ref-1"),
		Config:   cfg,
		Selector: types.StringValue("kind == \"k8s\""),
	}}

	val, err := workflowJobAgentsToValue(in)
	if err != nil {
		t.Fatalf("workflowJobAgentsToValue: %v", err)
	}
	out := workflowJobAgentsFromValue(val)
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
}

func TestWorkflowJobAgentsEmpty(t *testing.T) {
	t.Parallel()
	if out := workflowJobAgentsFromValue(nil); len(out) != 0 {
		t.Errorf("nil value should yield empty slice, got %d", len(out))
	}
}
