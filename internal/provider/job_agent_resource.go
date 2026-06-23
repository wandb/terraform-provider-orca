// Copyright IBM Corp. 2021, 2026

package provider

import (
	"context"
	"fmt"

	apiv1 "buf.build/gen/go/ctrlplane/ctrlplane/protocolbuffers/go/ctrlplane/api/v1"
	connect "connectrpc.com/connect"
	"github.com/ctrlplanedev/terraform-provider-ctrlplane/internal/api"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/defaults"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/mapdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/mapplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"google.golang.org/protobuf/types/known/structpb"
)

var (
	_ resource.Resource                   = &JobAgentResource{}
	_ resource.ResourceWithImportState    = &JobAgentResource{}
	_ resource.ResourceWithConfigure      = &JobAgentResource{}
	_ resource.ResourceWithValidateConfig = &JobAgentResource{}
)

func NewJobAgentResource() resource.Resource {
	return &JobAgentResource{}
}

type JobAgentResource struct {
	workspace *api.WorkspaceClient
}

func (r *JobAgentResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_job_agent"
}

func (r *JobAgentResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func (r *JobAgentResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *JobAgentResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:    true,
				Description: "The ID of the job agent",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				Required:    true,
				Description: "The name of the job agent",
			},
			"metadata": schema.MapAttribute{
				Optional:    true,
				Computed:    true,
				Description: "The metadata of the job agent",
				ElementType: types.StringType,
				Default: func() defaults.Map {
					empty, _ := types.MapValueFrom(context.Background(), types.StringType, map[string]string{})
					return mapdefault.StaticValue(empty)
				}(),
				PlanModifiers: []planmodifier.Map{
					mapplanmodifier.UseStateForUnknown(),
				},
			},
		},
		Blocks: map[string]schema.Block{
			"custom": schema.ListNestedBlock{
				Description: "Custom job agent configuration",
				NestedObject: schema.NestedBlockObject{
					Attributes: map[string]schema.Attribute{
						"type": schema.StringAttribute{
							Required:    true,
							Description: "Job agent type",
						},
						"config": schema.MapAttribute{
							Required:    true,
							Description: "Job agent configuration",
							ElementType: types.StringType,
						},
					},
				},
			},
			"argocd": schema.ListNestedBlock{
				Description: "ArgoCD job agent configuration",
				NestedObject: schema.NestedBlockObject{
					Attributes: map[string]schema.Attribute{
						"api_key": schema.StringAttribute{
							Required:    true,
							Description: "ArgoCD API token",
							Sensitive:   true,
						},
						"server_url": schema.StringAttribute{
							Required:    true,
							Description: "ArgoCD server address (host[:port] or URL)",
						},
						"template": schema.StringAttribute{
							Required:    true,
							Description: "ArgoCD application template",
						},
					},
				},
			},

			"argo_workflow": schema.ListNestedBlock{
				Description: "ArgoWorkflow job agent configuration",
				NestedObject: schema.NestedBlockObject{
					Attributes: map[string]schema.Attribute{
						"api_key": schema.StringAttribute{
							Required:    true,
							Description: "ArgoWorkflow API token",
							Sensitive:   true,
						},
						"webhook_secret": schema.StringAttribute{
							Required:    true,
							Description: "Argo Events Webhook Secret",
							Sensitive:   true,
						},
						"server_url": schema.StringAttribute{
							Required:    true,
							Description: "ArgoWorkflow server address (host[:port] or URL)",
						},
						"template": schema.StringAttribute{
							Required:    true,
							Description: "ArgoWorkflow application template",
						},
						"name": schema.StringAttribute{
							Required:    true,
							Description: "ArgoWorkflow template name",
						},
						"http_insecure": schema.BoolAttribute{
							Optional:    true,
							Computed:    true,
							Description: "Allow insecure HTTP connections (defaults to false)",
							Default:     booldefault.StaticBool(false),
						},
					},
				},
			},

			"github": schema.ListNestedBlock{
				Description: "GitHub job agent configuration",
				NestedObject: schema.NestedBlockObject{
					Attributes: map[string]schema.Attribute{
						"installation_id": schema.Int64Attribute{
							Required:    true,
							Description: "GitHub app installation ID",
						},
						"owner": schema.StringAttribute{
							Required:    true,
							Description: "GitHub repository owner",
						},
						"repo": schema.StringAttribute{
							Required:    true,
							Description: "GitHub repository name",
						},
					},
				},
			},
			"terraform_cloud": schema.ListNestedBlock{
				Description: "Terraform Cloud job agent configuration",
				NestedObject: schema.NestedBlockObject{
					Attributes: map[string]schema.Attribute{
						"address": schema.StringAttribute{
							Required:    true,
							Description: "Terraform Cloud address (e.g. https://app.terraform.io)",
						},
						"organization": schema.StringAttribute{
							Required:    true,
							Description: "Terraform Cloud organization name",
						},
						"template": schema.StringAttribute{
							Required:    true,
							Description: "Terraform Cloud workspace template",
						},
						"token": schema.StringAttribute{
							Optional:    true,
							Description: "Terraform Cloud API token",
							Sensitive:   true,
						},
						"webhook_url": schema.StringAttribute{
							Required:    true,
							Description: "The ctrlplane API endpoint for TFC webhook notifications (e.g. https://ctrlplane.example.com/api/tfe/webhook)",
						},
						"trigger_run_on_change": schema.BoolAttribute{
							Optional:    true,
							Description: "Whether to create a TFC run on dispatch. When false, only the workspace and variables are synced. Defaults to true.",
						},
					},
				},
			},
			"test_runner": schema.ListNestedBlock{
				Description: "Test runner job agent configuration",
				NestedObject: schema.NestedBlockObject{
					Attributes: map[string]schema.Attribute{
						"delay_seconds": schema.Int64Attribute{
							Optional:    true,
							Description: "Delay in seconds before resolving the job",
						},
						"message": schema.StringAttribute{
							Optional:    true,
							Description: "Optional message to include in the job output",
						},
						"status": schema.StringAttribute{
							Optional:    true,
							Description: "Final status to set (e.g. \"successful\", \"failure\")",
						},
					},
				},
			},
			"http_pull": schema.ListNestedBlock{
				Description: "HTTP pull job agent configuration. An external agent polls for and claims its jobs over the REST API; ctrlplane does not push work to it. Takes no configuration.",
				NestedObject: schema.NestedBlockObject{
					Attributes: map[string]schema.Attribute{},
				},
			},
		},
	}
}

