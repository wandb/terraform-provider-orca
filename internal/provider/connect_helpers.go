// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"time"

	connect "connectrpc.com/connect"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// metadataMapValue converts a proto metadata map into a Terraform map, yielding
// an empty (non-null) map when the input is nil. The metadata attributes are
// Computed with an empty-map default, and a proto getter returns nil for an
// absent map — which types.MapValueFrom would turn into a null map, breaking
// the "was empty, now null" consistency check after apply. Coercing nil to an
// empty map keeps server responses consistent with the schema default.
func metadataMapValue(m map[string]string) types.Map {
	if m == nil {
		m = map[string]string{}
	}
	v, _ := types.MapValueFrom(context.Background(), types.StringType, m)
	return v
}

// optionalStringPtr returns a pointer to the string value, or nil when the
// value is null OR unknown. Unlike types.String.ValueStringPointer (which maps
// an unknown value to a pointer to ""), this omits the field entirely for a
// Computed attribute that isn't known yet at apply — sending "" would be
// rejected by fields that validate non-empty input (e.g. a slug).
func optionalStringPtr(s types.String) *string {
	if s.IsNull() || s.IsUnknown() {
		return nil
	}
	v := s.ValueString()
	return &v
}

// optionalString maps a plain proto string to a Terraform value, treating the
// empty string as null. Proto3 scalar strings are never nil, so an absent
// optional field arrives as "" — preserving null avoids spurious drift against
// schemas where the attribute is Optional without a default.
func optionalString(s string) types.String {
	if s == "" {
		return types.StringNull()
	}
	return types.StringValue(s)
}

// optionalSelector maps a proto selector string to a Terraform value, treating
// both "" and the literal "false" as null. The API stores an absent
// job_agent_selector as the constant-false sentinel "false" (body.jobAgentSelector
// ?? "false"), so a deployment created with no selector reads back "false". Mapping
// that to null keeps the post-apply read consistent with a null config — otherwise
// Terraform reports "inconsistent result after apply" (was null, now "false").
func optionalSelector(s string) types.String {
	if s == "" || s == "false" {
		return types.StringNull()
	}
	return types.StringValue(s)
}

// isNotFound reports whether err is a Connect NotFound error, which on a Read
// means the remote object no longer exists and the resource should be removed
// from state rather than producing an error.
func isNotFound(err error) bool {
	return connect.CodeOf(err) == connect.CodeNotFound
}

// connectErrSummary maps a Connect error code to a short diagnostic summary.
func connectErrSummary(err error) string {
	switch connect.CodeOf(err) {
	case connect.CodeInvalidArgument:
		return "Invalid argument"
	case connect.CodeAlreadyExists:
		return "Already exists"
	case connect.CodePermissionDenied:
		return "Permission denied"
	case connect.CodeUnauthenticated:
		return "Authentication failed (check API key)"
	case connect.CodeNotFound:
		return "Not found"
	default:
		return "API error"
	}
}

// addConnectError appends a diagnostic for a failed Connect call, classifying
// it by connect.Code so the user gets an actionable summary.
func addConnectError(diags *diag.Diagnostics, action string, err error) {
	diags.AddError(fmt.Sprintf("%s: %s", action, connectErrSummary(err)), err.Error())
}

// rfc3339 renders a protobuf timestamp as an RFC3339 string, or "" when nil.
func rfc3339(ts *timestamppb.Timestamp) string {
	if ts == nil {
		return ""
	}
	return ts.AsTime().Format(time.RFC3339)
}
