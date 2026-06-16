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

var _ resource.Resource = &EnvironmentResource{}
var _ resource.ResourceWithImportState = &EnvironmentResource{}
var _ resource.ResourceWithConfigure = &EnvironmentResource{}

func NewEnvironmentResource() resource.Resource {
	return &EnvironmentResource{}
}

type EnvironmentResource struct {
	workspace *api.WorkspaceClient
}

// ImportState implements resource.ResourceWithImportState.
func (r *EnvironmentResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// Configure implements resource.ResourceWithConfigure.
func (r *EnvironmentResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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
func (r *EnvironmentResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data EnvironmentResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var selector *string
	if cel := normalizeCEL(data.ResourceSelector); cel != "" {
		selector = &cel
	}

	var metadata map[string]string
	if p := stringMapPointer(data.Metadata); p != nil {
		metadata = *p
	}

	created, err := r.workspace.System.CreateEnvironment(ctx, connect.NewRequest(&apiv1.CreateEnvironmentRequest{
		WorkspaceId:      r.workspace.WorkspaceID(),
		Name:             data.Name.ValueString(),
		Description:      data.Description.ValueStringPointer(),
		ResourceSelector: selector,
		Metadata:         metadata,
	}))
	if err != nil {
		addConnectError(&resp.Diagnostics, "Failed to create environment", err)
		return
	}

	envId := created.Msg.GetId()
	if envId == "" {
		resp.Diagnostics.AddError("Failed to create environment", "Empty environment ID in response")
		return
	}

	data.ID = types.StringValue(envId)

	got, err := r.workspace.System.GetEnvironment(ctx, connect.NewRequest(&apiv1.GetEnvironmentRequest{
		WorkspaceId:   r.workspace.WorkspaceID(),
		EnvironmentId: envId,
	}))
	if err != nil {
		addConnectError(&resp.Diagnostics, "Failed to read environment after create", err)
		return
	}

	applyEnvironment(&data, got.Msg)

	resp.Diagnostics.Append(resp.State.Set(ctx, data)...)
}

// Delete implements resource.Resource.
func (r *EnvironmentResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data EnvironmentResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	_, err := r.workspace.System.DeleteEnvironment(ctx, connect.NewRequest(&apiv1.DeleteEnvironmentRequest{
		WorkspaceId:   r.workspace.WorkspaceID(),
		EnvironmentId: data.ID.ValueString(),
	}))
	if err != nil && !isNotFound(err) {
		addConnectError(&resp.Diagnostics, "Failed to delete environment", err)
		return
	}
}

// Read implements resource.Resource.
func (r *EnvironmentResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data EnvironmentResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	got, err := r.workspace.System.GetEnvironment(ctx, connect.NewRequest(&apiv1.GetEnvironmentRequest{
		WorkspaceId:   r.workspace.WorkspaceID(),
		EnvironmentId: data.ID.ValueString(),
	}))
	if err != nil {
		if isNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		addConnectError(&resp.Diagnostics, "Failed to read environment", err)
		return
	}

	env := got.Msg
	if env.GetId() == "" {
		resp.Diagnostics.AddError("Failed to read environment", "Empty environment ID in response")
		return
	}
	if env.GetName() == "" {
		resp.Diagnostics.AddError("Failed to read environment", "Empty environment name in response")
		return
	}

	applyEnvironment(&data, env)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

// applyEnvironment hydrates the Terraform model from an authoritative
// *apiv1.Environment returned by the API.
func applyEnvironment(data *EnvironmentResourceModel, env *apiv1.Environment) {
	data.ID = types.StringValue(env.GetId())
	data.Name = types.StringValue(env.GetName())
	data.Description = optionalString(env.GetDescription())
	data.Metadata = metadataMapValue(env.GetMetadata())
	if selector := env.GetResourceSelector(); selector != "" {
		data.ResourceSelector = types.StringValue(selector)
	} else {
		data.ResourceSelector = types.StringNull()
	}
}

// Schema implements resource.Resource.
func (r *EnvironmentResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:    true,
				Description: "The ID of the environment",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				Required:    true,
				Description: "The name of the environment",
			},
			"description": schema.StringAttribute{
				Optional:    true,
				Description: "The description of the environment",
			},
			"resource_selector": schema.StringAttribute{
				Optional:    true,
				Description: "CEL expression used to select resources",
				PlanModifiers: []planmodifier.String{
					celNormalized(),
				},
			},
			"metadata": schema.MapAttribute{
				Optional:    true,
				Computed:    true,
				Description: "The metadata of the environment",
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
func (r *EnvironmentResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data EnvironmentResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var selector *string
	if cel := normalizeCEL(data.ResourceSelector); cel != "" {
		selector = &cel
	}

	var metadata map[string]string
	if p := stringMapPointer(data.Metadata); p != nil {
		metadata = *p
	}

	upserted, err := r.workspace.System.UpsertEnvironment(ctx, connect.NewRequest(&apiv1.UpsertEnvironmentRequest{
		WorkspaceId:      r.workspace.WorkspaceID(),
		EnvironmentId:    data.ID.ValueString(),
		Name:             data.Name.ValueString(),
		Description:      data.Description.ValueStringPointer(),
		ResourceSelector: selector,
		Metadata:         metadata,
	}))
	if err != nil {
		addConnectError(&resp.Diagnostics, "Failed to update environment", err)
		return
	}

	envId := upserted.Msg.GetId()
	if envId == "" {
		envId = data.ID.ValueString()
	}
	data.ID = types.StringValue(envId)

	got, err := r.workspace.System.GetEnvironment(ctx, connect.NewRequest(&apiv1.GetEnvironmentRequest{
		WorkspaceId:   r.workspace.WorkspaceID(),
		EnvironmentId: envId,
	}))
	if err != nil {
		addConnectError(&resp.Diagnostics, "Failed to read environment after update", err)
		return
	}

	applyEnvironment(&data, got.Msg)

	resp.Diagnostics.Append(resp.State.Set(ctx, data)...)
}

func (r *EnvironmentResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_environment"
}

type EnvironmentResourceModel struct {
	ID               types.String `tfsdk:"id"`
	Name             types.String `tfsdk:"name"`
	ResourceSelector types.String `tfsdk:"resource_selector"`
	Description      types.String `tfsdk:"description"`
	Metadata         types.Map    `tfsdk:"metadata"`
}
