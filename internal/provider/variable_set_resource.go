// Copyright IBM Corp. 2021, 2026

package provider

import (
	"context"
	"fmt"

	apiv1 "buf.build/gen/go/ctrlplane/ctrlplane/protocolbuffers/go/ctrlplane/api/v1"
	connect "connectrpc.com/connect"
	"github.com/ctrlplanedev/terraform-provider-ctrlplane/internal/api"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	structpb "google.golang.org/protobuf/types/known/structpb"
)

var _ resource.Resource = &VariableSetResource{}
var _ resource.ResourceWithImportState = &VariableSetResource{}
var _ resource.ResourceWithConfigure = &VariableSetResource{}

func NewVariableSetResource() resource.Resource {
	return &VariableSetResource{}
}

type VariableSetResource struct {
	workspace *api.WorkspaceClient
}

type VariableSetResourceModel struct {
	ID          types.String   `tfsdk:"id"`
	Name        types.String   `tfsdk:"name"`
	Description types.String   `tfsdk:"description"`
	Selector    CELStringValue `tfsdk:"selector"`
	Priority    types.Int64    `tfsdk:"priority"`
	Variables   types.List     `tfsdk:"variables"`
}

type VariableSetVariableModel struct {
	Key            types.String `tfsdk:"key"`
	Value          types.String `tfsdk:"value"`
	Sensitive      types.Bool   `tfsdk:"sensitive"`
	ReferenceValue types.Object `tfsdk:"reference_value"`
}

var variableSetVariableAttrTypes = map[string]attr.Type{
	"key":       types.StringType,
	"value":     types.StringType,
	"sensitive": types.BoolType,
	"reference_value": types.ObjectType{
		AttrTypes: referenceValueAttrTypes,
	},
}

func (r *VariableSetResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_variable_set"
}

func (r *VariableSetResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func (r *VariableSetResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *VariableSetResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a variable set in Ctrlplane. Variable sets allow you to define groups of variables that are applied to release targets matching a selector expression.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The ID of the variable set.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The name of the variable set.",
			},
			"description": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "A description of the variable set.",
			},
			"selector": schema.StringAttribute{
				CustomType:          CELStringType{},
				Required:            true,
				MarkdownDescription: "A CEL expression to select which release targets this variable set applies to.",
				PlanModifiers: []planmodifier.String{
					celNormalized(),
				},
			},
			"priority": schema.Int64Attribute{
				Optional:            true,
				Computed:            true,
				Default:             int64default.StaticInt64(0),
				MarkdownDescription: "The priority of the variable set. Higher priority sets take precedence.",
			},
			"variables": schema.ListNestedAttribute{
				Optional:            true,
				MarkdownDescription: "The variables in this variable set.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"key": schema.StringAttribute{
							Required:            true,
							MarkdownDescription: "The key of the variable.",
						},
						"value": schema.StringAttribute{
							Optional:            true,
							MarkdownDescription: "The literal value as a string. Numbers, booleans, and JSON objects are also accepted and will be sent with their appropriate types. Conflicts with `reference_value`.",
						},
						"sensitive": schema.BoolAttribute{
							Optional:            true,
							MarkdownDescription: "Whether the value is sensitive. When true, the value is stored as a sensitive value. Conflicts with `reference_value`.",
						},
						"reference_value": schema.SingleNestedAttribute{
							Optional:            true,
							MarkdownDescription: "A reference value pointing to a property on the matched resource. Conflicts with `value`.",
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
				},
			},
		},
	}
}

func (r *VariableSetResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data VariableSetResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	variables, diags := vsVariablesFromModel(ctx, data)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	priority := int32(data.Priority.ValueInt64())
	created, err := r.workspace.VariableSet.CreateVariableSet(ctx, connect.NewRequest(&apiv1.CreateVariableSetRequest{
		WorkspaceId: r.workspace.WorkspaceID(),
		Name:        data.Name.ValueString(),
		Selector:    data.Selector.ValueString(),
		Description: data.Description.ValueStringPointer(),
		Priority:    &priority,
		Variables:   variables,
	}))
	if err != nil {
		addConnectError(&resp.Diagnostics, "Failed to create variable set", err)
		return
	}

	vs := created.Msg
	if vs.GetId() == "" {
		resp.Diagnostics.AddError("Failed to create variable set", "Empty variable set ID in response")
		return
	}

	// CreateVariableSet returns a VariableSet without variables. Set the ID and
	// scalar fields from the response and keep the planned variables in state.
	data.ID = types.StringValue(vs.GetId())

	resp.Diagnostics.Append(resp.State.Set(ctx, data)...)
}

