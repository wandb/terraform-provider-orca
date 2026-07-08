// Copyright IBM Corp. 2021, 2026

package provider

import (
	"context"
	"fmt"
	"math"
	"math/big"

	apiv1 "buf.build/gen/go/ctrlplane/ctrlplane/protocolbuffers/go/ctrlplane/api/v1"
	connect "connectrpc.com/connect"
	"github.com/ctrlplanedev/terraform-provider-ctrlplane/internal/api"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

var _ resource.Resource = &DeploymentVariableResource{}
var _ resource.ResourceWithImportState = &DeploymentVariableResource{}
var _ resource.ResourceWithConfigure = &DeploymentVariableResource{}

func NewDeploymentVariableResource() resource.Resource {
	return &DeploymentVariableResource{}
}

type DeploymentVariableResource struct {
	workspace *api.WorkspaceClient
}

func (r *DeploymentVariableResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_deployment_variable"
}

func (r *DeploymentVariableResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func (r *DeploymentVariableResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *DeploymentVariableResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:    true,
				Description: "The ID of the deployment variable",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"deployment_id": schema.StringAttribute{
				Required:    true,
				Description: "The deployment ID this variable belongs to",
			},
			"key": schema.StringAttribute{
				Required:    true,
				Description: "The variable key",
			},
			"description": schema.StringAttribute{
				Optional:    true,
				Description: "The variable description",
			},
		},
	}
}

func (r *DeploymentVariableResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data DeploymentVariableResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	created, err := r.workspace.Deployment.CreateDeploymentVariable(ctx, connect.NewRequest(&apiv1.CreateDeploymentVariableRequest{
		WorkspaceId:  r.workspace.WorkspaceID(),
		DeploymentId: data.DeploymentId.ValueString(),
		Key:          data.Key.ValueString(),
		Description:  data.Description.ValueStringPointer(),
	}))
	if err != nil {
		addConnectError(&resp.Diagnostics, "Failed to create deployment variable", err)
		return
	}

	applyDeploymentVariable(&data, created.Msg)

	resp.Diagnostics.Append(resp.State.Set(ctx, data)...)
}

