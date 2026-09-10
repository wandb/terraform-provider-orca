// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"

	apiv1 "buf.build/gen/go/orca/orca/protocolbuffers/go/orca/api/v1"
	connect "connectrpc.com/connect"
	"github.com/ctrlplanedev/terraform-provider-ctrlplane/internal/api"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ datasource.DataSource = &DeploymentVariableDataSource{}
var _ datasource.DataSourceWithConfigure = &DeploymentVariableDataSource{}

// deploymentVariablePageSize is the page size requested from
// ListDeploymentVariables. A page shorter than this is treated as the last one.
const deploymentVariablePageSize int32 = 100

// deploymentVariableMaxPages bounds the pagination loop so a server that
// ignores Offset and omits Total cannot make Read spin forever.
const deploymentVariableMaxPages = 1000

func NewDeploymentVariableDataSource() datasource.DataSource {
	return &DeploymentVariableDataSource{}
}

type DeploymentVariableDataSource struct {
	workspace *api.WorkspaceClient
}

type DeploymentVariableDataSourceModel struct {
	ID           types.String `tfsdk:"id"`
	DeploymentId types.String `tfsdk:"deployment_id"`
	Key          types.String `tfsdk:"key"`
	Description  types.String `tfsdk:"description"`
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
				Validators: []validator.String{stringvalidator.LengthAtLeast(1)},
			},
			"key": schema.StringAttribute{
				Required: true, Description: "The exact variable key to look up",
				Validators: []validator.String{stringvalidator.LengthAtLeast(1)},
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
	var data DeploymentVariableDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	deploymentID := data.DeploymentId.ValueString()
	key := data.Key.ValueString()

	var offset int32
	for page := 0; page < deploymentVariableMaxPages; page++ {
		listed, err := d.workspace.Deployment.ListDeploymentVariables(ctx, connect.NewRequest(&apiv1.ListDeploymentVariablesRequest{
			WorkspaceId:  d.workspace.WorkspaceID(),
			DeploymentId: deploymentID,
			Limit:        deploymentVariablePageSize,
			Offset:       offset,
		}))
		if err != nil {
			if isNotFound(err) {
				resp.Diagnostics.AddError("Deployment not found", fmt.Sprintf(
					"No deployment with ID '%s' in workspace '%s'", deploymentID, d.workspace.WorkspaceID(),
				))
				return
			}
			addConnectError(&resp.Diagnostics, "Failed to read deployment variable", err)
			return
		}
		items := listed.Msg.GetItems()
		for _, item := range items {
			variable := item.GetVariable()
			if variable == nil || variable.GetKey() != key {
				continue
			}
			// Guard against a permissive server that ignores the DeploymentId
			// filter and returns variables from other deployments.
			if id := variable.GetDeploymentId(); id != "" && id != deploymentID {
				continue
			}
			if variable.GetId() == "" {
				resp.Diagnostics.AddError("Failed to read deployment variable", "The matching deployment variable has an empty ID")
				return
			}
			// Only the computed attributes come from the server. deployment_id and
			// key are the user's lookup inputs and are kept as configured, so the
			// resource's applyDeploymentVariable helper is deliberately not used.
			data.ID = types.StringValue(variable.GetId())
			data.Description = optionalString(variable.GetDescription())
			resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
			return
		}
		// Stop on an empty or short page, or once the server says we've reached
		// Total. Servers may omit Total, so the short-page check is what ends
		// the loop in that case.
		if int32(len(items)) < deploymentVariablePageSize {
			break
		}
		offset += int32(len(items))
		if total := listed.Msg.GetTotal(); total > 0 && offset >= total {
			break
		}
	}
	resp.Diagnostics.AddError("Deployment variable not found", fmt.Sprintf(
		"No deployment variable with key '%s' in deployment '%s' in workspace '%s'",
		key, deploymentID, d.workspace.WorkspaceID(),
	))
}