func (r *VariableSetResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data VariableSetResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	got, err := r.workspace.VariableSet.GetVariableSet(ctx, connect.NewRequest(&apiv1.GetVariableSetRequest{
		WorkspaceId:   r.workspace.WorkspaceID(),
		VariableSetId: data.ID.ValueString(),
	}))
	if err != nil {
		if isNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		addConnectError(&resp.Diagnostics, "Failed to read variable set", err)
		return
	}

	vs := got.Msg
	if vs.GetId() == "" {
		resp.Diagnostics.AddError("Failed to read variable set", "Empty response from server")
		return
	}

	data.ID = types.StringValue(vs.GetId())
	data.Name = types.StringValue(vs.GetName())
	data.Description = optionalString(vs.GetDescription())
	data.Selector = celStringValue(vs.GetSelector())
	data.Priority = types.Int64Value(int64(vs.GetPriority()))

	varList, diags := vsVariablesToModel(vs.GetVariables())
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	data.Variables = varList

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *VariableSetResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data VariableSetResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	variables, diags := vsVariablesFromModel(ctx, data)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	name := data.Name.ValueString()
	priority := int32(data.Priority.ValueInt64())
	selector := data.Selector.ValueString()

	updated, err := r.workspace.VariableSet.UpdateVariableSet(ctx, connect.NewRequest(&apiv1.UpdateVariableSetRequest{
		WorkspaceId:   r.workspace.WorkspaceID(),
		VariableSetId: data.ID.ValueString(),
		Name:          &name,
		Description:   data.Description.ValueStringPointer(),
		Selector:      &selector,
		Priority:      &priority,
		Variables:     &apiv1.VariableSetVariableList{Variables: variables},
	}))
	if err != nil {
		addConnectError(&resp.Diagnostics, "Failed to update variable set", err)
		return
	}

	if id := updated.Msg.GetId(); id != "" {
		data.ID = types.StringValue(id)
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, data)...)
}

func (r *VariableSetResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data VariableSetResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	_, err := r.workspace.VariableSet.DeleteVariableSet(ctx, connect.NewRequest(&apiv1.DeleteVariableSetRequest{
		WorkspaceId:   r.workspace.WorkspaceID(),
		VariableSetId: data.ID.ValueString(),
	}))
	if err != nil && !isNotFound(err) {
		addConnectError(&resp.Diagnostics, "Failed to delete variable set", err)
		return
	}
}

// vsVariablesFromModel converts the Terraform list of variables into the proto
// VariableSetVariableInput slice carried by the Connect API.
func vsVariablesFromModel(ctx context.Context, data VariableSetResourceModel) ([]*apiv1.VariableSetVariableInput, diag.Diagnostics) {
	var diags diag.Diagnostics

	if data.Variables.IsNull() || data.Variables.IsUnknown() {
		return []*apiv1.VariableSetVariableInput{}, diags
	}

	var models []VariableSetVariableModel
	diags.Append(data.Variables.ElementsAs(ctx, &models, false)...)
	if diags.HasError() {
		return nil, diags
	}

	variables := make([]*apiv1.VariableSetVariableInput, 0, len(models))
	for _, m := range models {
		value, err := vsVariableValueFromModel(m)
		if err != nil {
			diags.AddError("Failed to convert variable value", fmt.Sprintf("Variable '%s': %s", m.Key.ValueString(), err.Error()))
			return nil, diags
		}

		variables = append(variables, &apiv1.VariableSetVariableInput{
			Key:   m.Key.ValueString(),
			Value: value,
		})
	}

	return variables, diags
}