func (r *DeploymentVariableResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data DeploymentVariableResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	got, err := r.workspace.Deployment.GetDeploymentVariable(ctx, connect.NewRequest(&apiv1.GetDeploymentVariableRequest{
		WorkspaceId: r.workspace.WorkspaceID(),
		VariableId:  data.ID.ValueString(),
	}))
	if err != nil {
		if isNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		addConnectError(&resp.Diagnostics, "Failed to read deployment variable", err)
		return
	}

	variable := got.Msg.GetVariable()
	if variable == nil || variable.GetId() == "" {
		resp.Diagnostics.AddError("Failed to read deployment variable", "Empty response from server")
		return
	}

	applyDeploymentVariable(&data, variable)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func applyDeploymentVariable(data *DeploymentVariableResourceModel, v *apiv1.DeploymentVariable) {
	data.ID = types.StringValue(v.GetId())
	data.DeploymentId = types.StringValue(v.GetDeploymentId())
	data.Key = types.StringValue(v.GetKey())
	data.Description = optionalString(v.GetDescription())
}

func (r *DeploymentVariableResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data DeploymentVariableResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	updated, err := r.workspace.Deployment.UpdateDeploymentVariable(ctx, connect.NewRequest(&apiv1.UpdateDeploymentVariableRequest{
		WorkspaceId:  r.workspace.WorkspaceID(),
		VariableId:   data.ID.ValueString(),
		DeploymentId: data.DeploymentId.ValueString(),
		Key:          data.Key.ValueString(),
		Description:  data.Description.ValueStringPointer(),
	}))
	if err != nil {
		addConnectError(&resp.Diagnostics, "Failed to update deployment variable", err)
		return
	}

	applyDeploymentVariable(&data, updated.Msg)

	resp.Diagnostics.Append(resp.State.Set(ctx, data)...)
}

func (r *DeploymentVariableResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data DeploymentVariableResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	_, err := r.workspace.Deployment.DeleteDeploymentVariable(ctx, connect.NewRequest(&apiv1.DeleteDeploymentVariableRequest{
		WorkspaceId: r.workspace.WorkspaceID(),
		VariableId:  data.ID.ValueString(),
	}))
	if err != nil && !isNotFound(err) {
		addConnectError(&resp.Diagnostics, "Failed to delete deployment variable", err)
		return
	}
}

type DeploymentVariableResourceModel struct {
	ID           types.String `tfsdk:"id"`
	DeploymentId types.String `tfsdk:"deployment_id"`
	Key          types.String `tfsdk:"key"`
	Description  types.String `tfsdk:"description"`
}

func attrValueFromInterface(value interface{}) (attr.Value, attr.Type, error) {
	switch v := value.(type) {
	case nil:
		return types.DynamicNull(), types.DynamicType, nil
	case bool:
		return types.BoolValue(v), types.BoolType, nil
	case string:
		return types.StringValue(v), types.StringType, nil
	case int:
		return types.Int64Value(int64(v)), types.Int64Type, nil
	case int32:
		return types.Int64Value(int64(v)), types.Int64Type, nil
	case int64:
		return types.Int64Value(v), types.Int64Type, nil
	case float32:
		return types.Float64Value(float64(v)), types.Float64Type, nil
	case float64:
		if math.Trunc(v) == v {
			return types.Int64Value(int64(v)), types.Int64Type, nil
		}
		return types.Float64Value(v), types.Float64Type, nil
	case map[string]any:
		attrTypes := make(map[string]attr.Type, len(v))
		attrValues := make(map[string]attr.Value, len(v))
		for key, raw := range v {
			convertedValue, convertedType, err := attrValueFromInterface(raw)
			if err != nil {
				return nil, nil, err
			}
			attrTypes[key] = convertedType
			attrValues[key] = convertedValue
		}
		obj, diags := types.ObjectValue(attrTypes, attrValues)
		if diags.HasError() {
			return nil, nil, fmt.Errorf("failed to build object value")
		}
		return obj, obj.Type(context.Background()), nil
	case []interface{}:
		return nil, nil, fmt.Errorf("unsupported value type []interface{}")
	default:
		return nil, nil, fmt.Errorf("unsupported value type %T", value)
	}
}

func terraformValueToInterface(value tftypes.Value) (interface{}, error) {
	if !value.IsKnown() {
		return nil, nil
	}
	if value.IsNull() {
		return nil, nil
	}

	if tftypes.String.Equal(value.Type()) {
		var decoded string
		if err := value.As(&decoded); err != nil {
			return nil, err
		}
		return decoded, nil
	}
	if tftypes.Bool.Equal(value.Type()) {
		var decoded bool
		if err := value.As(&decoded); err != nil {
			return nil, err
		}
		return decoded, nil
	}
	if tftypes.Number.Equal(value.Type()) {
		var decoded *big.Float
		if err := value.As(&decoded); err != nil {
			return nil, err
		}
		if decoded == nil {
			return nil, nil
		}
		if decoded.IsInt() {
			integer, _ := decoded.Int64()
			return integer, nil
		}
		floatVal, _ := decoded.Float64()
		return floatVal, nil
	}

	switch value.Type().(type) {
	case tftypes.Object:
		var decoded map[string]tftypes.Value
		if err := value.As(&decoded); err != nil {
			return nil, err
		}
		result := make(map[string]interface{}, len(decoded))
		for key, raw := range decoded {
			converted, err := terraformValueToInterface(raw)
			if err != nil {
				return nil, err
			}
			result[key] = converted
		}
		return result, nil
	case tftypes.Map:
		var decoded map[string]tftypes.Value
		if err := value.As(&decoded); err != nil {
			return nil, err
		}
		result := make(map[string]interface{}, len(decoded))
		for key, raw := range decoded {
			converted, err := terraformValueToInterface(raw)
			if err != nil {
				return nil, err
			}
			result[key] = converted
		}
		return result, nil
	case tftypes.List, tftypes.Tuple, tftypes.Set:
		var decoded []tftypes.Value
		if err := value.As(&decoded); err != nil {
			return nil, err
		}
		result := make([]interface{}, 0, len(decoded))
		for _, raw := range decoded {
			converted, err := terraformValueToInterface(raw)
			if err != nil {
				return nil, err
			}
			result = append(result, converted)
		}
		return result, nil
	default:
		return nil, fmt.Errorf("unsupported terraform value type %s", value.Type().String())
	}
}
