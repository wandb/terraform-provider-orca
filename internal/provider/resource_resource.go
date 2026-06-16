// Copyright IBM Corp. 2021, 2026

package provider

import (
	"context"
	"fmt"
	"math/big"

	apiv1 "buf.build/gen/go/ctrlplane/ctrlplane/protocolbuffers/go/ctrlplane/api/v1"
	connect "connectrpc.com/connect"
	"github.com/ctrlplanedev/terraform-provider-ctrlplane/internal/api"
	"github.com/hashicorp/terraform-plugin-framework/attr"
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

var _ resource.Resource = &ResourceResource{}
var _ resource.ResourceWithImportState = &ResourceResource{}
var _ resource.ResourceWithConfigure = &ResourceResource{}

func NewResourceResource() resource.Resource {
	return &ResourceResource{}
}

type ResourceResource struct {
	workspace *api.WorkspaceClient
}

func (r *ResourceResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_resource"
}

func (r *ResourceResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func (r *ResourceResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *ResourceResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a resource in Ctrlplane.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:    true,
				Description: "The unique identifier of the resource (same as identifier)",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				Required:    true,
				Description: "The name of the resource",
			},
			"identifier": schema.StringAttribute{
				Required:    true,
				Description: "The unique identifier for the resource",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"kind": schema.StringAttribute{
				Required:    true,
				Description: "The kind/type of the resource (e.g., \"kubernetes/pod\")",
			},
			"version": schema.StringAttribute{
				Required:    true,
				Description: "The version of the resource",
			},
			"provider_id": schema.StringAttribute{
				Optional:    true,
				Description: "The ID of the resource provider used to sync this resource",
			},
			"config": schema.DynamicAttribute{
				Optional:    true,
				Description: "Configuration for the resource as a map of key-value pairs",
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
	}
}

func (r *ResourceResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data ResourceResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	configMap, err := resourceConfigFromDynamic(data.Config)
	if err != nil {
		resp.Diagnostics.AddError("Invalid config", err.Error())
		return
	}

	config, err := structpb.NewStruct(configMap)
	if err != nil {
		resp.Diagnostics.AddError("Invalid config", err.Error())
		return
	}

	metadata := resourceMetadataFromMap(data.Metadata)
	identifier := data.Identifier.ValueString()

	_, err = r.workspace.Resource.UpsertResourceByIdentifier(ctx, connect.NewRequest(&apiv1.UpsertResourceByIdentifierRequest{
		WorkspaceId: r.workspace.WorkspaceID(),
		Identifier:  identifier,
		Name:        data.Name.ValueString(),
		Version:     data.Version.ValueString(),
		Kind:        data.Kind.ValueString(),
		Config:      config,
		Metadata:    metadata,
	}))
	if err != nil {
		addConnectError(&resp.Diagnostics, "Failed to create resource", err)
		return
	}

	// UpsertResourceByIdentifier returns only an ack ({id, message}); the engine
	// sets provider_id and may normalize config. Hydrate state from an
	// authoritative read. The API is synchronous, so NotFound is a real error.
	got, err := r.workspace.Resource.GetResourceByIdentifier(ctx, connect.NewRequest(&apiv1.GetResourceByIdentifierRequest{
		WorkspaceId: r.workspace.WorkspaceID(),
		Identifier:  identifier,
	}))
	if err != nil {
		addConnectError(&resp.Diagnostics, "Failed to read resource after create", err)
		return
	}

	applyResource(&data, got.Msg)

	resp.Diagnostics.Append(resp.State.Set(ctx, data)...)
}

func (r *ResourceResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data ResourceResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// On import, only id is set; derive identifier from id.
	identifier := data.Identifier.ValueString()
	if identifier == "" {
		identifier = data.ID.ValueString()
	}

	got, err := r.workspace.Resource.GetResourceByIdentifier(ctx, connect.NewRequest(&apiv1.GetResourceByIdentifierRequest{
		WorkspaceId: r.workspace.WorkspaceID(),
		Identifier:  identifier,
	}))
	if err != nil {
		if isNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		addConnectError(&resp.Diagnostics, "Failed to read resource", err)
		return
	}

	applyResource(&data, got.Msg)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

// applyResource maps an authoritative *apiv1.Resource into the Terraform state model.
func applyResource(data *ResourceResourceModel, res *apiv1.Resource) {
	// id mirrors identifier (schema: "same as identifier").
	data.ID = types.StringValue(res.GetIdentifier())
	data.Name = types.StringValue(res.GetName())
	data.Identifier = types.StringValue(res.GetIdentifier())
	data.Kind = types.StringValue(res.GetKind())
	data.Version = types.StringValue(res.GetVersion())

	if providerID := res.GetProviderId(); providerID != "" {
		data.ProviderID = types.StringValue(providerID)
	}

	var configMap map[string]interface{}
	if cfg := res.GetConfig(); cfg != nil {
		configMap = cfg.AsMap()
	}
	data.Config = goMapToDynamic(configMap)

	data.Metadata = metadataMapValue(res.GetMetadata())
}

func (r *ResourceResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data ResourceResourceModel
	var state ResourceResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Preserve the existing ID since it is computed and not known from the plan.
	data.ID = state.ID

	configMap, err := resourceConfigFromDynamic(data.Config)
	if err != nil {
		resp.Diagnostics.AddError("Invalid config", err.Error())
		return
	}

	config, err := structpb.NewStruct(configMap)
	if err != nil {
		resp.Diagnostics.AddError("Invalid config", err.Error())
		return
	}

	metadata := resourceMetadataFromMap(data.Metadata)

	_, err = r.workspace.Resource.UpsertResourceByIdentifier(ctx, connect.NewRequest(&apiv1.UpsertResourceByIdentifierRequest{
		WorkspaceId: r.workspace.WorkspaceID(),
		Identifier:  data.Identifier.ValueString(),
		Name:        data.Name.ValueString(),
		Version:     data.Version.ValueString(),
		Kind:        data.Kind.ValueString(),
		Config:      config,
		Metadata:    metadata,
	}))
	if err != nil {
		addConnectError(&resp.Diagnostics, "Failed to update resource", err)
		return
	}

	// UpsertResourceByIdentifier returns only an ack ({id, message}); the engine
	// sets provider_id and may normalize config. Hydrate state from an
	// authoritative read. The API is synchronous, so NotFound is a real error.
	got, err := r.workspace.Resource.GetResourceByIdentifier(ctx, connect.NewRequest(&apiv1.GetResourceByIdentifierRequest{
		WorkspaceId: r.workspace.WorkspaceID(),
		Identifier:  data.Identifier.ValueString(),
	}))
	if err != nil {
		addConnectError(&resp.Diagnostics, "Failed to read resource after update", err)
		return
	}

	applyResource(&data, got.Msg)

	resp.Diagnostics.Append(resp.State.Set(ctx, data)...)
}

func (r *ResourceResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data ResourceResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	_, err := r.workspace.Resource.DeleteResourceByIdentifier(ctx, connect.NewRequest(&apiv1.DeleteResourceByIdentifierRequest{
		WorkspaceId: r.workspace.WorkspaceID(),
		Identifier:  data.Identifier.ValueString(),
	}))
	if err != nil && !isNotFound(err) {
		addConnectError(&resp.Diagnostics, "Failed to delete resource", err)
		return
	}
}

// ResourceResourceModel describes the resource data model.
type ResourceResourceModel struct {
	ID         types.String  `tfsdk:"id"`
	Name       types.String  `tfsdk:"name"`
	Identifier types.String  `tfsdk:"identifier"`
	Kind       types.String  `tfsdk:"kind"`
	Version    types.String  `tfsdk:"version"`
	ProviderID types.String  `tfsdk:"provider_id"`
	Config     types.Dynamic `tfsdk:"config"`
	Metadata   types.Map     `tfsdk:"metadata"`
}

// resourceConfigFromDynamic converts a types.Dynamic value to a Go map for the API.
func resourceConfigFromDynamic(d types.Dynamic) (map[string]interface{}, error) {
	if d.IsNull() || d.IsUnknown() {
		return map[string]interface{}{}, nil
	}

	val := d.UnderlyingValue()
	goVal := attrValueToGoValue(val)

	if m, ok := goVal.(map[string]interface{}); ok {
		return m, nil
	}

	return nil, fmt.Errorf("config must be an object/map, got %T", goVal)
}

// goMapToDynamic converts a Go map from the API to a types.Dynamic value.
func goMapToDynamic(m map[string]interface{}) types.Dynamic {
	if len(m) == 0 {
		return types.DynamicNull()
	}

	val, _ := goValueToAttrValue(m)
	return types.DynamicValue(val)
}

// attrValueToGoValue recursively converts an attr.Value to a Go value.
func attrValueToGoValue(val attr.Value) interface{} {
	if val == nil || val.IsNull() || val.IsUnknown() {
		return nil
	}

	switch v := val.(type) {
	case types.String:
		return v.ValueString()
	case types.Bool:
		return v.ValueBool()
	case types.Number:
		f, _ := v.ValueBigFloat().Float64()
		return f
	case types.Int64:
		return float64(v.ValueInt64())
	case types.Float64:
		return v.ValueFloat64()
	case types.Object:
		result := map[string]interface{}{}
		for k, a := range v.Attributes() {
			result[k] = attrValueToGoValue(a)
		}
		return result
	case types.Map:
		result := map[string]interface{}{}
		for k, e := range v.Elements() {
			result[k] = attrValueToGoValue(e)
		}
		return result
	case types.List:
		var result []interface{}
		for _, e := range v.Elements() {
			result = append(result, attrValueToGoValue(e))
		}
		return result
	case types.Tuple:
		var result []interface{}
		for _, e := range v.Elements() {
			result = append(result, attrValueToGoValue(e))
		}
		return result
	default:
		return nil
	}
}

// goValueToAttrValue recursively converts a Go value to an attr.Value and attr.Type.
func goValueToAttrValue(val interface{}) (attr.Value, attr.Type) {
	if val == nil {
		return types.StringNull(), types.StringType
	}

	switch v := val.(type) {
	case string:
		return types.StringValue(v), types.StringType
	case bool:
		return types.BoolValue(v), types.BoolType
	case float64:
		bf := new(big.Float).SetFloat64(v)
		return types.NumberValue(bf), types.NumberType
	case int:
		bf := new(big.Float).SetInt64(int64(v))
		return types.NumberValue(bf), types.NumberType
	case int64:
		bf := new(big.Float).SetInt64(v)
		return types.NumberValue(bf), types.NumberType
	case map[string]interface{}:
		attrValues := map[string]attr.Value{}
		attrTypes := map[string]attr.Type{}
		for k, elem := range v {
			av, at := goValueToAttrValue(elem)
			attrValues[k] = av
			attrTypes[k] = at
		}
		obj, diags := types.ObjectValue(attrTypes, attrValues)
		if diags.HasError() {
			return types.DynamicNull(), types.DynamicType
		}
		return obj, types.ObjectType{AttrTypes: attrTypes}
	case []interface{}:
		if len(v) == 0 {
			empty, _ := types.TupleValue([]attr.Type{}, []attr.Value{})
			return empty, types.TupleType{ElemTypes: []attr.Type{}}
		}
		elemValues := make([]attr.Value, len(v))
		elemTypes := make([]attr.Type, len(v))
		for i, elem := range v {
			ev, et := goValueToAttrValue(elem)
			elemValues[i] = ev
			elemTypes[i] = et
		}
		tuple, diags := types.TupleValue(elemTypes, elemValues)
		if diags.HasError() {
			return types.DynamicNull(), types.DynamicType
		}
		return tuple, types.TupleType{ElemTypes: elemTypes}
	default:
		return types.StringValue(fmt.Sprintf("%v", v)), types.StringType
	}
}

// resourceMetadataFromMap extracts a string map from a Terraform types.Map.
func resourceMetadataFromMap(value types.Map) map[string]string {
	if value.IsNull() || value.IsUnknown() {
		return map[string]string{}
	}

	var decoded map[string]string
	diags := value.ElementsAs(context.Background(), &decoded, false)
	if diags.HasError() {
		return map[string]string{}
	}

	return decoded
}
