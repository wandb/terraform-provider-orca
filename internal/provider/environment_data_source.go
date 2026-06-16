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

var _ datasource.DataSource = &EnvironmentDataSource{}
var _ datasource.DataSourceWithConfigure = &EnvironmentDataSource{}

func NewEnvironmentDataSource() datasource.DataSource {
	return &EnvironmentDataSource{}
}

type EnvironmentDataSource struct {
	workspace *api.WorkspaceClient
}

type EnvironmentDataSourceModel struct {
	ID               types.String `tfsdk:"id"`
	Name             types.String `tfsdk:"name"`
	Description      types.String `tfsdk:"description"`
	ResourceSelector types.String `tfsdk:"resource_selector"`
	Metadata         types.Map    `tfsdk:"metadata"`
}

func (d *EnvironmentDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_environment"
}

func (d *EnvironmentDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Fetch an existing environment by name within the configured workspace.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:    true,
				Description: "The ID of the environment",
			},
			"name": schema.StringAttribute{
				Required:    true,
				Description: "The name of the environment to look up",
			},
			"description": schema.StringAttribute{
				Computed:    true,
				Description: "The description of the environment",
			},
			"resource_selector": schema.StringAttribute{
				Computed:    true,
				Description: "CEL expression used to select resources",
			},
			"metadata": schema.MapAttribute{
				Computed:    true,
				Description: "The metadata of the environment",
				ElementType: types.StringType,
			},
		},
	}
}

func (d *EnvironmentDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *EnvironmentDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data EnvironmentDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	envResp, err := d.workspace.System.GetEnvironmentByName(ctx, connect.NewRequest(&apiv1.GetEnvironmentByNameRequest{
		WorkspaceId: d.workspace.WorkspaceID(),
		Name:        data.Name.ValueString(),
	}))
	if err != nil {
		if isNotFound(err) {
			resp.Diagnostics.AddError(
				"Environment not found",
				fmt.Sprintf("No environment with name '%s' in workspace '%s'", data.Name.ValueString(), d.workspace.WorkspaceID()),
			)
			return
		}
		addConnectError(&resp.Diagnostics, "Failed to read environment", err)
		return
	}

	env := envResp.Msg
	data.ID = types.StringValue(env.GetId())
	data.Name = types.StringValue(env.GetName())
	data.Description = optionalString(env.GetDescription())
	data.ResourceSelector = optionalString(env.GetResourceSelector())
	data.Metadata = metadataMapValue(env.GetMetadata())

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
