// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"

	apiv1 "buf.build/gen/go/ctrlplane/ctrlplane/protocolbuffers/go/ctrlplane/api/v1"
	connect "connectrpc.com/connect"
	"github.com/ctrlplanedev/terraform-provider-ctrlplane/internal/api"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ resource.Resource = &SecretResource{}
var _ resource.ResourceWithImportState = &SecretResource{}
var _ resource.ResourceWithConfigure = &SecretResource{}

func NewSecretResource() resource.Resource {
	return &SecretResource{}
}

type SecretResource struct {
	workspace *api.WorkspaceClient
}

// SecretResourceModel describes a secret reference. A secret does not hold the
// secret material itself — it points at a value managed by a secret provider
// (provider_id + path + key + version), so no sensitive value is stored in
// Terraform state.
type SecretResourceModel struct {
	ID            types.String `tfsdk:"id"`
	Scope         types.String `tfsdk:"scope"`
	DeploymentID  types.String `tfsdk:"deployment_id"`
	EnvironmentID types.String `tfsdk:"environment_id"`
	ResourceID    types.String `tfsdk:"resource_id"`
	JobAgentID    types.String `tfsdk:"job_agent_id"`
	Name          types.String `tfsdk:"name"`
	ProviderID    types.String `tfsdk:"provider_id"`
	Path          types.List   `tfsdk:"path"`
	Key           types.String `tfsdk:"key"`
	Version       types.String `tfsdk:"version"`
}

func (r *SecretResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_secret"
}

func (r *SecretResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func (r *SecretResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *SecretResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	requiresReplaceStr := []planmodifier.String{stringplanmodifier.RequiresReplace()}

	resp.Schema = schema.Schema{
		Description: "Manages a secret in Ctrlplane. A secret is a reference to a value stored in a secret provider; the value itself is never held in Terraform state.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:      true,
				Description:   "The unique identifier of the secret",
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"scope": schema.StringAttribute{
				Required:      true,
				Description:   "The scope of the secret (e.g. workspace, deployment, environment, resource, job_agent). Changing this forces a new secret.",
				PlanModifiers: requiresReplaceStr,
			},
			"deployment_id": schema.StringAttribute{
				Optional:      true,
				Description:   "The deployment this secret is scoped to. Changing this forces a new secret.",
				PlanModifiers: requiresReplaceStr,
			},
			"environment_id": schema.StringAttribute{
				Optional:      true,
				Description:   "The environment this secret is scoped to. Changing this forces a new secret.",
				PlanModifiers: requiresReplaceStr,
			},
			"resource_id": schema.StringAttribute{
				Optional:      true,
				Description:   "The resource this secret is scoped to. Changing this forces a new secret.",
				PlanModifiers: requiresReplaceStr,
			},
			"job_agent_id": schema.StringAttribute{
				Optional:      true,
				Description:   "The job agent this secret is scoped to. Changing this forces a new secret.",
				PlanModifiers: requiresReplaceStr,
			},
			"name": schema.StringAttribute{
				Required:    true,
				Description: "The name of the secret",
			},
			"provider_id": schema.StringAttribute{
				Required:    true,
				Description: "The ID of the secret provider that holds the secret value",
			},
			"path": schema.ListAttribute{
				Required:    true,
				ElementType: types.StringType,
				Description: "The path segments addressing the secret within the provider",
			},
			"key": schema.StringAttribute{
				Required:    true,
				Description: "The key addressing the secret value within the provider",
			},
			"version": schema.StringAttribute{
				Optional:    true,
				Description: "The version of the secret value to reference",
			},
		},
	}
}

// pathStrings decodes the path list into a []string for the request.
func (r *SecretResource) pathStrings(ctx context.Context, list types.List) ([]string, diag.Diagnostics) {
	var out []string
	diags := list.ElementsAs(ctx, &out, false)
	return out, diags
}