// vsVariableValueFromModel encodes a single variable model into the *structpb.Value
// carried by the proto API. The three mutually exclusive forms are encoded so they
// can be recovered on Read:
//   - reference_value -> map { "reference": <string>, "path": [<strings>] }
//   - sensitive value -> map { "sensitive": true }
//   - literal value   -> the value string, as a structpb string value
func vsVariableValueFromModel(m VariableSetVariableModel) (*structpb.Value, error) {
	if !m.ReferenceValue.IsNull() && !m.ReferenceValue.IsUnknown() {
		refAttrs := m.ReferenceValue.Attributes()

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

	if !m.Value.IsNull() && !m.Value.IsUnknown() {
		if m.Sensitive.ValueBool() {
			sensValue, err := structpb.NewValue(map[string]any{"sensitive": true})
			if err != nil {
				return nil, fmt.Errorf("failed to build sensitive value: %w", err)
			}
			return sensValue, nil
		}

		litValue, err := structpb.NewValue(m.Value.ValueString())
		if err != nil {
			return nil, fmt.Errorf("failed to build literal value: %w", err)
		}
		return litValue, nil
	}

	return nil, fmt.Errorf("one of value or reference_value must be provided")
}

// vsVariablesToModel converts proto variables back into a Terraform list for state,
// inverting the encoding performed by vsVariableValueFromModel.
func vsVariablesToModel(variables []*apiv1.VariableSetVariable) (types.List, diag.Diagnostics) {
	var diags diag.Diagnostics

	if len(variables) == 0 {
		return types.ListNull(types.ObjectType{AttrTypes: variableSetVariableAttrTypes}), diags
	}

	elems := make([]attr.Value, 0, len(variables))
	for _, v := range variables {
		strVal := types.StringNull()
		sensitiveVal := types.BoolNull()
		refVal := types.ObjectNull(referenceValueAttrTypes)

		decoded := v.GetValue().AsInterface()

		switch d := decoded.(type) {
		case map[string]any:
			ref, hasRef := d["reference"]
			rawPath, hasPath := d["path"]
			refStr, refIsString := ref.(string)
			if sens, ok := d["sensitive"].(bool); ok && sens {
				// Sensitive value marker.
				sensitiveVal = types.BoolValue(true)
			} else if hasRef && hasPath && refIsString && refStr != "" {
				pathSlice, _ := rawPath.([]any)
				pathElements := make([]attr.Value, len(pathSlice))
				for i, p := range pathSlice {
					ps, _ := p.(string)
					pathElements[i] = types.StringValue(ps)
				}
				pathList, listDiags := types.ListValue(types.StringType, pathElements)
				if listDiags.HasError() {
					diags.Append(listDiags...)
					return types.ListNull(types.ObjectType{AttrTypes: variableSetVariableAttrTypes}), diags
				}
				obj, objDiags := types.ObjectValue(referenceValueAttrTypes, map[string]attr.Value{
					"reference": types.StringValue(refStr),
					"path":      pathList,
				})
				if objDiags.HasError() {
					diags.Append(objDiags...)
					return types.ListNull(types.ObjectType{AttrTypes: variableSetVariableAttrTypes}), diags
				}
				refVal = obj
			} else {
				// An unexpected map: render as a literal string for state stability.
				strVal = types.StringValue(fmt.Sprintf("%v", decoded))
			}
		case string:
			strVal = types.StringValue(d)
		case nil:
			// Leave all fields null.
		default:
			// Numbers, booleans, etc. were sent as strings; render back to string.
			strVal = types.StringValue(fmt.Sprintf("%v", d))
		}

		obj, objDiags := types.ObjectValue(variableSetVariableAttrTypes, map[string]attr.Value{
			"key":             types.StringValue(v.GetKey()),
			"value":           strVal,
			"sensitive":       sensitiveVal,
			"reference_value": refVal,
		})
		if objDiags.HasError() {
			diags.Append(objDiags...)
			return types.ListNull(types.ObjectType{AttrTypes: variableSetVariableAttrTypes}), diags
		}
		elems = append(elems, obj)
	}

	list, listDiags := types.ListValue(types.ObjectType{AttrTypes: variableSetVariableAttrTypes}, elems)
	diags.Append(listDiags...)
	return list, diags
}
