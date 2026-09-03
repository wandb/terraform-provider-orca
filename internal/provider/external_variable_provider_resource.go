// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"encoding/json"
	"fmt"

	apiv1 "buf.build/gen/go/orca/orca/protocolbuffers/go/orca/api/v1"
	"connectrpc.com/connect"
	"github.com/ctrlplanedev/terraform-provider-ctrlplane/internal/api"
	providervalidator "github.com/ctrlplanedev/terraform-provider-ctrlplane/internal/validator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	schemavalidator "github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"google.golang.org/protobuf/types/known/structpb"
)

var (
	_ resource.Resource                   = &ExternalVariableProviderResource{}
	_ resource.ResourceWithConfigure      = &ExternalVariableProviderResource{}
	_ resource.ResourceWithImportState    = &ExternalVariableProviderResource{}
	_ resource.ResourceWithValidateConfig = &ExternalVariableProviderResource{}
)

func NewExternalVariableProviderResource() resource.Resource {
	return &ExternalVariableProviderResource{}
}

type ExternalVariableProviderResource struct {
	workspace *api.WorkspaceClient
}

type ExternalVariableProviderResourceModel struct {
	ID      types.String                           `tfsdk:"id"`
	Name    types.String                           `tfsdk:"name"`
	Statsig []ExternalVariableProviderStatsigModel `tfsdk:"statsig"`
	Custom  []ExternalVariableProviderCustomModel  `tfsdk:"custom"`
}

type ExternalVariableProviderStatsigModel struct {
	Endpoint   types.String `tfsdk:"endpoint"`
	Token      types.String `tfsdk:"token"`
	ConsoleURL types.String `tfsdk:"console_url"`
}

type ExternalVariableProviderCustomModel struct {
	Type   types.String `tfsdk:"type"`
	Config types.String `tfsdk:"config"`
}

func (r *ExternalVariableProviderResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_external_variable_provider"
}

