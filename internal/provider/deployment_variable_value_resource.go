// Copyright IBM Corp. 2021, 2026

package provider

import (
	"context"
	"encoding/json"
	"fmt"

	apiv1 "buf.build/gen/go/orca/orca/protocolbuffers/go/orca/api/v1"
	connect "connectrpc.com/connect"
	"github.com/ctrlplanedev/terraform-provider-ctrlplane/internal/api"
	providervalidator "github.com/ctrlplanedev/terraform-provider-ctrlplane/internal/validator"
	"github.com/google/uuid"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	schemavalidator "github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	structpb "google.golang.org/protobuf/types/known/structpb"
)

var _ resource.Resource = &DeploymentVariableValueResource{}
var _ resource.ResourceWithImportState = &DeploymentVariableValueResource{}
var _ resource.ResourceWithConfigure = &DeploymentVariableValueResource{}
var _ resource.ResourceWithValidateConfig = &DeploymentVariableValueResource{}

func NewDeploymentVariableValueResource() resource.Resource {
	return &DeploymentVariableValueResource{}
}

type DeploymentVariableValueResource struct {
	workspace *api.WorkspaceClient
}

type DeploymentVariableValueResourceModel struct {
	ID               types.String                          `tfsdk:"id"`
	VariableId       types.String                          `tfsdk:"variable_id"`
	Priority         types.Int64                           `tfsdk:"priority"`
	ResourceSelector CELStringValue                        `tfsdk:"resource_selector"`
	LiteralValue     types.Dynamic                         `tfsdk:"literal_value"`
	ReferenceValue   types.Object                          `tfsdk:"reference_value"`
	Statsig          []DeploymentVariableValueStatsigModel `tfsdk:"statsig"`
	Custom           []DeploymentVariableValueCustomModel  `tfsdk:"custom"`
}

type DeploymentVariableValueStatsigModel struct {
	ProviderID   types.String `tfsdk:"provider_id"`
	GateKey      types.String `tfsdk:"gate_key"`
	UserTemplate types.String `tfsdk:"user_template"`
}

type DeploymentVariableValueCustomModel struct {
	ProviderID types.String `tfsdk:"provider_id"`
	Config     types.String `tfsdk:"config"`
}

var referenceValueAttrTypes = map[string]attr.Type{
	"reference": types.StringType,
	"path":      types.ListType{ElemType: types.StringType},
}

func (r *DeploymentVariableValueResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_deployment_variable_value"
}

func (r *DeploymentVariableValueResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func (r *DeploymentVariableValueResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *DeploymentVariableValueResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a deployment variable value override in Ctrlplane. A variable value provides a specific value for a deployment variable, optionally scoped to resources matching a selector expression.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The ID of the deployment variable value.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"variable_id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The deployment variable ID this value belongs to.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"priority": schema.Int64Attribute{
				Required:            true,
				MarkdownDescription: "The priority of the variable value. Higher priority values take precedence when multiple values match.",
			},
			"resource_selector": schema.StringAttribute{
				CustomType:          CELStringType{},
				Optional:            true,
				MarkdownDescription: "A CEL expression to select which resources this value applies to.",
				PlanModifiers: []planmodifier.String{
					celNormalized(),
				},
			},
			"literal_value": schema.DynamicAttribute{
				Optional:            true,
				MarkdownDescription: "A literal value (string, number, boolean, or object). Conflicts with `reference_value`, `statsig`, and `custom`. Numbers are transmitted as double-precision floats, so integers larger than 2^53 lose precision — pass such values as strings.",
			},
			"reference_value": schema.SingleNestedAttribute{
				Optional:            true,
				MarkdownDescription: "A reference value pointing to a property on the matched resource. Conflicts with `literal_value`, `statsig`, and `custom`.",
				Attributes: map[string]schema.Attribute{
					"reference": schema.StringAttribute{
						Required:            true,
						MarkdownDescription: "The reference key.",
					},
					"path": schema.ListAttribute{
						Required:            true,
						ElementType:         types.StringType,
						MarkdownDescription: "The path segments to the value in the referenced resource.",
					},
				},
			},
		},
		Blocks: map[string]schema.Block{
			"statsig": schema.ListNestedBlock{
				MarkdownDescription: "Resolve this value from a Statsig feature gate.",
				NestedObject: schema.NestedBlockObject{
					Attributes: map[string]schema.Attribute{
						"provider_id": schema.StringAttribute{
							Required:            true,
							MarkdownDescription: "The ID of the Statsig external variable provider.",
						},
						"gate_key": schema.StringAttribute{
							Required:            true,
							MarkdownDescription: "The Statsig feature gate key.",
						},
						"user_template": schema.StringAttribute{
							Required:            true,
							MarkdownDescription: "The Statsig user template as a JSON object. Template expressions are evaluated for each release target.",
							Validators: []schemavalidator.String{
								providervalidator.NewJSONValidator(),
							},
						},
					},
				},
			},
			"custom": schema.ListNestedBlock{
				MarkdownDescription: "Provider-neutral external variable query configuration.",
				NestedObject: schema.NestedBlockObject{
					Attributes: map[string]schema.Attribute{
						"provider_id": schema.StringAttribute{
							Required:            true,
							MarkdownDescription: "The ID of the external variable provider.",
						},
						"config": schema.StringAttribute{
							Required:            true,
							MarkdownDescription: "Provider-specific query configuration as a JSON object.",
							Validators: []schemavalidator.String{
								providervalidator.NewJSONValidator(),
							},
						},
					},
				},
			},
		},
	}
}

