// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	apiv1connect "buf.build/gen/go/orca/orca/connectrpc/go/orca/api/v1/apiv1connect"
	apiv1 "buf.build/gen/go/orca/orca/protocolbuffers/go/orca/api/v1"
	connect "connectrpc.com/connect"
	"github.com/ctrlplanedev/terraform-provider-ctrlplane/internal/api"
	"github.com/google/uuid"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-testing/compare"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/statecheck"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
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
	fullPage := func(n int) []*apiv1.DeploymentVariable {
		page := make([]*apiv1.DeploymentVariable, 0, n)
		for i := 0; i < n; i++ {
			page = append(page, &apiv1.DeploymentVariable{Id: fmt.Sprintf("other-%d", i), DeploymentId: "deployment-id", Key: fmt.Sprintf("OTHER_%d", i)})
		}
		return page
	}
	full := int(deploymentVariablePageSize)
	for _, tc := range []struct {
		name      string
		pages     [][]*apiv1.DeploymentVariable
		omitTotal bool
		// repeatLastPage makes the fake server ignore Offset and keep returning
		// the final page, simulating a server that never advances.
		repeatLastPage bool
		err            error
		wantError      string
		wantCalls      int
	}{
		{name: "found", pages: [][]*apiv1.DeploymentVariable{{target}}, wantCalls: 1},
		{name: "later page exact match", pages: [][]*apiv1.DeploymentVariable{append(fullPage(full-1), &apiv1.DeploymentVariable{Id: "other", DeploymentId: "deployment-id", Key: "image_tag"}), {target}}, wantCalls: 2},
		{name: "later page without total", pages: [][]*apiv1.DeploymentVariable{fullPage(full), {target}}, omitTotal: true, wantCalls: 2},
		{name: "short page ends search without total", pages: [][]*apiv1.DeploymentVariable{{{Id: "other", DeploymentId: "deployment-id", Key: "OTHER"}}}, omitTotal: true, wantError: "Deployment variable not found", wantCalls: 1},
		{name: "full page then empty without total", pages: [][]*apiv1.DeploymentVariable{fullPage(full), {}}, omitTotal: true, wantError: "Deployment variable not found", wantCalls: 2},
		{name: "full page with total ends search", pages: [][]*apiv1.DeploymentVariable{fullPage(full)}, wantError: "Deployment variable not found", wantCalls: 1},
		{name: "server ignores offset is bounded", pages: [][]*apiv1.DeploymentVariable{fullPage(full)}, omitTotal: true, repeatLastPage: true, wantError: "Deployment variable not found", wantCalls: deploymentVariableMaxPages},
		{name: "server omits deployment ID", pages: [][]*apiv1.DeploymentVariable{{{Id: "variable-id", Key: "IMAGE_TAG", Description: description}}}, wantCalls: 1},
		{name: "other deployment same key is skipped", pages: [][]*apiv1.DeploymentVariable{{{Id: "foreign", DeploymentId: "other-deployment", Key: "IMAGE_TAG"}, target}}, wantCalls: 1},
		{name: "only other deployment matches", pages: [][]*apiv1.DeploymentVariable{{{Id: "foreign", DeploymentId: "other-deployment", Key: "IMAGE_TAG"}}}, wantError: "Deployment variable not found", wantCalls: 1},
		{name: "missing", pages: [][]*apiv1.DeploymentVariable{{{Id: "other", DeploymentId: "deployment-id", Key: "OTHER"}}}, wantError: "Deployment variable not found", wantCalls: 1},
		{name: "empty", pages: [][]*apiv1.DeploymentVariable{{}}, wantError: "Deployment variable not found", wantCalls: 1},
		{name: "nil variable", pages: [][]*apiv1.DeploymentVariable{{nil}}, wantError: "Deployment variable not found", wantCalls: 1},
		{name: "empty ID", pages: [][]*apiv1.DeploymentVariable{{{DeploymentId: "deployment-id", Key: "IMAGE_TAG"}}}, wantError: "empty ID", wantCalls: 1},
		{name: "deployment not found", err: connect.NewError(connect.CodeNotFound, errors.New("no such deployment")), wantError: "Deployment not found", wantCalls: 1},
		{name: "API error", err: connect.NewError(connect.CodePermissionDenied, errors.New("access denied")), wantError: "Failed to read deployment variable", wantCalls: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			calls, offset, total := 0, int32(0), int32(0)
			for _, page := range tc.pages {
				total += int32(len(page))
			}
			if tc.omitTotal {
				total = 0
			}
			workspaceID := uuid.New()
			d := &DeploymentVariableDataSource{workspace: &api.WorkspaceClient{ID: workspaceID, Deployment: deploymentVariableLookupClient{list: func(req *apiv1.ListDeploymentVariablesRequest) (*apiv1.ListDeploymentVariablesResponse, error) {
				if req.WorkspaceId != workspaceID.String() || req.DeploymentId != "deployment-id" || req.Offset != offset || req.Limit != deploymentVariablePageSize {
					t.Fatalf("unexpected request: %v", req)
				}
				calls++
				if tc.err != nil {
					return nil, tc.err
				}
				pageIndex := calls - 1
				if tc.repeatLastPage && pageIndex >= len(tc.pages) {
					pageIndex = len(tc.pages) - 1
				}
				if pageIndex >= len(tc.pages) {
					t.Fatal("unexpected extra request")
				}
				items := make([]*apiv1.DeploymentVariableWithValues, 0)
				for _, v := range tc.pages[pageIndex] {
					items = append(items, &apiv1.DeploymentVariableWithValues{Variable: v})
				}
				offset += int32(len(items))
				return &apiv1.ListDeploymentVariablesResponse{Items: items, Total: total}, nil
			}}}}
			var schemaResp datasource.SchemaResponse
			d.Schema(ctx, datasource.SchemaRequest{}, &schemaResp)
			state := tfsdk.State{Schema: schemaResp.Schema}
			diags := state.Set(ctx, &DeploymentVariableDataSourceModel{ID: types.StringNull(), DeploymentId: types.StringValue("deployment-id"), Key: types.StringValue("IMAGE_TAG"), Description: types.StringNull()})
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
			var got DeploymentVariableDataSourceModel
			if diags := resp.State.Get(ctx, &got); diags.HasError() {
				t.Fatal(diags)
			}
			if got.ID.ValueString() != target.Id || got.DeploymentId.ValueString() != target.DeploymentId || got.Key.ValueString() != target.Key || got.Description.ValueString() != description {
				t.Fatalf("unexpected state: %+v", got)
			}
		})
	}
}