func (r *JobAgentResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var data JobAgentResourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	count := countJobAgentConfigs(data)
	if count == 0 {
		resp.Diagnostics.AddError(
			"Invalid job agent configuration",
			"Exactly one of custom, argocd, argo_workflow, github, terraform_cloud, test_runner, or http_pull must be set.",
		)
		return
	}
	if count > 1 {
		resp.Diagnostics.AddError(
			"Invalid job agent configuration",
			"Only one of custom, argocd, argo_workflow, github, terraform_cloud, test_runner, or http_pull can be set.",
		)
	}
}

func (r *JobAgentResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data JobAgentResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	jobAgentType, config, configErr := jobAgentConfigFromModel(data)
	if configErr != nil {
		resp.Diagnostics.AddError("Failed to create job agent", configErr.Error())
		return
	}
	if config == nil {
		resp.Diagnostics.AddError("Failed to create job agent", "Exactly one job agent type must be configured")
		return
	}

	configStruct, err := jobAgentConfigStruct(config)
	if err != nil {
		resp.Diagnostics.AddError("Failed to create job agent", err.Error())
		return
	}

	created, err := r.workspace.Job.UpsertJobAgent(ctx, connect.NewRequest(&apiv1.UpsertJobAgentRequest{
		WorkspaceId: r.workspace.WorkspaceID(),
		Name:        data.Name.ValueString(),
		Type:        jobAgentType,
		Config:      configStruct,
	}))
	if err != nil {
		addConnectError(&resp.Diagnostics, "Failed to create job agent", err)
		return
	}

	agentId := created.Msg.GetId()
	if agentId == "" {
		resp.Diagnostics.AddError("Failed to create job agent", "Empty job agent ID in response")
		return
	}

	data.ID = types.StringValue(agentId)

	got, err := r.workspace.Job.GetJobAgent(ctx, connect.NewRequest(&apiv1.GetJobAgentRequest{
		WorkspaceId: r.workspace.WorkspaceID(),
		JobAgentId:  agentId,
	}))
	if err != nil {
		addConnectError(&resp.Diagnostics, "Failed to read job agent after create", err)
		return
	}

	applyJobAgent(&data, got.Msg)

	resp.Diagnostics.Append(resp.State.Set(ctx, data)...)
}

