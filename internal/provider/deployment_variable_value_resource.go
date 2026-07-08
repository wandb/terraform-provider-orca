// Copyright IBM Corp. 2021, 2026

package provider

import (
	"context"
	"fmt"

	apiv1 "buf.build/gen/go/ctrlplane/ctrlplane/protocolbuffers/go/ctrlplane/api/v1"
	connect "connectrpc.com/connect"
	"github.com/ctrlplanedev/terraform-provider-ctrlplane/internal/api"
	"github.com/google/uuid"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
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
	ID               types.String  `tfsdk:"id"`
	VariableId       types.String  `tfsdk:"variable_id"`
	Priority         types.Int64   `tfsdk:"priority"`
	ResourceSelector types.String  `tfsdk:"resource_selector"`
	LiteralValue     types.Dynamic `tfsdk:"literal_value"`
	ReferenceValue   types.Object  `tfsdk:"reference_value"`
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
				Optional:            true,
				MarkdownDescription: "A CEL expression to select which resources this value applies to.",
				PlanModifiers: []planmodifier.String{
					celNormalized(),
				},
			},
			"literal_value": schema.DynamicAttribute{
				Optional:            true,
				MarkdownDescription: "A literal value (string, number, boolean, or object). Conflicts with `reference_value`. Numbers are transmitted as double-precision floats, so integers larger than 2^53 lose precision — pass such values as strings.",
			},
			"reference_value": schema.SingleNestedAttribute{
				Optional:            true,
				MarkdownDescription: "A reference value pointing to a property on the matched resource. Conflicts with `literal_value`.",
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

	if hasLiteral && hasReference {
		resp.Diagnostics.AddAttributeError(
			path.Root("literal_value"),
			"Conflicting value types",
			"Only one of literal_value or reference_value may be specified, not both.",
		)
	}

	if !hasLiteral && !hasReference {
		// Allow unknowns during plan - only error if both are definitively null
		if !data.LiteralValue.IsUnknown() && !data.ReferenceValue.IsUnknown() {
			resp.Diagnostics.AddError(
				"Missing value",
				"Exactly one of literal_value or reference_value must be specified.",
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

	protoValue, err := structpbValueFromModel(data)
	if err != nil {
		resp.Diagnostics.AddError("Failed to create deployment variable value", fmt.Sprintf("Failed to build value: %s", err.Error()))
		return
	}

	var selector *string
	if cel := normalizeCEL(data.ResourceSelector); cel != "" {
		selector = &cel
	}

	created, err := r.workspace.Deployment.UpsertDeploymentVariableValue(ctx, connect.NewRequest(&apiv1.UpsertDeploymentVariableValueRequest{
		WorkspaceId:          r.workspace.WorkspaceID(),
		ValueId:              valueID,
		DeploymentVariableId: data.VariableId.ValueString(),
		Priority:             data.Priority.ValueInt64(),
		ResourceSelector:     selector,
		Value:                protoValue,
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

	resp.Diagnostics.Append(applyDeploymentVariableValue(ctx, &data, got.Msg)...)
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

	value := got.Msg
	if value.GetId() == "" {
		resp.Diagnostics.AddError("Failed to read deployment variable value", "Empty response from server")
		return
	}

	resp.Diagnostics.Append(applyDeploymentVariableValue(ctx, &data, value)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

// applyDeploymentVariableValue maps a proto DeploymentVariableValue onto the
// model (id, variable id, priority, resource_selector, and the value union).
func applyDeploymentVariableValue(ctx context.Context, data *DeploymentVariableValueResourceModel, value *apiv1.DeploymentVariableValue) diag.Diagnostics {
	data.ID = types.StringValue(value.GetId())
	data.VariableId = types.StringValue(value.GetDeploymentVariableId())
	data.Priority = types.Int64Value(value.GetPriority())

	if selector := value.GetResourceSelector(); selector != "" {
		data.ResourceSelector = types.StringValue(selector)
	} else {
		data.ResourceSelector = types.StringNull()
	}

	return setValueOnModel(ctx, data, value.GetValue())
}

func (r *DeploymentVariableValueResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data DeploymentVariableValueResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	protoValue, err := structpbValueFromModel(data)
	if err != nil {
		resp.Diagnostics.AddError("Failed to update deployment variable value", fmt.Sprintf("Failed to build value: %s", err.Error()))
		return
	}

	var selector *string
	if cel := normalizeCEL(data.ResourceSelector); cel != "" {
		selector = &cel
	}

	upserted, err := r.workspace.Deployment.UpsertDeploymentVariableValue(ctx, connect.NewRequest(&apiv1.UpsertDeploymentVariableValueRequest{
		WorkspaceId:          r.workspace.WorkspaceID(),
		ValueId:              data.ID.ValueString(),
		DeploymentVariableId: data.VariableId.ValueString(),
		Priority:             data.Priority.ValueInt64(),
		ResourceSelector:     selector,
		Value:                protoValue,
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

	resp.Diagnostics.Append(applyDeploymentVariableValue(ctx, &data, got.Msg)...)
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
func setValueOnModel(_ context.Context, data *DeploymentVariableValueResourceModel, value *structpb.Value) diag.Diagnostics {
	var diags diag.Diagnostics

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

	attrValue, _, err := attrValueFromInterface(decoded)
	if err != nil {
		data.LiteralValue = types.DynamicNull()
		data.ReferenceValue = types.ObjectNull(referenceValueAttrTypes)
		return diags
	}

	data.LiteralValue = types.DynamicValue(attrValue)
	data.ReferenceValue = types.ObjectNull(referenceValueAttrTypes)
	return diags
}
