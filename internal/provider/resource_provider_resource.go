// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"encoding/json"
	"fmt"

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
	"google.golang.org/protobuf/types/known/structpb"
)

var _ resource.Resource = &ResourceProviderResource{}
var _ resource.ResourceWithImportState = &ResourceProviderResource{}
var _ resource.ResourceWithConfigure = &ResourceProviderResource{}

func NewResourceProviderResource() resource.Resource {
	return &ResourceProviderResource{}
}

type ResourceProviderResource struct {
	workspace *api.WorkspaceClient
}

func (r *ResourceProviderResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_resource_provider"
}

func (r *ResourceProviderResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("name"), req.ID)...)
}

func (r *ResourceProviderResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *ResourceProviderResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a resource provider and its resources in Ctrlplane. " +
			"All resources belonging to the provider are declared as nested blocks and " +
			"sent as a complete set on every apply, avoiding race conditions that occur " +
			"when managing individual resources separately.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:    true,
				Description: "The unique identifier of the resource provider",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				Required:    true,
				Description: "The name of the resource provider",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"metadata": schema.MapAttribute{
				Optional:    true,
				Computed:    true,
				Description: "Metadata key-value pairs for the resource provider",
				ElementType: types.StringType,
				Default: func() defaults.Map {
					empty, _ := types.MapValueFrom(context.Background(), types.StringType, map[string]string{})
					return mapdefault.StaticValue(empty)
				}(),
			},
		},
		Blocks: map[string]schema.Block{
			"resource": schema.ListNestedBlock{
				Description: "Resources managed by this provider",
				NestedObject: schema.NestedBlockObject{
					Attributes: map[string]schema.Attribute{
						"name": schema.StringAttribute{
							Required:    true,
							Description: "The name of the resource",
						},
						"identifier": schema.StringAttribute{
							Required:    true,
							Description: "The unique identifier for the resource",
						},
						"kind": schema.StringAttribute{
							Required:    true,
							Description: "The kind/type of the resource",
						},
						"version": schema.StringAttribute{
							Required:    true,
							Description: "The version of the resource",
						},
						"config": schema.StringAttribute{
							Optional:    true,
							Description: "JSON-encoded configuration for the resource. Use jsonencode() to set this value.",
						},
						"metadata": schema.MapAttribute{
							Optional:    true,
							Computed:    true,
							Description: "Metadata key-value pairs for the resource",
							ElementType: types.StringType,
							Default: func() defaults.Map {
								empty, _ := types.MapValueFrom(context.Background(), types.StringType, map[string]string{})
								return mapdefault.StaticValue(empty)
							}(),
						},
					},
				},
			},
		},
	}
}

func (r *ResourceProviderResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data ResourceProviderModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	upserted, err := r.workspace.Resource.UpsertResourceProvider(ctx, connect.NewRequest(&apiv1.UpsertResourceProviderRequest{
		WorkspaceId: r.workspace.WorkspaceID(),
		Name:        data.Name.ValueString(),
		Metadata:    resourceMetadataFromMap(data.Metadata),
	}))
	if err != nil {
		addConnectError(&resp.Diagnostics, "Failed to create resource provider", err)
		return
	}

	providerID := upserted.Msg.GetId()
	if providerID == "" {
		resp.Diagnostics.AddError("Failed to create resource provider", "Empty resource provider ID in response")
		return
	}

	data.ID = types.StringValue(providerID)

	if len(data.Resources) > 0 {
		if err := r.setResources(ctx, providerID, data.Resources); err != nil {
			resp.Diagnostics.AddError("Failed to set resources", err.Error())
			return
		}

	}

	resp.Diagnostics.Append(resp.State.Set(ctx, data)...)
}