func (r *JobAgentResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data JobAgentResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	got, err := r.workspace.Job.GetJobAgent(ctx, connect.NewRequest(&apiv1.GetJobAgentRequest{
		WorkspaceId: r.workspace.WorkspaceID(),
		JobAgentId:  data.ID.ValueString(),
	}))
	if err != nil {
		if isNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		addConnectError(&resp.Diagnostics, "Failed to read job agent", err)
		return
	}

	jobAgent := got.Msg
	if jobAgent.GetId() == "" {
		resp.Diagnostics.AddError("Failed to read job agent", "Empty job agent ID in response")
		return
	}

	applyJobAgent(&data, jobAgent)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

// applyJobAgent maps a proto JobAgent onto the model. Sensitive fields the API
// never returns (Terraform Cloud token, Argo Workflow apiKey/webhookSecret) are
// preserved from whatever is already in data — prior state on Read, the plan on
// Create/Update — since the server response cannot supply them.
func applyJobAgent(data *JobAgentResourceModel, jobAgent *apiv1.JobAgent) {
	data.ID = types.StringValue(jobAgent.GetId())
	data.Name = types.StringValue(jobAgent.GetName())
	if md := jobAgent.GetMetadata(); md == nil {
		empty, _ := types.MapValueFrom(context.Background(), types.StringType, map[string]string{})
		data.Metadata = empty
	} else {
		data.Metadata = stringMapValue(&md)
	}

	// Capture sensitive fields the API doesn't return before the blocks are
	// rebuilt from the server config.
	var priorToken types.String
	if len(data.TerraformCloud) > 0 {
		priorToken = data.TerraformCloud[0].Token
	}

	var priorArgoWorkflowApiKey, priorArgoWorkflowWebhookSecret types.String
	if len(data.ArgoWorkflow) > 0 {
		priorArgoWorkflowApiKey = data.ArgoWorkflow[0].ApiKey
		priorArgoWorkflowWebhookSecret = data.ArgoWorkflow[0].WebhookSecret
	}

	setJobAgentBlocksFromAPI(data, jobAgent.GetType(), jobAgentConfigMap(jobAgent.GetConfig()))

	if len(data.TerraformCloud) > 0 && !priorToken.IsNull() {
		data.TerraformCloud[0].Token = priorToken
	}
	if len(data.ArgoWorkflow) > 0 {
		data.ArgoWorkflow[0].ApiKey = priorArgoWorkflowApiKey
		data.ArgoWorkflow[0].WebhookSecret = priorArgoWorkflowWebhookSecret
	}
}

func (r *JobAgentResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data JobAgentResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	jobAgentType, config, configErr := jobAgentConfigFromModel(data)
	if configErr != nil {
		resp.Diagnostics.AddError("Failed to update job agent", configErr.Error())
		return
	}
	if config == nil {
		resp.Diagnostics.AddError("Failed to update job agent", "Exactly one job agent type must be configured")
		return
	}

	configStruct, err := jobAgentConfigStruct(config)
	if err != nil {
		resp.Diagnostics.AddError("Failed to update job agent", err.Error())
		return
	}

	upserted, err := r.workspace.Job.UpsertJobAgent(ctx, connect.NewRequest(&apiv1.UpsertJobAgentRequest{
		WorkspaceId: r.workspace.WorkspaceID(),
		JobAgentId:  data.ID.ValueString(),
		Name:        data.Name.ValueString(),
		Type:        jobAgentType,
		Config:      configStruct,
	}))
	if err != nil {
		addConnectError(&resp.Diagnostics, "Failed to update job agent", err)
		return
	}

	agentId := upserted.Msg.GetId()
	if agentId == "" {
		resp.Diagnostics.AddError("Failed to update job agent", "Empty job agent ID in response")
		return
	}

	data.ID = types.StringValue(agentId)

	got, err := r.workspace.Job.GetJobAgent(ctx, connect.NewRequest(&apiv1.GetJobAgentRequest{
		WorkspaceId: r.workspace.WorkspaceID(),
		JobAgentId:  agentId,
	}))
	if err != nil {
		addConnectError(&resp.Diagnostics, "Failed to read job agent after update", err)
		return
	}

	applyJobAgent(&data, got.Msg)

	resp.Diagnostics.Append(resp.State.Set(ctx, data)...)
}

func (r *JobAgentResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data JobAgentResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	_, err := r.workspace.Job.DeleteJobAgent(ctx, connect.NewRequest(&apiv1.DeleteJobAgentRequest{
		WorkspaceId: r.workspace.WorkspaceID(),
		JobAgentId:  data.ID.ValueString(),
	}))
	if err != nil && !isNotFound(err) {
		addConnectError(&resp.Diagnostics, "Failed to delete job agent", err)
		return
	}
}

type JobAgentResourceModel struct {
	ID             types.String                `tfsdk:"id"`
	Name           types.String                `tfsdk:"name"`
	Metadata       types.Map                   `tfsdk:"metadata"`
	Custom         []JobAgentCustomModel       `tfsdk:"custom"`
	ArgoCD         []JobAgentArgoCDModel       `tfsdk:"argocd"`
	ArgoWorkflow   []JobAgentArgoWorkflowModel `tfsdk:"argo_workflow"`
	GitHub         []JobAgentGitHubModel       `tfsdk:"github"`
	TerraformCloud []JobAgentTFCModel          `tfsdk:"terraform_cloud"`
	TestRunner     []JobAgentTestRunnerModel   `tfsdk:"test_runner"`
	HTTPPull       []JobAgentHTTPPullModel     `tfsdk:"http_pull"`
}

type JobAgentCustomModel struct {
	Type   types.String `tfsdk:"type"`
	Config types.Map    `tfsdk:"config"`
}

type JobAgentArgoCDModel struct {
	ApiKey    types.String `tfsdk:"api_key"`
	ServerUrl types.String `tfsdk:"server_url"`
	Template  types.String `tfsdk:"template"`
}

type JobAgentArgoWorkflowModel struct {
	ApiKey        types.String `tfsdk:"api_key"`
	WebhookSecret types.String `tfsdk:"webhook_secret"`
	ServerUrl     types.String `tfsdk:"server_url"`
	Template      types.String `tfsdk:"template"`
	Name          types.String `tfsdk:"name"`
	HttpInsecure  types.Bool   `tfsdk:"http_insecure"`
}
type JobAgentGitHubModel struct {
	InstallationId types.Int64  `tfsdk:"installation_id"`
	Owner          types.String `tfsdk:"owner"`
	Repo           types.String `tfsdk:"repo"`
}

type JobAgentTFCModel struct {
	Address            types.String `tfsdk:"address"`
	Organization       types.String `tfsdk:"organization"`
	Template           types.String `tfsdk:"template"`
	Token              types.String `tfsdk:"token"`
	WebhookUrl         types.String `tfsdk:"webhook_url"`
	TriggerRunOnChange types.Bool   `tfsdk:"trigger_run_on_change"`
}

type JobAgentTestRunnerModel struct {
	DelaySeconds types.Int64  `tfsdk:"delay_seconds"`
	Message      types.String `tfsdk:"message"`
	Status       types.String `tfsdk:"status"`
}

// JobAgentHTTPPullModel configures the http-pull agent. It carries no
// configuration; an external agent polls for and claims its jobs over the REST
// API. The presence of the block selects the agent type.
type JobAgentHTTPPullModel struct{}

func countJobAgentConfigs(data JobAgentResourceModel) int {
	count := 0
	if len(data.Custom) > 0 {
		count++
	}
	if len(data.ArgoCD) > 0 {
		count++
	}
	if len(data.ArgoWorkflow) > 0 {
		count++
	}
	if len(data.GitHub) > 0 {
		count++
	}
	if len(data.TerraformCloud) > 0 {
		count++
	}
	if len(data.TestRunner) > 0 {
		count++
	}
	if len(data.HTTPPull) > 0 {
		count++
	}
	return count
}

func jobAgentConfigFromModel(data JobAgentResourceModel) (string, *map[string]interface{}, error) {
	switch {
	case len(data.Custom) > 0:
		custom := data.Custom[0]
		customType := custom.Type.ValueString()
		if custom.Type.IsNull() || custom.Type.IsUnknown() || customType == "" {
			return "", nil, fmt.Errorf("custom.type is required")
		}
		config := stringInterfaceMapPointer(custom.Config)
		if config == nil {
			return "", nil, fmt.Errorf("custom.config must be a non-empty map")
		}
		return customType, config, nil
	case len(data.ArgoCD) > 0:
		argocd := data.ArgoCD[0]
		cfg := map[string]interface{}{
			"apiKey":    argocd.ApiKey.ValueString(),
			"serverUrl": argocd.ServerUrl.ValueString(),
			"template":  argocd.Template.ValueString(),
		}
		return "argo-cd", &cfg, nil
	case len(data.ArgoWorkflow) > 0:
		argoWorkflow := data.ArgoWorkflow[0]
		cfg := map[string]interface{}{
			"apiKey":        argoWorkflow.ApiKey.ValueString(),
			"webhookSecret": argoWorkflow.WebhookSecret.ValueString(),
			"serverUrl":     argoWorkflow.ServerUrl.ValueString(),
			"template":      argoWorkflow.Template.ValueString(),
			"name":          argoWorkflow.Name.ValueString(),
			"httpInsecure":  argoWorkflow.HttpInsecure.ValueBool(),
		}
		return "argo-workflow", &cfg, nil

	case len(data.GitHub) > 0:
		github := data.GitHub[0]
		cfg := map[string]interface{}{
			"installationId": github.InstallationId.ValueInt64(),
			"owner":          github.Owner.ValueString(),
			"repo":           github.Repo.ValueString(),
		}
		return "github-app", &cfg, nil
	case len(data.TerraformCloud) > 0:
		tfc := data.TerraformCloud[0]
		cfg := map[string]interface{}{
			"address":      tfc.Address.ValueString(),
			"organization": tfc.Organization.ValueString(),
			"template":     tfc.Template.ValueString(),
			"webhookUrl":   tfc.WebhookUrl.ValueString(),
		}
		if !tfc.Token.IsNull() && !tfc.Token.IsUnknown() && tfc.Token.ValueString() != "" {
			cfg["token"] = tfc.Token.ValueString()
		}
		if !tfc.TriggerRunOnChange.IsNull() && !tfc.TriggerRunOnChange.IsUnknown() {
			cfg["triggerRunOnChange"] = tfc.TriggerRunOnChange.ValueBool()
		}
		return "tfe", &cfg, nil
	case len(data.TestRunner) > 0:
		testRunner := data.TestRunner[0]
		cfg := map[string]interface{}{}
		if !testRunner.DelaySeconds.IsNull() && !testRunner.DelaySeconds.IsUnknown() {
			cfg["delaySeconds"] = testRunner.DelaySeconds.ValueInt64()
		}
		if !testRunner.Message.IsNull() && !testRunner.Message.IsUnknown() && testRunner.Message.ValueString() != "" {
			cfg["message"] = testRunner.Message.ValueString()
		}
		if !testRunner.Status.IsNull() && !testRunner.Status.IsUnknown() && testRunner.Status.ValueString() != "" {
			cfg["status"] = testRunner.Status.ValueString()
		}
		return "test-runner", &cfg, nil
	case len(data.HTTPPull) > 0:
		cfg := map[string]interface{}{}
		return "http-pull", &cfg, nil
	default:
		return "", nil, nil
	}
}

func setJobAgentBlocksFromAPI(data *JobAgentResourceModel, jobType string, config map[string]interface{}) {
	data.ArgoCD = nil
	data.ArgoWorkflow = nil
	data.GitHub = nil
	data.TerraformCloud = nil
	data.TestRunner = nil
	data.HTTPPull = nil
	data.Custom = nil

	switch jobType {
	case "argo-cd":
		data.ArgoCD = []JobAgentArgoCDModel{
			{
				ApiKey:    types.StringValue(fmt.Sprint(config["apiKey"])),
				ServerUrl: types.StringValue(fmt.Sprint(config["serverUrl"])),
				Template:  types.StringValue(fmt.Sprint(config["template"])),
			},
		}

	case "argo-workflow":
		httpInsecure := types.BoolValue(false)
		if v, ok := config["httpInsecure"]; ok {
			httpInsecure = boolValueOrNull(v)
		}
		argoWorkflow := JobAgentArgoWorkflowModel{
			ApiKey:        types.StringNull(),
			WebhookSecret: types.StringNull(),
			ServerUrl:     types.StringValue(fmt.Sprint(config["serverUrl"])),
			Template:      types.StringValue(fmt.Sprint(config["template"])),
			Name:          types.StringValue(fmt.Sprint(config["name"])),
			HttpInsecure:  httpInsecure,
		}
		data.ArgoWorkflow = []JobAgentArgoWorkflowModel{argoWorkflow}
	case "github-app":
		github := JobAgentGitHubModel{
			InstallationId: types.Int64Value(toInt64(config["installationId"])),
			Owner:          types.StringValue(fmt.Sprint(config["owner"])),
			Repo:           types.StringValue(fmt.Sprint(config["repo"])),
		}
		data.GitHub = []JobAgentGitHubModel{github}
	case "tfe":
		tfc := JobAgentTFCModel{
			Address:            stringValueOrNull(config["address"]),
			Organization:       stringValueOrNull(config["organization"]),
			Template:           stringValueOrNull(config["template"]),
			Token:              types.StringNull(),
			WebhookUrl:         stringValueOrNull(config["webhookUrl"]),
			TriggerRunOnChange: boolValueOrNull(config["triggerRunOnChange"]),
		}
		data.TerraformCloud = []JobAgentTFCModel{tfc}
	case "test-runner":
		testRunner := JobAgentTestRunnerModel{
			DelaySeconds: types.Int64Null(),
			Message:      types.StringNull(),
			Status:       types.StringNull(),
		}
		if delay, ok := config["delaySeconds"]; ok && delay != nil {
			testRunner.DelaySeconds = types.Int64Value(toInt64(delay))
		}
		if msg, ok := config["message"]; ok && msg != nil && fmt.Sprint(msg) != "" {
			testRunner.Message = types.StringValue(fmt.Sprint(msg))
		}
		if status, ok := config["status"]; ok && status != nil && fmt.Sprint(status) != "" {
			testRunner.Status = types.StringValue(fmt.Sprint(status))
		}
		data.TestRunner = []JobAgentTestRunnerModel{testRunner}
	case "http-pull":
		data.HTTPPull = []JobAgentHTTPPullModel{{}}
	default:
		data.Custom = []JobAgentCustomModel{
			{
				Type:   types.StringValue(jobType),
				Config: interfaceMapStringValue(config),
			},
		}
	}
}

// jobAgentConfigStruct converts the generic config map produced by
// jobAgentConfigFromModel into the *structpb.Struct expected by the proto
// request. A nil config yields a nil struct (an absent config).
func jobAgentConfigStruct(config *map[string]interface{}) (*structpb.Struct, error) {
	if config == nil {
		return nil, nil
	}
	s, err := structpb.NewStruct(*config)
	if err != nil {
		return nil, fmt.Errorf("invalid job agent config: %w", err)
	}
	return s, nil
}

// jobAgentConfigMap converts the *structpb.Struct returned by the proto API
// into the generic map consumed by setJobAgentBlocksFromAPI. A nil struct
// yields an empty map so downstream block mapping has a safe value to read.
func jobAgentConfigMap(config *structpb.Struct) map[string]interface{} {
	if config == nil {
		return map[string]interface{}{}
	}
	return config.AsMap()
}

func toInt64(value interface{}) int64 {
	switch v := value.(type) {
	case int:
		return int64(v)
	case int32:
		return int64(v)
	case int64:
		return v
	case float32:
		return int64(v)
	case float64:
		return int64(v)
	default:
		return 0
	}
}