func (r *DeploymentVariableValueResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var data DeploymentVariableValueResourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	hasLiteral := !data.LiteralValue.IsNull() && !data.LiteralValue.IsUnknown()
	hasReference := !data.ReferenceValue.IsNull() && !data.ReferenceValue.IsUnknown()
	valueCount := len(data.Statsig) + len(data.Custom)
	if hasLiteral {
		valueCount++
	}
	if hasReference {
		valueCount++
	}

	if valueCount > 1 {
		resp.Diagnostics.AddError(
			"Conflicting value types",
			"Only one of literal_value, reference_value, statsig, or custom may be specified.",
		)
	}

	if valueCount == 0 {
		// Allow unknowns during plan - only error if both are definitively null
		if !data.LiteralValue.IsUnknown() && !data.ReferenceValue.IsUnknown() {
			resp.Diagnostics.AddError(
				"Missing value",
				"Exactly one of literal_value, reference_value, statsig, or custom must be specified.",
			)
		}
	}
}

func (r *DeploymentVariableValueResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data DeploymentVariableValueResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	valueID := data.ID.ValueString()
	if data.ID.IsNull() || data.ID.IsUnknown() || valueID == "" {
		valueID = uuid.NewString()
		data.ID = types.StringValue(valueID)
	}

	protoValue, externalValue, err := deploymentVariableValuePayloadFromModel(data)
	if err != nil {
		resp.Diagnostics.AddError("Failed to create deployment variable value", fmt.Sprintf("Failed to build value: %s", err.Error()))
		return
	}

	selector := celStringPointer(data.ResourceSelector)

	created, err := r.workspace.Deployment.UpsertDeploymentVariableValue(ctx, connect.NewRequest(&apiv1.UpsertDeploymentVariableValueRequest{
		WorkspaceId:          r.workspace.WorkspaceID(),
		ValueId:              valueID,
		DeploymentVariableId: data.VariableId.ValueString(),
		Priority:             data.Priority.ValueInt64(),
		ResourceSelector:     selector,
		Value:                protoValue,
		External:             externalValue,
	}))
	if err != nil {
		addConnectError(&resp.Diagnostics, "Failed to create deployment variable value", err)
		return
	}

	valID := created.Msg.GetId()
	if valID == "" {
		resp.Diagnostics.AddError("Failed to create deployment variable value", "Empty value ID in response")
		return
	}

	data.ID = types.StringValue(valID)

	got, err := r.workspace.Deployment.GetDeploymentVariableValue(ctx, connect.NewRequest(&apiv1.GetDeploymentVariableValueRequest{
		WorkspaceId: r.workspace.WorkspaceID(),
		ValueId:     valID,
	}))
	if err != nil {
		addConnectError(&resp.Diagnostics, "Failed to read deployment variable value after create", err)
		return
	}

	resp.Diagnostics.Append(r.applyDeploymentVariableValue(ctx, &data, got.Msg.GetValue())...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, data)...)
}