func (r *ResourceProviderResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data ResourceProviderModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	providerResp, err := r.workspace.Resource.GetResourceProviderByName(ctx, connect.NewRequest(&apiv1.GetResourceProviderByNameRequest{
		WorkspaceId: r.workspace.WorkspaceID(),
		Name:        data.Name.ValueString(),
	}))
	if err != nil {
		if isNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		addConnectError(&resp.Diagnostics, "Failed to read resource provider", err)
		return
	}

	provider := providerResp.Msg
	if provider.GetId() == "" {
		resp.Diagnostics.AddError("Failed to read resource provider", "Empty resource provider ID in response")
		return
	}

	data.ID = types.StringValue(provider.GetId())
	data.Name = types.StringValue(provider.GetName())
	data.Metadata = metadataMapValue(provider.GetMetadata())

	resourcesResp, err := r.workspace.Resource.GetResourceProviderResources(ctx, connect.NewRequest(&apiv1.GetResourceProviderResourcesRequest{
		WorkspaceId: r.workspace.WorkspaceID(),
		Name:        data.Name.ValueString(),
	}))
	if err != nil {
		addConnectError(&resp.Diagnostics, "Failed to list provider resources", err)
		return
	}

	var updatedResources []ResourceProviderResourceItemModel
	for _, apiRes := range resourcesResp.Msg.GetItems() {
		updatedResources = append(updatedResources, ResourceProviderResourceItemModel{
			Name:       types.StringValue(apiRes.GetName()),
			Identifier: types.StringValue(apiRes.GetIdentifier()),
			Kind:       types.StringValue(apiRes.GetKind()),
			Version:    types.StringValue(apiRes.GetVersion()),
			Config:     configStructToJSONString(apiRes.GetConfig()),
			Metadata:   metadataMapValue(apiRes.GetMetadata()),
		})
	}
	data.Resources = updatedResources

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *ResourceProviderResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data ResourceProviderModel
	var state ResourceProviderModel

	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	upserted, err := r.workspace.Resource.UpsertResourceProvider(ctx, connect.NewRequest(&apiv1.UpsertResourceProviderRequest{
		WorkspaceId: r.workspace.WorkspaceID(),
		Name:        data.Name.ValueString(),
		Metadata:    resourceMetadataFromMap(data.Metadata),
	}))
	if err != nil {
		addConnectError(&resp.Diagnostics, "Failed to update resource provider", err)
		return
	}

	if id := upserted.Msg.GetId(); id != "" {
		data.ID = types.StringValue(id)
	}

	// Set all current resources on the provider first so the backend is
	// never left in a partially-deleted state if the call fails.
	if len(data.Resources) > 0 {
		if err := r.setResources(ctx, data.ID.ValueString(), data.Resources); err != nil {
			resp.Diagnostics.AddError("Failed to set resources", err.Error())
			return
		}
	}

	// Delete resources that were removed from the config.
	newIdentifiers := make(map[string]bool, len(data.Resources))
	for _, res := range data.Resources {
		newIdentifiers[res.Identifier.ValueString()] = true
	}
	for _, res := range state.Resources {
		identifier := res.Identifier.ValueString()
		if newIdentifiers[identifier] {
			continue
		}
		if err := r.deleteResource(ctx, identifier); err != nil {
			resp.Diagnostics.AddError("Failed to delete resource",
				fmt.Sprintf("Failed to delete resource '%s': %s", identifier, err.Error()))
			return
		}
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, data)...)
}

func (r *ResourceProviderResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data ResourceProviderModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	for _, res := range data.Resources {
		identifier := res.Identifier.ValueString()
		if err := r.deleteResource(ctx, identifier); err != nil {
			resp.Diagnostics.AddError("Failed to delete resource",
				fmt.Sprintf("Failed to delete resource '%s': %s", identifier, err.Error()))
			return
		}
	}

	_, err := r.workspace.Resource.DeleteResourceProviderByName(ctx, connect.NewRequest(&apiv1.DeleteResourceProviderByNameRequest{
		WorkspaceId: r.workspace.WorkspaceID(),
		Name:        data.Name.ValueString(),
	}))
	if err != nil && !isNotFound(err) {
		addConnectError(&resp.Diagnostics, "Failed to delete resource provider", err)
		return
	}
}

