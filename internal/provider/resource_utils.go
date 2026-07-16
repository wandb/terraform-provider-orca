// Copyright IBM Corp. 2021, 2026

package provider

import (
	"context"

	"github.com/google/cel-go/cel"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// celEnv is used only to parse selector strings into an AST so we can compare
// them structurally. Parsing is purely syntactic and needs no declarations.
var celEnv, _ = cel.NewEnv(cel.EnableMacroCallTracking())

func stringMapPointer(value types.Map) *map[string]string {
	if value.IsNull() || value.IsUnknown() {
		return nil
	}

	var decoded map[string]string
	diags := value.ElementsAs(context.Background(), &decoded, false)
	if diags.HasError() {
		return nil
	}

	return &decoded
}

func stringMapValue(value *map[string]string) types.Map {
	if value == nil {
		return types.MapNull(types.StringType)
	}

	result, _ := types.MapValueFrom(context.Background(), types.StringType, *value)
	return result
}

// celCanonical parses a CEL selector and re-emits it in cel-go's canonical
// form, collapsing the author's parenthesization to a single deterministic
// shape. Returns ("", false) when the string does not parse.
func celCanonical(expr string) (string, bool) {
	ast, iss := celEnv.Parse(expr)
	if iss != nil && iss.Err() != nil {
		return "", false
	}
	out, err := cel.AstToString(ast)
	if err != nil {
		return "", false
	}
	return out, true
}

// celEquivalent reports whether two selector strings are the same expression.
// It compares canonical ASTs so that diffs which differ only in
// parenthesization or whitespace — the engine re-serializes selectors fully
// parenthesized — are treated as equal. It does NOT recognize boolean-algebra
// rewrites (e.g. factoring `(p && a) || (p && b)` into `p && (a || b)`); those
// produce different ASTs and remain a visible diff. When either side fails to
// parse, only exact string equality is accepted.
func celEquivalent(a, b string) bool {
	if a == b {
		return true
	}

	ca, okA := celCanonical(a)
	cb, okB := celCanonical(b)
	return okA && okB && ca == cb
}

// celNormalizedPlanModifier keeps the prior state value when the planned
// config and state are the same CEL expression. The engine re-serializes
// selectors in a canonical, fully-parenthesized form, so the value returned by
// Read differs textually (parentheses, whitespace) from a hand-written config
// even when the expression is identical. Without this, every plan would show a
// spurious in-place update.
type celNormalizedPlanModifier struct{}

func (celNormalizedPlanModifier) Description(_ context.Context) string {
	return "Suppresses diffs when the planned and prior-state CEL are the same expression."
}

func (m celNormalizedPlanModifier) MarkdownDescription(ctx context.Context) string {
	return m.Description(ctx)
}

func (celNormalizedPlanModifier) PlanModifyString(_ context.Context, req planmodifier.StringRequest, resp *planmodifier.StringResponse) {
	if req.StateValue.IsNull() || req.PlanValue.IsUnknown() {
		return
	}
	if celEquivalent(req.PlanValue.ValueString(), req.StateValue.ValueString()) {
		resp.PlanValue = req.StateValue
	}
}

func celNormalized() planmodifier.String {
	return celNormalizedPlanModifier{}
}
