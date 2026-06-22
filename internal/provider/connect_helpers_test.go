// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"errors"
	"testing"
	"time"

	connect "connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestOptionalString(t *testing.T) {
	t.Parallel()
	if got := optionalString(""); !got.IsNull() {
		t.Errorf("optionalString(\"\") = %v, want null", got)
	}
	if got := optionalString("x"); got.IsNull() || got.ValueString() != "x" {
		t.Errorf("optionalString(\"x\") = %v, want \"x\"", got)
	}
}

func TestOptionalSelector(t *testing.T) {
	t.Parallel()
	// The API stores an absent job_agent_selector as the literal "false";
	// both "" and "false" must map to null so a null config round-trips.
	if got := optionalSelector(""); !got.IsNull() {
		t.Errorf("optionalSelector(\"\") = %v, want null", got)
	}
	if got := optionalSelector("false"); !got.IsNull() {
		t.Errorf("optionalSelector(\"false\") = %v, want null", got)
	}
	if got := optionalSelector("resource.kind == 'k8s'"); got.IsNull() || got.ValueString() != "resource.kind == 'k8s'" {
		t.Errorf("optionalSelector(real) = %v, want passthrough", got)
	}
}

func TestRFC3339(t *testing.T) {
	t.Parallel()
	if got := rfc3339(nil); got != "" {
		t.Errorf("rfc3339(nil) = %q, want \"\"", got)
	}
	ts := timestamppb.New(time.Date(2026, 6, 13, 4, 5, 6, 0, time.UTC))
	if got := rfc3339(ts); got != "2026-06-13T04:05:06Z" {
		t.Errorf("rfc3339 = %q, want RFC3339 UTC", got)
	}
}

func TestIsNotFoundAndSummary(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name     string
		err      error
		notFound bool
		summary  string
	}{
		{"not_found", connect.NewError(connect.CodeNotFound, errors.New("x")), true, "Not found"},
		{"invalid_arg", connect.NewError(connect.CodeInvalidArgument, errors.New("x")), false, "Invalid argument"},
		{"already_exists", connect.NewError(connect.CodeAlreadyExists, errors.New("x")), false, "Already exists"},
		{"perm_denied", connect.NewError(connect.CodePermissionDenied, errors.New("x")), false, "Permission denied"},
		{"unauthenticated", connect.NewError(connect.CodeUnauthenticated, errors.New("x")), false, "Authentication failed (check API key)"},
		{"plain_error", errors.New("boom"), false, "API error"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isNotFound(tc.err); got != tc.notFound {
				t.Errorf("isNotFound = %v, want %v", got, tc.notFound)
			}
			if got := connectErrSummary(tc.err); got != tc.summary {
				t.Errorf("connectErrSummary = %q, want %q", got, tc.summary)
			}
		})
	}
}
