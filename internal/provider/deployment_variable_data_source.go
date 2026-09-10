// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"

	apiv1 "buf.build/gen/go/orca/orca/protocolbuffers/go/orca/api/v1"
	connect "connectrpc.com/connect"
	"github.com/ctrlplanedev/terraform-provider-ctrlplane/internal/api"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
)

var _ datasource.DataSource = &DeploymentVariableDataSource{}
var _ datasource.DataSourceWithConfigure = &DeploymentVariableDataSource{}

func NewDeploymentVariableDataSource() datasource.DataSource {
	return &DeploymentVariableDataSource{}
}

type DeploymentVariableDataSource struct {
	workspace *api.WorkspaceClient
}

func (d *DeploymentVariableDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_deployment_variable"
}

func (d *DeploymentVariableDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Fetch an existing deployment variable by deployment ID and key within the configured workspace.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed: true, Description: "The ID of the deployment variable",
			},
			"deployment_id": schema.StringAttribute{
				Required: true, Description: "The deployment ID this variable belongs to",
			},
			"key": schema.StringAttribute{
				Required: true, Description: "The exact variable key to look up",
			},
			"description": schema.StringAttribute{
				Computed: true, Description: "The variable description",
			},
		},
	}
}

func (d *DeploymentVariableDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	workspace, ok := req.ProviderData.(*api.WorkspaceClient)
	if !ok {
		resp.Diagnostics.AddError("Invalid provider data", "The provider data is not a *api.WorkspaceClient")
		return
	}
	d.workspace = workspace
}

func (d *DeploymentVariableDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data DeploymentVariableResourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	for offset := int32(0); ; {
		listed, err := d.workspace.Deployment.ListDeploymentVariables(ctx, connect.NewRequest(&apiv1.ListDeploymentVariablesRequest{
			WorkspaceId:  d.workspace.WorkspaceID(),
			DeploymentId: data.DeploymentId.ValueString(),
			Limit:        100,
			Offset:       offset,
		}))
		if err != nil {
			addConnectError(&resp.Diagnostics, "Failed to read deployment variable", err)
			return
		}
		items := listed.Msg.GetItems()
		for _, item := range items {
			variable := item.GetVariable()
			if variable == nil || variable.GetKey() != data.Key.ValueString() {
				continue
			}
			if variable.GetId() == "" {
				resp.Diagnostics.AddError("Failed to read deployment variable", "The matching deployment variable has an empty ID")
				return
			}
			applyDeploymentVariable(&data, variable)
			resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
			return
		}
		offset += int32(len(items))
		if len(items) == 0 || offset >= listed.Msg.GetTotal() {
			break
		}
	}
	resp.Diagnostics.AddError("Deployment variable not found", fmt.Sprintf(
		"No deployment variable with key '%s' in deployment '%s' in workspace '%s'",
		data.Key.ValueString(), data.DeploymentId.ValueString(), d.workspace.WorkspaceID(),
	))
}
