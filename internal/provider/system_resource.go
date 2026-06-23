// Copyright IBM Corp. 2021, 2026

package provider

import (
	"context"

	apiv1 "buf.build/gen/go/ctrlplane/ctrlplane/protocolbuffers/go/ctrlplane/api/v1"
	connect "connectrpc.com/connect"
	"github.com/ctrlplanedev/terraform-provider-ctrlplane/internal/api"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/defaults"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/mapdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ resource.Resource = &SystemResource{}

var _ resource.ResourceWithImportState = &SystemResource{}
var _ resource.ResourceWithConfigure = &SystemResource{}

func NewSystemResource() resource.Resource {
	return &SystemResource{}
}

type SystemResource struct {
	workspace *api.WorkspaceClient
}

// ImportState implements resource.ResourceWithImportState.
func (r *SystemResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// Configure implements resource.ResourceWithConfigure.
func (r *SystemResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

// Create implements resource.Resource.
func (r *SystemResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data SystemResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var metadata map[string]string
	if p := stringMapPointer(data.Metadata); p != nil {
		metadata = *p
	}

	created, err := r.workspace.System.CreateSystem(ctx, connect.NewRequest(&apiv1.CreateSystemRequest{
		WorkspaceId: r.workspace.WorkspaceID(),
		Name:        data.Name.ValueString(),
		Description: data.Description.ValueStringPointer(),
		Metadata:    metadata,
	}))
	if err != nil {
		addConnectError(&resp.Diagnostics, "Failed to create system", err)
		return
	}

	systemId := created.Msg.GetId()
	if systemId == "" {
		resp.Diagnostics.AddError("Failed to create system", "Empty system ID in response")
		return
	}

	data.ID = types.StringValue(systemId)

	got, err := r.workspace.System.GetSystem(ctx, connect.NewRequest(&apiv1.GetSystemRequest{
		WorkspaceId: r.workspace.WorkspaceID(),
		SystemId:    systemId,
	}))
	if err != nil {
		addConnectError(&resp.Diagnostics, "Failed to read system after create", err)
		return
	}

	applySystem(&data, got.Msg)

	resp.Diagnostics.Append(resp.State.Set(ctx, data)...)
}

// Delete implements resource.Resource.
func (r *SystemResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data SystemResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	_, err := r.workspace.System.DeleteSystem(ctx, connect.NewRequest(&apiv1.DeleteSystemRequest{
		WorkspaceId: r.workspace.WorkspaceID(),
		SystemId:    data.ID.ValueString(),
	}))
	if err != nil && !isNotFound(err) {
		addConnectError(&resp.Diagnostics, "Failed to delete system", err)
		return
	}
}

// Read implements resource.Resource.
func (r *SystemResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data SystemResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	got, err := r.workspace.System.GetSystem(ctx, connect.NewRequest(&apiv1.GetSystemRequest{
		WorkspaceId: r.workspace.WorkspaceID(),
		SystemId:    data.ID.ValueString(),
	}))
	if err != nil {
		if isNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		addConnectError(&resp.Diagnostics, "Failed to read system", err)
		return
	}

	system := got.Msg
	if system.GetId() == "" {
		resp.Diagnostics.AddError("Failed to read system", "Empty system ID in response")
		return
	}

	applySystem(&data, system)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

// applySystem maps an authoritative *apiv1.System response onto the Terraform
// resource model.
func applySystem(data *SystemResourceModel, s *apiv1.System) {
	data.ID = types.StringValue(s.GetId())
	data.Name = types.StringValue(s.GetName())
	data.Description = optionalString(s.GetDescription())
	data.Metadata = metadataMapValue(s.GetMetadata())
}

// Schema implements resource.Resource.
func (r *SystemResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:    true,
				Description: "The ID of the system",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				Required:    true,
				Description: "The name of the system",
			},
			"description": schema.StringAttribute{
				Optional:    true,
				Description: "The description of the system",
			},
			"metadata": schema.MapAttribute{
				Optional:    true,
				Computed:    true,
				Description: "The metadata of the system",
				ElementType: types.StringType,
				Default: func() defaults.Map {
					empty, _ := types.MapValueFrom(context.Background(), types.StringType, map[string]string{})
					return mapdefault.StaticValue(empty)
				}(),
			},
		},
	}
}

// Update implements resource.Resource.
func (r *SystemResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data SystemResourceModel
	var state SystemResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Preserve the existing ID since it is computed and not known from the plan.
	data.ID = state.ID

	var metadata map[string]string
	if p := stringMapPointer(data.Metadata); p != nil {
		metadata = *p
	}

	upserted, err := r.workspace.System.UpsertSystem(ctx, connect.NewRequest(&apiv1.UpsertSystemRequest{
		WorkspaceId: r.workspace.WorkspaceID(),
		SystemId:    data.ID.ValueString(),
		Name:        data.Name.ValueString(),
		Description: data.Description.ValueStringPointer(),
		Metadata:    metadata,
	}))
	if err != nil {
		addConnectError(&resp.Diagnostics, "Failed to update system", err)
		return
	}

	systemId := data.ID.ValueString()
	if id := upserted.Msg.GetId(); id != "" {
		systemId = id
	}

	got, err := r.workspace.System.GetSystem(ctx, connect.NewRequest(&apiv1.GetSystemRequest{
		WorkspaceId: r.workspace.WorkspaceID(),
		SystemId:    systemId,
	}))
	if err != nil {
		addConnectError(&resp.Diagnostics, "Failed to read system after update", err)
		return
	}

	applySystem(&data, got.Msg)

	resp.Diagnostics.Append(resp.State.Set(ctx, data)...)
}

func (r *SystemResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_system"
}

type SystemResourceModel struct {
	ID          types.String `tfsdk:"id"`
	Name        types.String `tfsdk:"name"`
	Description types.String `tfsdk:"description"`
	Metadata    types.Map    `tfsdk:"metadata"`
}
