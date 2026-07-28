// Copyright IBM Corp. 2021, 2026

package provider

import (
	"testing"

	"github.com/google/cel-go/cel"
	exprpb "google.golang.org/genproto/googleapis/api/expr/v1alpha1"
	"google.golang.org/protobuf/proto"
)

func TestCELProtoEquivalentIgnoresIDs(t *testing.T) {
	left := mustParseCELExpr(t, `{"key": resource.name}`)
	right := proto.CloneOf(left)

	right.Id += 1000
	entry := right.GetStructExpr().GetEntries()[0]
	entry.Id += 1000
	entry.GetMapKey().Id += 1000
	entry.GetValue().Id += 1000
	entry.GetValue().GetSelectExpr().GetOperand().Id += 1000

	if proto.Equal(left, right) {
		t.Fatal("test setup did not change expression IDs")
	}
	if !celProtoEquivalent(left, right) {
		t.Fatal("celProtoEquivalent() = false, want true for ID-only differences")
	}
}

func mustParseCELExpr(t *testing.T, expression string) *exprpb.Expr {
	t.Helper()
	parsed, issues := celEnv.Parse(expression)
	if issues != nil && issues.Err() != nil {
		t.Fatalf("Parse(%q): %v", expression, issues.Err())
	}
	parsedExpr, err := cel.AstToParsedExpr(parsed)
	if err != nil {
		t.Fatalf("AstToParsedExpr(%q): %v", expression, err)
	}
	return parsedExpr.GetExpr()
}

func TestCelEquivalent(t *testing.T) {
	tests := []struct {
		name  string
		state string
		plan  string
		want  bool
	}{
		{
			// The engine re-serializes the AST fully parenthesized (left-assoc,
			// every node wrapped); the HCL config keeps the author's grouping.
			// Same expression, different text. From the ct_aws_cluster_autoscaler
			// plan diff.
			name:  "engine canonical parens vs author parens",
			state: "((((resource.version == 'ctrlplane.dev/kubernetes/cluster/v1') && ((resource.metadata['kubernetes/status'] == 'running' || resource.metadata['kubernetes/status'] == 'updating'))) && (resource.kind == 'AmazonElasticKubernetesService')) && ('tags/env' in resource.metadata)) && (resource.metadata['tags/env'].startsWith('managed-install'))",
			plan:  "(resource.version == 'ctrlplane.dev/kubernetes/cluster/v1') && ((resource.metadata['kubernetes/status'] == 'running' || resource.metadata['kubernetes/status'] == 'updating')) && resource.kind == 'AmazonElasticKubernetesService' && ('tags/env' in resource.metadata && resource.metadata['tags/env'].startsWith('managed-install'))",
			want:  true,
		},
		{
			// KNOWN LIMITATION: engine factors `(in m && A) || (in m && B)` into
			// `(in m) && (A || B)` — logically equivalent by distribution, but a
			// different AST. Text canonicalization can't see boolean-algebra
			// equality, so this still shows a (non-converging) diff. From
			// ct_wiz_sensor. Fix is either a config rewrite to the engine's form
			// or full semantic equivalence in the deferred refactor.
			name:  "engine factors disjunction - not suppressed",
			state: "(((resource.version == 'ctrlplane.dev/kubernetes/cluster/v1') && ((resource.metadata['kubernetes/status'] == 'running' || resource.metadata['kubernetes/status'] == 'updating'))) && ('tags/env' in resource.metadata)) && ((resource.metadata['tags/env'].startsWith('managed-install')) || (resource.metadata['tags/env'].startsWith('shared-tenancy')))",
			plan:  "(resource.version == 'ctrlplane.dev/kubernetes/cluster/v1') && ((resource.metadata['kubernetes/status'] == 'running' || resource.metadata['kubernetes/status'] == 'updating')) && (('tags/env' in resource.metadata && resource.metadata['tags/env'].startsWith('managed-install')) || ('tags/env' in resource.metadata && resource.metadata['tags/env'].startsWith('shared-tenancy')))",
			want:  false,
		},
		{
			name:  "whitespace only",
			state: "resource.kind == 'a'   &&  resource.name == 'b'",
			plan:  "resource.kind == 'a' && resource.name == 'b'",
			want:  true,
		},
		{
			name:  "different expressions",
			state: "resource.name == 'jungle'",
			plan:  "resource.name == 'zoo-qa'",
			want:  false,
		},
		{
			name:  "unparseable falls back to literal compare - equal",
			state: "false",
			plan:  "false",
			want:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := celEquivalent(tt.state, tt.plan); got != tt.want {
				t.Errorf("celEquivalent(%q, %q) = %v, want %v", tt.state, tt.plan, got, tt.want)
			}
		})
	}
}

func TestPolicyRulesFromModelPreservesCELText(t *testing.T) {
	versionSelector := "version.tag == \"two  spaces\"\n && version.name == 'api'"
	environmentSelector := "environment.name == 'qa'\n || environment.name == 'staging'"
	rules, diags := policyRulesFromModel(PolicyResourceModel{
		VersionSelector: []PolicyVersionSelector{{Selector: celStringValue(versionSelector)}},
		EnvironmentProgression: []PolicyEnvironmentProgression{
			{DependsOnEnvironmentSelector: celStringValue(environmentSelector)},
		},
	})
	if diags.HasError() {
		t.Fatalf("policyRulesFromModel() diagnostics: %v", diags)
	}
	if got := rules[0].GetVersionSelector().GetSelector(); got != versionSelector {
		t.Fatalf("version selector = %q, want %q", got, versionSelector)
	}
	if got := rules[1].GetEnvironmentProgression().GetDependsOnEnvironmentSelector(); got != environmentSelector {
		t.Fatalf("environment selector = %q, want %q", got, environmentSelector)
	}
}
