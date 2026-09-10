// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"errors"
	"strings"
	"testing"

	apiv1connect "buf.build/gen/go/orca/orca/connectrpc/go/orca/api/v1/apiv1connect"
	apiv1 "buf.build/gen/go/orca/orca/protocolbuffers/go/orca/api/v1"
	connect "connectrpc.com/connect"
	"github.com/ctrlplanedev/terraform-provider-ctrlplane/internal/api"
	"github.com/google/uuid"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type deploymentVariableLookupClient struct {
	apiv1connect.DeploymentServiceClient
	list func(*apiv1.ListDeploymentVariablesRequest) (*apiv1.ListDeploymentVariablesResponse, error)
}

func (c deploymentVariableLookupClient) ListDeploymentVariables(ctx context.Context, req *connect.Request[apiv1.ListDeploymentVariablesRequest]) (*connect.Response[apiv1.ListDeploymentVariablesResponse], error) {
	msg, err := c.list(req.Msg)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(msg), nil
}

func TestDeploymentVariableDataSourceRead(t *testing.T) {
	description := "Container image tag"
	target := &apiv1.DeploymentVariable{Id: "variable-id", DeploymentId: "deployment-id", Key: "IMAGE_TAG", Description: description}
	for _, tc := range []struct {
		name      string
		pages     [][]*apiv1.DeploymentVariable
		err       error
		wantError string
		wantCalls int
	}{
		{name: "found", pages: [][]*apiv1.DeploymentVariable{{target}}, wantCalls: 1},
		{name: "later page exact match", pages: [][]*apiv1.DeploymentVariable{{{Id: "other", Key: "image_tag"}}, {target}}, wantCalls: 2},
		{name: "missing", pages: [][]*apiv1.DeploymentVariable{{{Id: "other", Key: "OTHER"}}}, wantError: "Deployment variable not found", wantCalls: 1},
		{name: "empty", pages: [][]*apiv1.DeploymentVariable{{}}, wantError: "Deployment variable not found", wantCalls: 1},
		{name: "nil variable", pages: [][]*apiv1.DeploymentVariable{{nil}}, wantError: "Deployment variable not found", wantCalls: 1},
		{name: "empty ID", pages: [][]*apiv1.DeploymentVariable{{{Key: "IMAGE_TAG"}}}, wantError: "empty ID", wantCalls: 1},
		{name: "API error", err: connect.NewError(connect.CodePermissionDenied, errors.New("access denied")), wantError: "Failed to read deployment variable", wantCalls: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			calls, offset, total := 0, int32(0), int32(0)
			for _, page := range tc.pages {
				total += int32(len(page))
			}
			workspaceID := uuid.New()
			d := &DeploymentVariableDataSource{workspace: &api.WorkspaceClient{ID: workspaceID, Deployment: deploymentVariableLookupClient{list: func(req *apiv1.ListDeploymentVariablesRequest) (*apiv1.ListDeploymentVariablesResponse, error) {
				if req.WorkspaceId != workspaceID.String() || req.DeploymentId != "deployment-id" || req.Offset != offset || req.Limit != 100 {
					t.Fatalf("unexpected request: %v", req)
				}
				calls++
				if tc.err != nil {
					return nil, tc.err
				}
				if calls > len(tc.pages) {
					t.Fatal("unexpected extra request")
				}
				items := make([]*apiv1.DeploymentVariableWithValues, 0)
				for _, v := range tc.pages[calls-1] {
					items = append(items, &apiv1.DeploymentVariableWithValues{Variable: v})
				}
				offset += int32(len(items))
				return &apiv1.ListDeploymentVariablesResponse{Items: items, Total: total}, nil
			}}}}
			var schemaResp datasource.SchemaResponse
			d.Schema(ctx, datasource.SchemaRequest{}, &schemaResp)
			state := tfsdk.State{Schema: schemaResp.Schema}
			diags := state.Set(ctx, &DeploymentVariableResourceModel{ID: types.StringNull(), DeploymentId: types.StringValue("deployment-id"), Key: types.StringValue("IMAGE_TAG"), Description: types.StringNull()})
			if diags.HasError() {
				t.Fatal(diags)
			}
			resp := datasource.ReadResponse{State: tfsdk.State{Schema: schemaResp.Schema}}
			d.Read(ctx, datasource.ReadRequest{Config: tfsdk.Config{Schema: schemaResp.Schema, Raw: state.Raw}}, &resp)
			if calls != tc.wantCalls {
				t.Fatalf("got %d calls, want %d", calls, tc.wantCalls)
			}
			if tc.wantError != "" {
				if !resp.Diagnostics.HasError() || !strings.Contains(resp.Diagnostics.Errors()[0].Summary()+resp.Diagnostics.Errors()[0].Detail(), tc.wantError) {
					t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
				}
				return
			}
			if resp.Diagnostics.HasError() {
				t.Fatal(resp.Diagnostics)
			}
			var got DeploymentVariableResourceModel
			if diags := resp.State.Get(ctx, &got); diags.HasError() {
				t.Fatal(diags)
			}
			if got.ID.ValueString() != target.Id || got.DeploymentId.ValueString() != target.DeploymentId || got.Key.ValueString() != target.Key || got.Description.ValueString() != description {
				t.Fatalf("unexpected state: %+v", got)
			}
		})
	}
}
