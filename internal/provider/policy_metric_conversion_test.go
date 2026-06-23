// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"reflect"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// TestPolicyVerificationMetricRoundTrip locks the verification-metric JSON
// contract carried in *structpb.Struct (the engine wire shape is inferred from
// the old OpenAPI model, so a round-trip test is the cheapest guard against an
// encoding regression). Covers both provider kinds.
func TestPolicyVerificationMetricRoundTrip(t *testing.T) {
	t.Parallel()

	queries, d := types.MapValue(types.StringType, map[string]attr.Value{
		"latency": types.StringValue("avg:trace.latency{*}"),
	})
	if d.HasError() {
		t.Fatalf("build queries map: %v", d)
	}

	cases := []struct {
		name string
		in   PolicyVerificationMetric
	}{
		{
			name: "sleep",
			in: PolicyVerificationMetric{
				Name:     types.StringValue("soak"),
				Interval: types.StringValue("30s"),
				Count:    types.Int64Value(3),
				Success:  &PolicyVerificationCondition{Condition: types.StringValue("result < 0.05"), Threshold: types.Int64Null()},
				Sleep:    &PolicySleepProvider{DurationSeconds: types.Int64Value(30)},
			},
		},
		{
			name: "datadog",
			in: PolicyVerificationMetric{
				Name:     types.StringValue("latency"),
				Interval: types.StringValue("1m0s"),
				Count:    types.Int64Value(5),
				Success:  &PolicyVerificationCondition{Condition: types.StringValue("result < 0.1"), Threshold: types.Int64Value(2)},
				Failure:  &PolicyVerificationCondition{Condition: types.StringValue("result > 0.5"), Threshold: types.Int64Value(1)},
				Datadog: &PolicyDatadogProvider{
					Site:       types.StringNull(),
					Interval:   types.StringNull(),
					Queries:    queries,
					ApiKey:     types.StringValue("ak"),
					AppKey:     types.StringValue("app"),
					Aggregator: types.StringNull(),
					Formula:    types.StringNull(),
				},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			st, err := policyVerificationMetricStruct(tc.in)
			if err != nil {
				t.Fatalf("policyVerificationMetricStruct: %v", err)
			}
			out, err := policyVerificationMetricToModel(st)
			if err != nil {
				t.Fatalf("policyVerificationMetricToModel: %v", err)
			}
			if !reflect.DeepEqual(out, tc.in) {
				t.Errorf("round-trip mismatch:\n got  %#v\n want %#v", out, tc.in)
			}
		})
	}
}

func TestPolicyVerificationMetricRequiresProvider(t *testing.T) {
	t.Parallel()
	_, err := policyVerificationMetricStruct(PolicyVerificationMetric{
		Name:     types.StringValue("x"),
		Interval: types.StringValue("30s"),
		Count:    types.Int64Value(1),
		Success:  &PolicyVerificationCondition{Condition: types.StringValue("ok"), Threshold: types.Int64Null()},
	})
	if err == nil {
		t.Fatal("expected error when no provider block is set")
	}
}
