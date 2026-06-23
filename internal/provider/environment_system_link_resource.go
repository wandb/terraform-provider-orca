// Copyright IBM Corp. 2021, 2026

package provider

import (
	"context"
	"strings"

	apiv1 "buf.build/gen/go/ctrlplane/ctrlplane/protocolbuffers/go/ctrlplane/api/v1"
	connect "connectrpc.com/connect"
	"github.com/ctrlplanedev/terraform-provider-ctrlplane/internal/api"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ resource.Resource = &EnvironmentSystemLinkResource{}
var _ resource.ResourceWithImportState = &EnvironmentSystemLinkResource{}
var _ resource.ResourceWithConfigure = &EnvironmentSystemLinkResource{}

func NewEnvironmentSystemLinkResource() resource.Resource {
	return &EnvironmentSystemLinkResource{}
}

type EnvironmentSystemLinkResource struct {
	workspace *api.WorkspaceClient
}

type EnvironmentSystemLinkResourceModel struct {
	ID            types.String `tfsdk:"id"`
	SystemID      types.String `tfsdk:"system_id"`
	EnvironmentID types.String `tfsdk:"environment_id"`
}

func (r *EnvironmentSystemLinkResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_environment_system_link"
}

func (r *EnvironmentSystemLinkResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts := strings.SplitN(req.ID, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		resp.Diagnostics.AddError(
			"Invalid import ID",
			"Import ID must be in the format: system_id/environment_id",
		)
		return
	}

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("system_id"), parts[0])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("environment_id"), parts[1])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
}

func (r *EnvironmentSystemLinkResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	workspace, ok := req.ProviderData.(*api.WorkspaceClient)
	if !ok {
		resp.Diagnostics.AddError("Invalid provider data", "The provider data is not a *api.WorkspaceClient")
		return
	}

	r.workspace = workspace
}

func (r *EnvironmentSystemLinkResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Links an environment to a system in Ctrlplane. An environment can be linked to multiple systems.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:    true,
				Description: "Composite identifier in the format system_id/environment_id",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"system_id": schema.StringAttribute{
				Required:    true,
				Description: "The ID of the system to link the environment to",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"environment_id": schema.StringAttribute{
				Required:    true,
				Description: "The ID of the environment to link",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
		},
	}
}

func (r *EnvironmentSystemLinkResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data EnvironmentSystemLinkResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	systemID := data.SystemID.ValueString()
	environmentID := data.EnvironmentID.ValueString()

	_, err := r.workspace.System.LinkEnvironmentToSystem(ctx, connect.NewRequest(&apiv1.EnvironmentSystemLinkRequest{
		WorkspaceId:   r.workspace.WorkspaceID(),
		SystemId:      systemID,
		EnvironmentId: environmentID,
	}))
	if err != nil {
		addConnectError(&resp.Diagnostics, "Failed to link environment to system", err)
		return
	}

	data.ID = types.StringValue(systemID + "/" + environmentID)

	resp.Diagnostics.Append(resp.State.Set(ctx, data)...)
}

func (r *EnvironmentSystemLinkResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data EnvironmentSystemLinkResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	systemID := data.SystemID.ValueString()
	environmentID := data.EnvironmentID.ValueString()

	got, err := r.workspace.System.GetEnvironmentSystemLink(ctx, connect.NewRequest(&apiv1.EnvironmentSystemLinkRequest{
		WorkspaceId:   r.workspace.WorkspaceID(),
		SystemId:      systemID,
		EnvironmentId: environmentID,
	}))
	if err != nil {
		if isNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		addConnectError(&resp.Diagnostics, "Failed to read environment system link", err)
		return
	}

	link := got.Msg
	data.SystemID = types.StringValue(link.GetSystemId())
	data.EnvironmentID = types.StringValue(link.GetEnvironmentId())
	data.ID = types.StringValue(link.GetSystemId() + "/" + link.GetEnvironmentId())

	resp.Diagnostics.Append(resp.State.Set(ctx, data)...)
}

func (r *EnvironmentSystemLinkResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	resp.Diagnostics.AddError(
		"Update not supported",
		"Environment system links cannot be updated in-place. Changing system_id or environment_id requires resource replacement.",
	)
}

func (r *EnvironmentSystemLinkResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data EnvironmentSystemLinkResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	_, err := r.workspace.System.UnlinkEnvironmentFromSystem(ctx, connect.NewRequest(&apiv1.EnvironmentSystemLinkRequest{
		WorkspaceId:   r.workspace.WorkspaceID(),
		SystemId:      data.SystemID.ValueString(),
		EnvironmentId: data.EnvironmentID.ValueString(),
	}))
	if err != nil && !isNotFound(err) {
		addConnectError(&resp.Diagnostics, "Failed to unlink environment from system", err)
		return
	}
}