func (r *DeploymentVariableValueResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data DeploymentVariableValueResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	got, err := r.workspace.Deployment.GetDeploymentVariableValue(ctx, connect.NewRequest(&apiv1.GetDeploymentVariableValueRequest{
		WorkspaceId: r.workspace.WorkspaceID(),
		ValueId:     data.ID.ValueString(),
	}))
	if err != nil {
		if isNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		addConnectError(&resp.Diagnostics, "Failed to read deployment variable value", err)
		return
	}

	value := got.Msg.GetValue()
	if value.GetId() == "" {
		resp.Diagnostics.AddError("Failed to read deployment variable value", "Empty response from server")
		return
	}

	resp.Diagnostics.Append(r.applyDeploymentVariableValue(ctx, &data, value)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

// applyDeploymentVariableValue maps a proto DeploymentVariableValue onto the
// model (id, variable id, priority, resource_selector, and the value union).
func (r *DeploymentVariableValueResource) applyDeploymentVariableValue(ctx context.Context, data *DeploymentVariableValueResourceModel, value *apiv1.DeploymentVariableValue) diag.Diagnostics {
	var diags diag.Diagnostics

	data.ID = types.StringValue(value.GetId())
	data.VariableId = types.StringValue(value.GetDeploymentVariableId())
	data.Priority = types.Int64Value(value.GetPriority())

	data.ResourceSelector = optionalCELStringValue(value.GetResourceSelector())

	if external := value.GetExternal(); external != nil {
		provider, err := r.workspace.ExternalVariableProvider.GetExternalVariableProvider(ctx, connect.NewRequest(&apiv1.GetExternalVariableProviderRequest{
			WorkspaceId: r.workspace.WorkspaceID(),
			Id:          external.GetProviderId(),
		}))
		if err != nil {
			addConnectError(&diags, "Failed to read external variable provider", err)
			return diags
		}
		if err := setExternalValueOnModel(data, external, provider.Msg.GetExternalVariableProvider().GetType()); err != nil {
			diags.AddError("Failed to read external deployment variable value", err.Error())
		}
		return diags
	}

	return setValueOnModel(ctx, data, value.GetValue())
}

func (r *DeploymentVariableValueResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data DeploymentVariableValueResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	protoValue, externalValue, err := deploymentVariableValuePayloadFromModel(data)
	if err != nil {
		resp.Diagnostics.AddError("Failed to update deployment variable value", fmt.Sprintf("Failed to build value: %s", err.Error()))
		return
	}

	selector := celStringPointer(data.ResourceSelector)

	upserted, err := r.workspace.Deployment.UpsertDeploymentVariableValue(ctx, connect.NewRequest(&apiv1.UpsertDeploymentVariableValueRequest{
		WorkspaceId:          r.workspace.WorkspaceID(),
		ValueId:              data.ID.ValueString(),
		DeploymentVariableId: data.VariableId.ValueString(),
		Priority:             data.Priority.ValueInt64(),
		ResourceSelector:     selector,
		Value:                protoValue,
		External:             externalValue,
	}))
	if err != nil {
		addConnectError(&resp.Diagnostics, "Failed to update deployment variable value", err)
		return
	}

	valID := upserted.Msg.GetId()
	if valID == "" {
		resp.Diagnostics.AddError("Failed to update deployment variable value", "Empty value ID in response")
		return
	}

	data.ID = types.StringValue(valID)

	got, err := r.workspace.Deployment.GetDeploymentVariableValue(ctx, connect.NewRequest(&apiv1.GetDeploymentVariableValueRequest{
		WorkspaceId: r.workspace.WorkspaceID(),
		ValueId:     valID,
	}))
	if err != nil {
		addConnectError(&resp.Diagnostics, "Failed to read deployment variable value after update", err)
		return
	}

	resp.Diagnostics.Append(r.applyDeploymentVariableValue(ctx, &data, got.Msg.GetValue())...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, data)...)
}

func (r *DeploymentVariableValueResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data DeploymentVariableValueResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	_, err := r.workspace.Deployment.DeleteDeploymentVariableValue(ctx, connect.NewRequest(&apiv1.DeleteDeploymentVariableValueRequest{
		WorkspaceId: r.workspace.WorkspaceID(),
		ValueId:     data.ID.ValueString(),
	}))
	if err != nil && !isNotFound(err) {
		addConnectError(&resp.Diagnostics, "Failed to delete deployment variable value", err)
		return
	}
}

func deploymentVariableValuePayloadFromModel(data DeploymentVariableValueResourceModel) (*structpb.Value, *apiv1.ExternalVariableValue, error) {
	external, err := externalVariableValueFromModel(data)
	if err != nil {
		return nil, nil, err
	}
	if external != nil {
		return nil, external, nil
	}

	value, err := structpbValueFromModel(data)
	if err != nil {
		return nil, nil, err
	}
	return value, nil, nil
}

func externalVariableValueFromModel(data DeploymentVariableValueResourceModel) (*apiv1.ExternalVariableValue, error) {
	var providerID string
	var config map[string]any

	switch {
	case len(data.Statsig) == 1:
		statsig := data.Statsig[0]
		userTemplate, err := jsonObjectFromString(statsig.UserTemplate)
		if err != nil {
			return nil, fmt.Errorf("invalid statsig user_template: %w", err)
		}
		providerID = statsig.ProviderID.ValueString()
		config = map[string]any{
			"key":          statsig.GateKey.ValueString(),
			"userTemplate": userTemplate,
		}
	case len(data.Custom) == 1:
		custom := data.Custom[0]
		customConfig, err := jsonObjectFromString(custom.Config)
		if err != nil {
			return nil, fmt.Errorf("invalid custom config: %w", err)
		}
		providerID = custom.ProviderID.ValueString()
		config = customConfig
	case len(data.Statsig) == 0 && len(data.Custom) == 0:
		return nil, nil
	default:
		return nil, fmt.Errorf("only one statsig or custom block may be configured")
	}

	configStruct, err := structpb.NewStruct(config)
	if err != nil {
		return nil, fmt.Errorf("invalid external variable config: %w", err)
	}
	return &apiv1.ExternalVariableValue{
		ProviderId: providerID,
		Config:     configStruct,
	}, nil
}

func setExternalValueOnModel(data *DeploymentVariableValueResourceModel, external *apiv1.ExternalVariableValue, providerType string) error {
	data.LiteralValue = types.DynamicNull()
	data.ReferenceValue = types.ObjectNull(referenceValueAttrTypes)
	data.Statsig = nil
	data.Custom = nil

	if external == nil {
		return fmt.Errorf("external variable value is missing")
	}

	config := map[string]any{}
	if external.GetConfig() != nil {
		config = external.GetConfig().AsMap()
	}

	if providerType == "statsig" {
		gateKey, ok := config["key"].(string)
		if !ok || gateKey == "" {
			return fmt.Errorf("statsig config is missing key")
		}
		userTemplate, ok := config["userTemplate"]
		if !ok {
			return fmt.Errorf("statsig config is missing userTemplate")
		}
		userTemplateJSON, err := json.Marshal(userTemplate)
		if err != nil {
			return fmt.Errorf("encode statsig userTemplate: %w", err)
		}
		data.Statsig = []DeploymentVariableValueStatsigModel{{
			ProviderID:   types.StringValue(external.GetProviderId()),
			GateKey:      types.StringValue(gateKey),
			UserTemplate: types.StringValue(string(userTemplateJSON)),
		}}
		return nil
	}

	configJSON, err := json.Marshal(config)
	if err != nil {
		return fmt.Errorf("encode custom external variable config: %w", err)
	}
	data.Custom = []DeploymentVariableValueCustomModel{{
		ProviderID: types.StringValue(external.GetProviderId()),
		Config:     types.StringValue(string(configJSON)),
	}}
	return nil
}

// structpbValueFromModel converts the Terraform model into the single
// *structpb.Value carried by the proto API. A reference value is encoded as a
// map { "reference": <string>, "path": [<strings>] }; a literal value is
// encoded directly from its decoded Go representation. This replaces the old
// api.Value union, which is being removed.
func structpbValueFromModel(data DeploymentVariableValueResourceModel) (*structpb.Value, error) {
	if !data.ReferenceValue.IsNull() && !data.ReferenceValue.IsUnknown() {
		refAttrs := data.ReferenceValue.Attributes()

		referenceAttr, ok := refAttrs["reference"]
		if !ok {
			return nil, fmt.Errorf("reference_value is missing 'reference' attribute")
		}
		reference, ok := referenceAttr.(types.String)
		if !ok {
			return nil, fmt.Errorf("reference_value.reference is not a string")
		}

		pathAttr, ok := refAttrs["path"]
		if !ok {
			return nil, fmt.Errorf("reference_value is missing 'path' attribute")
		}
		pathList, ok := pathAttr.(types.List)
		if !ok {
			return nil, fmt.Errorf("reference_value.path is not a list")
		}

		var pathStrings []string
		diags := pathList.ElementsAs(context.Background(), &pathStrings, false)
		if diags.HasError() {
			return nil, fmt.Errorf("failed to convert reference_value.path to []string")
		}

		pathAny := make([]any, len(pathStrings))
		for i, p := range pathStrings {
			pathAny[i] = p
		}

		refValue, err := structpb.NewValue(map[string]any{
			"reference": reference.ValueString(),
			"path":      pathAny,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to build reference value: %w", err)
		}
		return refValue, nil
	}

	if !data.LiteralValue.IsNull() && !data.LiteralValue.IsUnknown() {
		tfValue, err := data.LiteralValue.ToTerraformValue(context.Background())
		if err != nil {
			return nil, fmt.Errorf("failed to read literal value: %w", err)
		}

		decoded, err := terraformValueToInterface(tfValue)
		if err != nil {
			return nil, fmt.Errorf("failed to convert literal value: %w", err)
		}

		litValue, err := structpb.NewValue(decoded)
		if err != nil {
			return nil, fmt.Errorf("failed to build literal value: %w", err)
		}
		return litValue, nil
	}

	return nil, fmt.Errorf("one of literal_value or reference_value must be provided")
}

// setValueOnModel reads from the proto *structpb.Value and sets the appropriate
// field on the model. A map carrying both "reference" and "path" keys is treated
// as a reference value; anything else is treated as a literal value.
func setValueOnModel(ctx context.Context, data *DeploymentVariableValueResourceModel, value *structpb.Value) diag.Diagnostics {
	var diags diag.Diagnostics
	data.Statsig = nil
	data.Custom = nil

	if value == nil {
		data.LiteralValue = types.DynamicNull()
		data.ReferenceValue = types.ObjectNull(referenceValueAttrTypes)
		return diags
	}

	decoded := value.AsInterface()

	// Try reference value first: a map with both "reference" and "path" keys.
	if m, ok := decoded.(map[string]any); ok {
		ref, hasRef := m["reference"]
		rawPath, hasPath := m["path"]
		refStr, refIsString := ref.(string)
		if hasRef && hasPath && refIsString && refStr != "" {
			pathSlice, _ := rawPath.([]any)
			pathElements := make([]attr.Value, len(pathSlice))
			for i, p := range pathSlice {
				ps, _ := p.(string)
				pathElements[i] = types.StringValue(ps)
			}

			pathList, listDiags := types.ListValue(types.StringType, pathElements)
			if listDiags.HasError() {
				diags.Append(listDiags...)
				return diags
			}

			refObj, objDiags := types.ObjectValue(referenceValueAttrTypes, map[string]attr.Value{
				"reference": types.StringValue(refStr),
				"path":      pathList,
			})
			if objDiags.HasError() {
				diags.Append(objDiags...)
				return diags
			}

			data.ReferenceValue = refObj
			data.LiteralValue = types.DynamicNull()
			return diags
		}
	}

	// Otherwise treat as a literal value.
	if decoded == nil {
		data.LiteralValue = types.DynamicNull()
		data.ReferenceValue = types.ObjectNull(referenceValueAttrTypes)
		return diags
	}

	var expected attr.Type
	if underlying := data.LiteralValue.UnderlyingValue(); underlying != nil {
		expected = underlying.Type(ctx)
	}
	attrValue, err := literalValueFromInterface(ctx, decoded, expected)
	if err != nil {
		diags.AddError("Failed to read literal value", err.Error())
		return diags
	}

	data.LiteralValue = types.DynamicValue(attrValue)
	data.ReferenceValue = types.ObjectNull(referenceValueAttrTypes)
	return diags
}

// The API does not retain Terraform's list/tuple/set or map/object distinctions.
// Use the plan (create/update) or prior state (read) to restore those types,
// including nested collections. Imports without prior types infer tuples/objects.
func literalValueFromInterface(ctx context.Context, raw any, expected attr.Type) (attr.Value, error) {
	if expected == nil || expected.Equal(types.DynamicType) {
		value, _, err := attrValueFromInterface(raw)
		return value, err
	}
	if raw == nil {
		return expected.ValueFromTerraform(ctx, tftypes.NewValue(expected.TerraformType(ctx), nil))
	}
	switch typ := expected.(type) {
	case types.ListType, types.SetType, types.TupleType:
		items, ok := raw.([]any)
		if !ok {
			return nil, fmt.Errorf("expected %s, got %T", expected, raw)
		}
		values := make([]attr.Value, len(items))
		for i, item := range items {
			var elemType attr.Type
			switch typ := typ.(type) {
			case types.ListType:
				elemType = typ.ElemType
			case types.SetType:
				elemType = typ.ElemType
			case types.TupleType:
				if len(items) != len(typ.ElemTypes) {
					return nil, fmt.Errorf("expected tuple with %d elements, got %d", len(typ.ElemTypes), len(items))
				}
				elemType = typ.ElemTypes[i]
			}
			value, err := literalValueFromInterface(ctx, item, elemType)
			if err != nil {
				return nil, fmt.Errorf("element %d: %w", i, err)
			}
			values[i] = value
		}
		var result attr.Value
		var diags diag.Diagnostics
		switch typ := typ.(type) {
		case types.ListType:
			result, diags = types.ListValue(typ.ElemType, values)
		case types.SetType:
			result, diags = types.SetValue(typ.ElemType, values)
		case types.TupleType:
			result, diags = types.TupleValue(typ.ElemTypes, values)
		}
		if diags.HasError() {
			return nil, fmt.Errorf("invalid collection: %v", diags)
		}
		return result, nil
	case types.MapType, types.ObjectType:
		items, ok := raw.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("expected %s, got %T", expected, raw)
		}
		values := make(map[string]attr.Value, len(items))
		for key, item := range items {
			var elemType attr.Type
			switch typ := typ.(type) {
			case types.MapType:
				elemType = typ.ElemType
			case types.ObjectType:
				elemType = typ.AttrTypes[key]
			}
			value, err := literalValueFromInterface(ctx, item, elemType)
			if err != nil {
				return nil, fmt.Errorf("attribute %s: %w", key, err)
			}
			values[key] = value
		}
		var result attr.Value
		var diags diag.Diagnostics
		switch typ := typ.(type) {
		case types.MapType:
			result, diags = types.MapValue(typ.ElemType, values)
		case types.ObjectType:
			result, diags = types.ObjectValue(typ.AttrTypes, values)
		}
		if diags.HasError() {
			return nil, fmt.Errorf("invalid object or map: %v", diags)
		}
		return result, nil
	default:
		value, _, err := attrValueFromInterface(raw)
		if err != nil {
			return nil, err
		}
		tfValue, err := value.ToTerraformValue(ctx)
		if err != nil {
			return nil, err
		}
		return expected.ValueFromTerraform(ctx, tfValue)
	}
}
