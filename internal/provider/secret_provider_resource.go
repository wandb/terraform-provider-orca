// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"

	apiv1 "buf.build/gen/go/ctrlplane/ctrlplane/protocolbuffers/go/ctrlplane/api/v1"
	connect "connectrpc.com/connect"
	"github.com/ctrlplanedev/terraform-provider-ctrlplane/internal/api"
	providervalidator "github.com/ctrlplanedev/terraform-provider-ctrlplane/internal/validator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	schemavalidator "github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ resource.Resource = &SecretProviderResource{}
var _ resource.ResourceWithImportState = &SecretProviderResource{}
var _ resource.ResourceWithConfigure = &SecretProviderResource{}

func NewSecretProviderResource() resource.Resource {
	return &SecretProviderResource{}
}

type SecretProviderResource struct {
	workspace *api.WorkspaceClient
}

type SecretProviderResourceModel struct {
	ID              types.String `tfsdk:"id"`
	Name            types.String `tfsdk:"name"`
	Type            types.String `tfsdk:"type"`
	ConfigWO        types.String `tfsdk:"config_wo"`
	ConfigWOVersion types.Int64  `tfsdk:"config_wo_version"`
}

func (r *SecretProviderResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_secret_provider"
}

func (r *SecretProviderResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("name"), req, resp)
}

func (r *SecretProviderResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *SecretProviderResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a secret provider in Ctrlplane. A secret provider connects Ctrlplane to an external secret store that holds secret values.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:      true,
				Description:   "The unique identifier of the secret provider",
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"name": schema.StringAttribute{
				Required:    true,
				Description: "The name of the secret provider (unique within the workspace)",
			},
			"type": schema.StringAttribute{
				Required:    true,
				Description: "The type of the secret provider. Supported values: `google-secrets-manager`, `doppler`, `aws-secrets-manager`.",
			},
			"config_wo": schema.StringAttribute{
				Required:    true,
				Sensitive:   true,
				WriteOnly:   true,
				Description: "Provider-specific configuration serialized as JSON. Terraform does not store this value in plan or state artifacts. After importing a secret provider, configure this attribute because the API cannot return it.",
				Validators: []schemavalidator.String{
					providervalidator.NewJSONValidator(),
				},
			},
			"config_wo_version": schema.Int64Attribute{
				Optional:    true,
				Computed:    true,
				Default:     int64default.StaticInt64(1),
				Description: "Version of the write-only provider configuration. Increment this value to apply rotated credentials. This value is not recovered during import.",
			},
		},
	}
}

func (r *SecretProviderResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data SecretProviderResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var config types.String
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("config_wo"), &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	created, err := r.workspace.SecretProvider.CreateSecretProvider(ctx, connect.NewRequest(&apiv1.CreateSecretProviderRequest{
		WorkspaceId: r.workspace.WorkspaceID(),
		Name:        data.Name.ValueString(),
		Type:        data.Type.ValueString(),
		Config:      config.ValueString(),
	}))
	if err != nil {
		addConnectError(&resp.Diagnostics, "Failed to create secret provider", err)
		return
	}

	provider := created.Msg
	data.ID = types.StringValue(provider.GetId())
	data.Name = types.StringValue(provider.GetName())
	data.Type = types.StringValue(provider.GetType())

	resp.Diagnostics.Append(resp.State.Set(ctx, data)...)
}

func (r *SecretProviderResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data SecretProviderResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	got, err := r.workspace.SecretProvider.GetSecretProvider(ctx, connect.NewRequest(&apiv1.GetSecretProviderRequest{
		WorkspaceId: r.workspace.WorkspaceID(),
		Name:        data.Name.ValueString(),
	}))
	if err != nil {
		if isNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		addConnectError(&resp.Diagnostics, "Failed to read secret provider", err)
		return
	}

	provider := got.Msg
	data.ID = types.StringValue(provider.GetId())
	data.Name = types.StringValue(provider.GetName())
	data.Type = types.StringValue(provider.GetType())

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *SecretProviderResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data SecretProviderResourceModel
	var state SecretProviderResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var config types.String
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("config_wo"), &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	data.ID = state.ID

	updated, err := r.workspace.SecretProvider.UpdateSecretProvider(ctx, connect.NewRequest(&apiv1.UpdateSecretProviderRequest{
		WorkspaceId: r.workspace.WorkspaceID(),
		Id:          data.ID.ValueString(),
		Name:        data.Name.ValueString(),
		Type:        data.Type.ValueString(),
		Config:      config.ValueString(),
	}))
	if err != nil {
		addConnectError(&resp.Diagnostics, "Failed to update secret provider", err)
		return
	}

	provider := updated.Msg
	if id := provider.GetId(); id != "" {
		data.ID = types.StringValue(id)
	}
	data.Name = types.StringValue(provider.GetName())
	data.Type = types.StringValue(provider.GetType())

	resp.Diagnostics.Append(resp.State.Set(ctx, data)...)
}

func (r *SecretProviderResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data SecretProviderResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	_, err := r.workspace.SecretProvider.DeleteSecretProvider(ctx, connect.NewRequest(&apiv1.DeleteSecretProviderRequest{
		WorkspaceId: r.workspace.WorkspaceID(),
		Id:          data.ID.ValueString(),
	}))
	if err != nil && !isNotFound(err) {
		addConnectError(&resp.Diagnostics, "Failed to delete secret provider", err)
		return
	}
}