// applySecret maps a proto Secret onto the model.
func applySecret(ctx context.Context, data *SecretResourceModel, secret *apiv1.Secret) diag.Diagnostics {
	var diags diag.Diagnostics

	data.ID = types.StringValue(secret.GetId())
	data.Scope = types.StringValue(secret.GetScope())
	data.DeploymentID = optionalString(secret.GetDeploymentId())
	data.EnvironmentID = optionalString(secret.GetEnvironmentId())
	data.ResourceID = optionalString(secret.GetResourceId())
	data.JobAgentID = optionalString(secret.GetJobAgentId())
	data.Name = types.StringValue(secret.GetName())
	data.ProviderID = types.StringValue(secret.GetProviderId())
	data.Key = types.StringValue(secret.GetKey())
	data.Version = optionalString(secret.GetVersion())

	pathList, d := types.ListValueFrom(ctx, types.StringType, secret.GetPath())
	diags.Append(d...)
	data.Path = pathList

	return diags
}

func (r *SecretResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data SecretResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	pathSegments, d := r.pathStrings(ctx, data.Path)
	resp.Diagnostics.Append(d...)
	if resp.Diagnostics.HasError() {
		return
	}

	created, err := r.workspace.Secret.CreateSecret(ctx, connect.NewRequest(&apiv1.CreateSecretRequest{
		WorkspaceId:   r.workspace.WorkspaceID(),
		Scope:         data.Scope.ValueString(),
		DeploymentId:  data.DeploymentID.ValueString(),
		EnvironmentId: data.EnvironmentID.ValueString(),
		ResourceId:    data.ResourceID.ValueString(),
		JobAgentId:    data.JobAgentID.ValueString(),
		Name:          data.Name.ValueString(),
		ProviderId:    data.ProviderID.ValueString(),
		Path:          pathSegments,
		Key:           data.Key.ValueString(),
		Version:       data.Version.ValueString(),
	}))
	if err != nil {
		addConnectError(&resp.Diagnostics, "Failed to create secret", err)
		return
	}

	resp.Diagnostics.Append(applySecret(ctx, &data, created.Msg)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, data)...)
}

func (r *SecretResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data SecretResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	got, err := r.workspace.Secret.GetSecret(ctx, connect.NewRequest(&apiv1.GetSecretRequest{
		WorkspaceId: r.workspace.WorkspaceID(),
		Id:          data.ID.ValueString(),
	}))
	if err != nil {
		if isNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		addConnectError(&resp.Diagnostics, "Failed to read secret", err)
		return
	}

	resp.Diagnostics.Append(applySecret(ctx, &data, got.Msg)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *SecretResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data SecretResourceModel
	var state SecretResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	data.ID = state.ID

	pathSegments, d := r.pathStrings(ctx, data.Path)
	resp.Diagnostics.Append(d...)
	if resp.Diagnostics.HasError() {
		return
	}

	// scope and the scoping IDs are immutable (RequiresReplace), so UpdateSecret
	// only carries the mutable fields.
	updated, err := r.workspace.Secret.UpdateSecret(ctx, connect.NewRequest(&apiv1.UpdateSecretRequest{
		WorkspaceId: r.workspace.WorkspaceID(),
		Id:          data.ID.ValueString(),
		Name:        data.Name.ValueString(),
		ProviderId:  data.ProviderID.ValueString(),
		Path:        pathSegments,
		Key:         data.Key.ValueString(),
		Version:     data.Version.ValueString(),
	}))
	if err != nil {
		addConnectError(&resp.Diagnostics, "Failed to update secret", err)
		return
	}

	resp.Diagnostics.Append(applySecret(ctx, &data, updated.Msg)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, data)...)
}

func (r *SecretResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data SecretResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	_, err := r.workspace.Secret.DeleteSecret(ctx, connect.NewRequest(&apiv1.DeleteSecretRequest{
		WorkspaceId: r.workspace.WorkspaceID(),
		Id:          data.ID.ValueString(),
	}))
	if err != nil && !isNotFound(err) {
		addConnectError(&resp.Diagnostics, "Failed to delete secret", err)
		return
	}
}
