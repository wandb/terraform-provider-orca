// Copyright IBM Corp. 2021, 2026

package provider

import (
	"context"

	"github.com/google/cel-go/cel"
	celast "github.com/google/cel-go/common/ast"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	exprpb "google.golang.org/genproto/googleapis/api/expr/v1alpha1"
	"google.golang.org/protobuf/proto"
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

// parsedCELExpr parses a CEL selector and returns only its expression tree,
// excluding source positions and other parse metadata.
func parsedCELExpr(expression string) (*exprpb.Expr, bool) {
	parsed, issues := celEnv.Parse(expression)
	if issues != nil && issues.Err() != nil {
		return nil, false
	}
	parsedExpr, err := cel.AstToParsedExpr(parsed)
	if err != nil {
		return nil, false
	}
	return parsedExpr.GetExpr(), true
}

// celCanonical re-emits a CEL selector in cel-go's stable text form. It is a
// fallback for equivalent logical expressions that Ctrlplane reassociates into
// a different tree shape.
func celCanonical(expression string) (string, bool) {
	parsed, issues := celEnv.Parse(expression)
	if issues != nil && issues.Err() != nil {
		return "", false
	}
	canonical, err := cel.AstToString(parsed)
	return canonical, err == nil
}

// celProtoEquivalent compares parsed CEL expression trees without their node
// IDs, which cel-go does not guarantee to keep stable between compilations.
func celProtoEquivalent(left, right *exprpb.Expr) bool {
	normalize := func(expression *exprpb.Expr) (*exprpb.Expr, bool) {
		native, err := celast.ProtoToExpr(expression)
		if err != nil {
			return nil, false
		}
		native.RenumberIDs(func(int64) int64 { return 0 })
		normalized, err := celast.ExprToProto(native)
		return normalized, err == nil
	}

	normalizedLeft, okLeft := normalize(left)
	normalizedRight, okRight := normalize(right)
	return okLeft && okRight && proto.Equal(normalizedLeft, normalizedRight)
}

// celEquivalent reports whether two selector strings are the same expression.
// It compares parsed expression trees so that diffs which differ only in
// parenthesization or whitespace — the engine re-serializes selectors fully
// parenthesized — are treated as equal. It does NOT recognize boolean-algebra
// rewrites (e.g. factoring `(p && a) || (p && b)` into `p && (a || b)`); those
// produce different ASTs and remain a visible diff. When either side fails to
// parse, only exact string equality is accepted.
func celEquivalent(a, b string) bool {
	if a == b {
		return true
	}

	left, okLeft := parsedCELExpr(a)
	right, okRight := parsedCELExpr(b)
	if !okLeft || !okRight {
		return false
	}
	if celProtoEquivalent(left, right) {
		return true
	}

	canonicalLeft, okLeft := celCanonical(a)
	canonicalRight, okRight := celCanonical(b)
	return okLeft && okRight && canonicalLeft == canonicalRight
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