func (r *ExternalVariableProviderResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func (r *ExternalVariableProviderResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *ExternalVariableProviderResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a reusable external variable provider in Ctrlplane.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The ID of the external variable provider.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The name of the external variable provider.",
			},
		},
		Blocks: map[string]schema.Block{
			"statsig": schema.ListNestedBlock{
				MarkdownDescription: "Statsig connection configuration.",
				NestedObject: schema.NestedBlockObject{
					Attributes: map[string]schema.Attribute{
						"endpoint": schema.StringAttribute{
							Required:            true,
							MarkdownDescription: "The Statsig API endpoint, such as `https://api.statsig.com`.",
						},
						"token": schema.StringAttribute{
							Required:            true,
							MarkdownDescription: "A Ctrlplane secret expression containing the Statsig server API key, such as `{{ secret \"statsig-server-key\" }}`.",
						},
						"console_url": schema.StringAttribute{
							Optional:            true,
							MarkdownDescription: "Optional Statsig project console URL used to link gates back to Statsig.",
						},
					},
				},
			},
			"custom": schema.ListNestedBlock{
				MarkdownDescription: "Provider-neutral configuration for an external variable provider type not modeled by this Terraform provider.",
				NestedObject: schema.NestedBlockObject{
					Attributes: map[string]schema.Attribute{
						"type": schema.StringAttribute{
							Required:            true,
							MarkdownDescription: "The external variable provider type.",
						},
						"config": schema.StringAttribute{
							Required:            true,
							MarkdownDescription: "Provider-specific configuration as a JSON object.",
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

func (r *ExternalVariableProviderResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var data ExternalVariableProviderResourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	count := len(data.Statsig) + len(data.Custom)
	if count != 1 {
		resp.Diagnostics.AddError(
			"Invalid external variable provider configuration",
			"Exactly one statsig or custom block must be configured.",
		)
	}
}

func (r *ExternalVariableProviderResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data ExternalVariableProviderResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	providerType, config, err := externalVariableProviderConfigFromModel(data)
	if err != nil {
		resp.Diagnostics.AddError("Failed to create external variable provider", err.Error())
		return
	}
	configStruct, err := structpb.NewStruct(config)
	if err != nil {
		resp.Diagnostics.AddError("Failed to create external variable provider", fmt.Sprintf("Invalid provider config: %s", err))
		return
	}

	created, err := r.workspace.ExternalVariableProvider.CreateExternalVariableProvider(ctx, connect.NewRequest(&apiv1.CreateExternalVariableProviderRequest{
		WorkspaceId: r.workspace.WorkspaceID(),
		Name:        data.Name.ValueString(),
		Type:        providerType,
		Config:      configStruct,
	}))
	if err != nil {
		addConnectError(&resp.Diagnostics, "Failed to create external variable provider", err)
		return
	}

	provider := created.Msg.GetExternalVariableProvider()
	if provider.GetId() == "" {
		resp.Diagnostics.AddError("Failed to create external variable provider", "Empty external variable provider ID in response")
		return
	}
	if err := applyExternalVariableProvider(&data, provider); err != nil {
		resp.Diagnostics.AddError("Failed to create external variable provider", err.Error())
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, data)...)
}

func (r *ExternalVariableProviderResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data ExternalVariableProviderResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	got, err := r.workspace.ExternalVariableProvider.GetExternalVariableProvider(ctx, connect.NewRequest(&apiv1.GetExternalVariableProviderRequest{
		WorkspaceId: r.workspace.WorkspaceID(),
		Id:          data.ID.ValueString(),
	}))
	if err != nil {
		if isNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		addConnectError(&resp.Diagnostics, "Failed to read external variable provider", err)
		return
	}

	provider := got.Msg.GetExternalVariableProvider()
	if provider.GetId() == "" {
		resp.Diagnostics.AddError("Failed to read external variable provider", "Empty external variable provider in response")
		return
	}
	if err := applyExternalVariableProvider(&data, provider); err != nil {
		resp.Diagnostics.AddError("Failed to read external variable provider", err.Error())
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *ExternalVariableProviderResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data ExternalVariableProviderResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	providerType, config, err := externalVariableProviderConfigFromModel(data)
	if err != nil {
		resp.Diagnostics.AddError("Failed to update external variable provider", err.Error())
		return
	}
	configStruct, err := structpb.NewStruct(config)
	if err != nil {
		resp.Diagnostics.AddError("Failed to update external variable provider", fmt.Sprintf("Invalid provider config: %s", err))
		return
	}

	updated, err := r.workspace.ExternalVariableProvider.UpdateExternalVariableProvider(ctx, connect.NewRequest(&apiv1.UpdateExternalVariableProviderRequest{
		WorkspaceId: r.workspace.WorkspaceID(),
		Id:          data.ID.ValueString(),
		Name:        data.Name.ValueString(),
		Type:        providerType,
		Config:      configStruct,
	}))
	if err != nil {
		addConnectError(&resp.Diagnostics, "Failed to update external variable provider", err)
		return
	}

	provider := updated.Msg.GetExternalVariableProvider()
	if provider.GetId() == "" {
		resp.Diagnostics.AddError("Failed to update external variable provider", "Empty external variable provider ID in response")
		return
	}
	if err := applyExternalVariableProvider(&data, provider); err != nil {
		resp.Diagnostics.AddError("Failed to update external variable provider", err.Error())
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, data)...)
}

func (r *ExternalVariableProviderResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data ExternalVariableProviderResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	_, err := r.workspace.ExternalVariableProvider.DeleteExternalVariableProvider(ctx, connect.NewRequest(&apiv1.DeleteExternalVariableProviderRequest{
		WorkspaceId: r.workspace.WorkspaceID(),
		Id:          data.ID.ValueString(),
	}))
	if err != nil && !isNotFound(err) {
		addConnectError(&resp.Diagnostics, "Failed to delete external variable provider", err)
	}
}

func externalVariableProviderConfigFromModel(data ExternalVariableProviderResourceModel) (string, map[string]any, error) {
	switch {
	case len(data.Statsig) == 1:
		statsig := data.Statsig[0]
		config := map[string]any{
			"endpoint": statsig.Endpoint.ValueString(),
			"token":    statsig.Token.ValueString(),
		}
		if !statsig.ConsoleURL.IsNull() && !statsig.ConsoleURL.IsUnknown() && statsig.ConsoleURL.ValueString() != "" {
			config["consoleUrl"] = statsig.ConsoleURL.ValueString()
		}
		return "statsig", config, nil
	case len(data.Custom) == 1:
		custom := data.Custom[0]
		config, err := jsonObjectFromString(custom.Config)
		if err != nil {
			return "", nil, fmt.Errorf("invalid custom config: %w", err)
		}
		return custom.Type.ValueString(), config, nil
	default:
		return "", nil, fmt.Errorf("exactly one statsig or custom block must be configured")
	}
}

func setExternalVariableProviderBlocksFromAPI(data *ExternalVariableProviderResourceModel, providerType string, config map[string]any) error {
	data.Statsig = nil
	data.Custom = nil

	if providerType == "statsig" {
		statsig := ExternalVariableProviderStatsigModel{
			Endpoint:   stringValueOrNull(config["endpoint"]),
			Token:      stringValueOrNull(config["token"]),
			ConsoleURL: types.StringNull(),
		}
		if consoleURL, ok := config["consoleUrl"]; ok && consoleURL != nil && fmt.Sprint(consoleURL) != "" {
			statsig.ConsoleURL = types.StringValue(fmt.Sprint(consoleURL))
		}
		data.Statsig = []ExternalVariableProviderStatsigModel{statsig}
		return nil
	}

	configJSON, err := json.Marshal(config)
	if err != nil {
		return fmt.Errorf("encode custom provider config: %w", err)
	}
	data.Custom = []ExternalVariableProviderCustomModel{{
		Type:   types.StringValue(providerType),
		Config: types.StringValue(string(configJSON)),
	}}
	return nil
}

func applyExternalVariableProvider(data *ExternalVariableProviderResourceModel, provider *apiv1.ExternalVariableProvider) error {
	if provider == nil {
		return fmt.Errorf("external variable provider is missing")
	}

	data.ID = types.StringValue(provider.GetId())
	data.Name = types.StringValue(provider.GetName())
	config := map[string]any{}
	if provider.GetConfig() != nil {
		config = provider.GetConfig().AsMap()
	}
	return setExternalVariableProviderBlocksFromAPI(data, provider.GetType(), config)
}

func jsonObjectFromString(value types.String) (map[string]any, error) {
	if value.IsNull() || value.IsUnknown() || value.ValueString() == "" {
		return nil, fmt.Errorf("must be a JSON object")
	}

	var decoded map[string]any
	if err := json.Unmarshal([]byte(value.ValueString()), &decoded); err != nil {
		return nil, fmt.Errorf("must be a JSON object: %w", err)
	}
	if decoded == nil {
		return nil, fmt.Errorf("must be a JSON object")
	}
	return decoded, nil
}