// setResources calls SetResourceProviderResources with the full list.
func (r *ResourceProviderResource) setResources(ctx context.Context, providerID string, items []ResourceProviderResourceItemModel) error {
	apiResources, err := resourceInputsFromModel(items)
	if err != nil {
		return err
	}

	_, err = r.workspace.Resource.SetResourceProviderResources(ctx, connect.NewRequest(&apiv1.SetResourceProviderResourcesRequest{
		WorkspaceId: r.workspace.WorkspaceID(),
		ProviderId:  providerID,
		Resources:   apiResources,
	}))
	if err != nil {
		return err
	}
	return nil
}

// deleteResource deletes a single resource by identifier, ignoring NotFound.
func (r *ResourceProviderResource) deleteResource(ctx context.Context, identifier string) error {
	_, err := r.workspace.Resource.DeleteResourceByIdentifier(ctx, connect.NewRequest(&apiv1.DeleteResourceByIdentifierRequest{
		WorkspaceId: r.workspace.WorkspaceID(),
		Identifier:  identifier,
	}))
	if err != nil && !isNotFound(err) {
		return err
	}
	return nil
}

// ResourceProviderModel describes the resource provider data model.
type ResourceProviderModel struct {
	ID        types.String                        `tfsdk:"id"`
	Name      types.String                        `tfsdk:"name"`
	Metadata  types.Map                           `tfsdk:"metadata"`
	Resources []ResourceProviderResourceItemModel `tfsdk:"resource"`
}

// ResourceProviderResourceItemModel describes an individual resource within the provider.
type ResourceProviderResourceItemModel struct {
	Name       types.String `tfsdk:"name"`
	Identifier types.String `tfsdk:"identifier"`
	Kind       types.String `tfsdk:"kind"`
	Version    types.String `tfsdk:"version"`
	Config     types.String `tfsdk:"config"`
	Metadata   types.Map    `tfsdk:"metadata"`
}

// resourceInputsFromModel converts Terraform model resource items to proto ResourceInput format.
func resourceInputsFromModel(items []ResourceProviderResourceItemModel) ([]*apiv1.ResourceInput, error) {
	result := make([]*apiv1.ResourceInput, 0, len(items))
	for _, item := range items {
		config, err := configStructFromJSONString(item.Config)
		if err != nil {
			return nil, fmt.Errorf("resource '%s': %w", item.Identifier.ValueString(), err)
		}
		result = append(result, &apiv1.ResourceInput{
			Name:       item.Name.ValueString(),
			Identifier: item.Identifier.ValueString(),
			Kind:       item.Kind.ValueString(),
			Version:    item.Version.ValueString(),
			Config:     config,
			Metadata:   resourceMetadataFromMap(item.Metadata),
		})
	}
	return result, nil
}

// configStructFromJSONString parses a JSON-object string into a *structpb.Struct.
// An empty/null/unknown value yields a nil struct (an absent config).
func configStructFromJSONString(s types.String) (*structpb.Struct, error) {
	if s.IsNull() || s.IsUnknown() || s.ValueString() == "" {
		return nil, nil
	}
	var config map[string]interface{}
	if err := json.Unmarshal([]byte(s.ValueString()), &config); err != nil {
		return nil, fmt.Errorf("config must be a JSON object: %w", err)
	}
	return structpb.NewStruct(config)
}

// configStructToJSONString renders a *structpb.Struct config back to the
// JSON-string form held in state. An empty or nil struct yields a null value.
func configStructToJSONString(s *structpb.Struct) types.String {
	if s == nil {
		return types.StringNull()
	}
	m := s.AsMap()
	if len(m) == 0 {
		return types.StringNull()
	}
	b, err := json.Marshal(m)
	if err != nil {
		return types.StringNull()
	}
	return types.StringValue(string(b))
}
