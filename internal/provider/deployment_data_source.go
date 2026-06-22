// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"

	apiv1 "buf.build/gen/go/ctrlplane/ctrlplane/protocolbuffers/go/ctrlplane/api/v1"
	connect "connectrpc.com/connect"
	"github.com/ctrlplanedev/terraform-provider-ctrlplane/internal/api"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ datasource.DataSource = &DeploymentDataSource{}
var _ datasource.DataSourceWithConfigure = &DeploymentDataSource{}

func NewDeploymentDataSource() datasource.DataSource {
	return &DeploymentDataSource{}
}

type DeploymentDataSource struct {
	workspace *api.WorkspaceClient
}

type DeploymentDataSourceModel struct {
	ID               types.String `tfsdk:"id"`
	Name             types.String `tfsdk:"name"`
	Slug             types.String `tfsdk:"slug"`
	Description      types.String `tfsdk:"description"`
	ResourceSelector types.String `tfsdk:"resource_selector"`
	JobAgentSelector types.String `tfsdk:"job_agent_selector"`
	Metadata         types.Map    `tfsdk:"metadata"`
}

func (d *DeploymentDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_deployment"
}

func (d *DeploymentDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Fetch an existing deployment by name within the configured workspace.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:    true,
				Description: "The ID of the deployment",
			},
			"name": schema.StringAttribute{
				Required:    true,
				Description: "The name of the deployment to look up",
			},
			"slug": schema.StringAttribute{
				Computed:    true,
				Description: "The slug of the deployment",
			},
			"description": schema.StringAttribute{
				Computed:    true,
				Description: "The description of the deployment",
			},
			"resource_selector": schema.StringAttribute{
				Computed:    true,
				Description: "CEL expression used to select resources",
			},
			"job_agent_selector": schema.StringAttribute{
				Computed:    true,
				Description: "CEL expression used to match a job agent",
			},
			"metadata": schema.MapAttribute{
				Computed:    true,
				Description: "The metadata of the deployment",
				ElementType: types.StringType,
			},
		},
	}
}

func (d *DeploymentDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *DeploymentDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data DeploymentDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	depResp, err := d.workspace.Deployment.GetDeploymentByName(ctx, connect.NewRequest(&apiv1.GetDeploymentByNameRequest{
		WorkspaceId: d.workspace.WorkspaceID(),
		Name:        data.Name.ValueString(),
	}))
	if err != nil {
		if isNotFound(err) {
			resp.Diagnostics.AddError(
				"Deployment not found",
				fmt.Sprintf("No deployment with name '%s' in workspace '%s'", data.Name.ValueString(), d.workspace.WorkspaceID()),
			)
			return
		}
		addConnectError(&resp.Diagnostics, "Failed to read deployment", err)
		return
	}

	dep := depResp.Msg.GetDeployment()
	if dep == nil {
		resp.Diagnostics.AddError(
			"Deployment not found",
			fmt.Sprintf("No deployment with name '%s' in workspace '%s'", data.Name.ValueString(), d.workspace.WorkspaceID()),
		)
		return
	}

	data.ID = types.StringValue(dep.GetId())
	data.Name = types.StringValue(dep.GetName())
	data.Slug = types.StringValue(dep.GetSlug())
	data.Description = optionalString(dep.GetDescription())
	data.Metadata = metadataMapValue(dep.GetMetadata())
	data.ResourceSelector = optionalString(dep.GetResourceSelector())
	data.JobAgentSelector = optionalSelector(dep.GetJobAgentSelector())

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
