// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"os"

	"github.com/ctrlplanedev/terraform-provider-ctrlplane/internal/api"
	"github.com/hashicorp/terraform-plugin-framework/action"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/ephemeral"
	"github.com/hashicorp/terraform-plugin-framework/function"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Ensure CtrlplaneProvider satisfies various provider interfaces.
var _ provider.Provider = &CtrlplaneProvider{}
var _ provider.ProviderWithFunctions = &CtrlplaneProvider{}
var _ provider.ProviderWithEphemeralResources = &CtrlplaneProvider{}
var _ provider.ProviderWithActions = &CtrlplaneProvider{}

// CtrlplaneProvider defines the provider implementation.
type CtrlplaneProvider struct {
	// version is set to the provider version on release, "dev" when the
	// provider is built and ran locally, and "test" when running acceptance
	// testing.
	version string
}

// CtrlplaneProviderModel describes the provider data model.
type CtrlplaneProviderModel struct {
	URL       types.String `tfsdk:"url"`
	ApiKey    types.String `tfsdk:"api_key"`
	Workspace types.String `tfsdk:"workspace"`
}

func (p *CtrlplaneProvider) Metadata(ctx context.Context, req provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "ctrlplane"
	resp.Version = p.version
}

func (p *CtrlplaneProvider) Schema(ctx context.Context, req provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			"workspace": schema.StringAttribute{
				Description:         "The workspace to use. Can be set in the CTRLPLANE_WORKSPACE environment variable. Can be a workspace ID or slug.",
				MarkdownDescription: "The workspace to use. Can be set in the CTRLPLANE_WORKSPACE environment variable. Can be a workspace ID or slug.",
				Optional:            true,
			},
			"url": schema.StringAttribute{
				Description:         "The URL of the Ctrlplane endpoint. Can be set in the CTRLPLANE_URL environment variable. Defaults to `https://app.ctrlplane.dev` if not set.",
				MarkdownDescription: "The URL of the Ctrlplane endpoint. Can be set in the CTRLPLANE_URL environment variable. Defaults to `https://app.ctrlplane.dev` if not set.",
				Optional:            true,
			},
			"api_key": schema.StringAttribute{
				Description:         "The token to use for authentication. Can be set in the CTRLPLANE_API_KEY environment variable.",
				MarkdownDescription: "The token to use for authentication. Can be set in the CTRLPLANE_API_KEY environment variable.",
				Optional:            true,
				Sensitive:           true,
			},
		},
	}
}

func (p *CtrlplaneProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var data CtrlplaneProviderModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	if data.URL.IsNull() {
		envURL := os.Getenv("CTRLPLANE_URL")
		if envURL != "" {
			data.URL = types.StringValue(envURL)
		} else {
			data.URL = types.StringValue("https://app.ctrlplane.dev")
		}
	}

	if data.ApiKey.IsNull() {
		envAPIKey := os.Getenv("CTRLPLANE_API_KEY")
		if envAPIKey == "" {
			resp.Diagnostics.AddError("API key not set", "The CTRLPLANE_API_KEY environment variable is not set")
			return
		}
		data.ApiKey = types.StringValue(envAPIKey)
	}

	// Set Workspace from environment if not provided.
	if data.Workspace.IsNull() {
		envWorkspace := os.Getenv("CTRLPLANE_WORKSPACE")
		if envWorkspace == "" {
			resp.Diagnostics.AddError("Workspace not set", "The workspace must be set either in the provider configuration or in the CTRLPLANE_WORKSPACE environment variable")
			return
		}
		data.Workspace = types.StringValue(envWorkspace)
	}

	client, err := api.NewWorkspaceClient(data.URL.ValueString(), data.ApiKey.ValueString(), data.Workspace.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Failed to create client", err.Error())
		return
	}

	// Example client configuration for data sources and resources
	resp.DataSourceData = client
	resp.ResourceData = client
}

func (p *CtrlplaneProvider) Resources(ctx context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		NewSystemResource,
		NewEnvironmentResource,
		NewDeploymentResource,
		NewJobAgentResource,
		NewDeploymentVariableResource,
		NewDeploymentVariableValueResource,
		NewPolicyResource,
		NewResourceResource,
		NewResourceProviderResource,
		NewRelationshipRuleResource,
		NewEnvironmentSystemLinkResource,
		NewDeploymentSystemLinkResource,
		NewWorkflowResource,
		NewVariableSetResource,
		NewSecretResource,
		NewSecretProviderResource,
	}
}

func (p *CtrlplaneProvider) EphemeralResources(ctx context.Context) []func() ephemeral.EphemeralResource {
	return []func() ephemeral.EphemeralResource{}
}

func (p *CtrlplaneProvider) DataSources(ctx context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{
		NewEnvironmentDataSource,
		NewDeploymentDataSource,
	}
}

func (p *CtrlplaneProvider) Functions(ctx context.Context) []func() function.Function {
	return []func() function.Function{}
}

func (p *CtrlplaneProvider) Actions(ctx context.Context) []func() action.Action {
	return []func() action.Action{}
}

func New(version string) func() provider.Provider {
	return func() provider.Provider {
		return &CtrlplaneProvider{
			version: version,
		}
	}
}