func TestDeploymentVariableDataSourceRejectsEmptyInputs(t *testing.T) {
	ctx := context.Background()
	var schemaResp datasource.SchemaResponse
	(&DeploymentVariableDataSource{}).Schema(ctx, datasource.SchemaRequest{}, &schemaResp)
	for _, name := range []string{"deployment_id", "key"} {
		t.Run(name, func(t *testing.T) {
			attr, ok := schemaResp.Schema.Attributes[name].(schema.StringAttribute)
			if !ok {
				t.Fatalf("%s is not a string attribute", name)
			}
			for _, value := range []string{"", "x"} {
				var resp validator.StringResponse
				for _, v := range attr.Validators {
					v.ValidateString(ctx, validator.StringRequest{Path: path.Root(name), ConfigValue: types.StringValue(value)}, &resp)
				}
				if got, want := resp.Diagnostics.HasError(), value == ""; got != want {
					t.Fatalf("value %q: got error=%v, want %v: %v", value, got, want, resp.Diagnostics)
				}
			}
		})
	}
}

func TestAccDeploymentVariableDataSource(t *testing.T) {
	name := fmt.Sprintf("tf-acc-vards-%d", time.Now().UnixNano())

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccDeploymentVariableDataSourceConfig(name),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.CompareValuePairs(
						"ctrlplane_deployment_variable.test", tfjsonpath.New("id"),
						"data.ctrlplane_deployment_variable.test", tfjsonpath.New("id"),
						compare.ValuesSame(),
					),
					statecheck.ExpectKnownValue(
						"data.ctrlplane_deployment_variable.test",
						tfjsonpath.New("key"),
						knownvalue.StringExact(name),
					),
					statecheck.ExpectKnownValue(
						"data.ctrlplane_deployment_variable.test",
						tfjsonpath.New("description"),
						knownvalue.StringExact("Terraform acceptance test variable"),
					),
				},
			},
		},
	})
}

func testAccDeploymentVariableDataSourceConfig(key string) string {
	return fmt.Sprintf(`
%s
resource "ctrlplane_system" "test" {
  name = %q
}

resource "ctrlplane_deployment" "test" {
  name              = %q
  resource_selector = "resource.name == '%s'"
}

resource "ctrlplane_deployment_variable" "test" {
  deployment_id = ctrlplane_deployment.test.id
  key           = %q
  description   = "Terraform acceptance test variable"
}

data "ctrlplane_deployment_variable" "test" {
  deployment_id = ctrlplane_deployment.test.id
  key           = ctrlplane_deployment_variable.test.key
}
`, testAccProviderConfig(), key, key+"-deployment", key, key)
}
