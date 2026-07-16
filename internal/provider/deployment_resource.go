// Copyright IBM Corp. 2021, 2026

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
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/defaults"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/mapdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"google.golang.org/protobuf/types/known/structpb"
)

var (
	_ resource.Resource                   = &DeploymentResource{}
	_ resource.ResourceWithImportState    = &DeploymentResource{}
	_ resource.ResourceWithConfigure      = &DeploymentResource{}
	_ resource.ResourceWithValidateConfig = &DeploymentResource{}
)

func NewDeploymentResource() resource.Resource {
	return &DeploymentResource{}
}

type DeploymentResource struct {
	workspace *api.WorkspaceClient
}

func (r *DeploymentResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_deployment"
}

func (r *DeploymentResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func (r *DeploymentResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *DeploymentResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:    true,
				Description: "The ID of the deployment",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				Required:    true,
				Description: "The name of the deployment",
			},
			"metadata": schema.MapAttribute{
				Optional:    true,
				Computed:    true,
				Description: "The metadata of the deployment",
				ElementType: types.StringType,
				Default: func() defaults.Map {
					empty, _ := types.MapValueFrom(context.Background(), types.StringType, map[string]string{})
					return mapdefault.StaticValue(empty)
				}(),
			},
			"resource_selector": schema.StringAttribute{
				Optional:    true,
				Description: "CEL expression used to select resources",
				PlanModifiers: []planmodifier.String{
					celNormalized(),
				},
			},
			"job_agent_selector": schema.StringAttribute{
				Optional:    true,
				Description: "CEL expression to match job agents",
			},
		},
		Blocks: map[string]schema.Block{
			"argocd": schema.SingleNestedBlock{
				Description: "ArgoCD job agent configuration",
				Attributes: map[string]schema.Attribute{
					"api_key":    schema.StringAttribute{Optional: true, Sensitive: true, Description: "ArgoCD API token"},
					"server_url": schema.StringAttribute{Optional: true, Description: "ArgoCD server address"},
					"template":   schema.StringAttribute{Optional: true, Description: "ArgoCD application template"},
				},
			},
			"argo_workflow": schema.SingleNestedBlock{
				Description: "Argo Workflow job agent configuration",
				Attributes: map[string]schema.Attribute{
					"api_key":        schema.StringAttribute{Optional: true, Sensitive: true, Description: "Argo Workflow API token"},
					"server_url":     schema.StringAttribute{Optional: true, Description: "Argo Workflow server address"},
					"template":       schema.StringAttribute{Optional: true, Description: "Argo Workflow application template"},
					"name":           schema.StringAttribute{Optional: true, Description: "The name of the argo template to call"},
					"webhook_secret": schema.StringAttribute{Optional: true, Sensitive: true, Description: "ArgoEvents webhook secret"},
					"http_insecure":  schema.BoolAttribute{Optional: true, Computed: true, Description: "Allow insecure HTTP connections", Default: booldefault.StaticBool(false)},
				},
			},
			"github": schema.SingleNestedBlock{
				Description: "GitHub job agent configuration",
				Attributes: map[string]schema.Attribute{
					"installation_id": schema.Int64Attribute{Optional: true, Description: "GitHub app installation ID"},
					"owner":           schema.StringAttribute{Optional: true, Description: "GitHub repository owner"},
					"ref":             schema.StringAttribute{Optional: true, Description: "Git ref to run the workflow on"},
					"repo":            schema.StringAttribute{Optional: true, Description: "GitHub repository name"},
					"workflow_id":     schema.Int64Attribute{Optional: true, Description: "GitHub Actions workflow ID"},
				},
			},
			"terraform_cloud": schema.SingleNestedBlock{
				Description: "Terraform Cloud job agent configuration",
				Attributes: map[string]schema.Attribute{
					"address":               schema.StringAttribute{Optional: true, Description: "Terraform Cloud address"},
					"organization":          schema.StringAttribute{Optional: true, Description: "Terraform Cloud organization name"},
					"template":              schema.StringAttribute{Optional: true, Description: "Terraform Cloud workspace template"},
					"token":                 schema.StringAttribute{Optional: true, Sensitive: true, Description: "Terraform Cloud API token"},
					"trigger_run_on_change": schema.BoolAttribute{Optional: true, Description: "Whether to create a TFC run on dispatch"},
				},
			},
			"test_runner": schema.SingleNestedBlock{
				Description: "Test runner job agent configuration",
				Attributes: map[string]schema.Attribute{
					"delay_seconds": schema.Int64Attribute{Optional: true, Description: "Delay in seconds before resolving the job"},
					"message":       schema.StringAttribute{Optional: true, Description: "Optional message to include in the job output"},
					"status":        schema.StringAttribute{Optional: true, Description: "Final status to set"},
				},
			},
		},
	}
}

func (r *DeploymentResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var data DeploymentResourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	count := 0
	if data.ArgoCD != nil {
		count++
	}
	if data.ArgoWorkflow != nil {
		count++
	}
	if data.GitHub != nil {
		count++
	}
	if data.TerraformCloud != nil {
		count++
	}
	if data.TestRunner != nil {
		count++
	}
	if count > 1 {
		resp.Diagnostics.AddError(
			"Invalid job agent configuration",
			"Only one of argocd, argo_workflow, github, terraform_cloud, or test_runner can be set.",
		)
	}
}

func (r *DeploymentResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data DeploymentResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resourceSelector := optionalStringPtr(data.ResourceSelector)

	jobAgentConfig, err := deploymentJobAgentConfigStruct(&data)
	if err != nil {
		resp.Diagnostics.AddError("Failed to create deployment", err.Error())
		return
	}

	var metadata map[string]string
	if p := stringMapPointer(data.Metadata); p != nil {
		metadata = *p
	}

	created, err := r.workspace.Deployment.CreateDeployment(ctx, connect.NewRequest(&apiv1.CreateDeploymentRequest{
		WorkspaceId:      r.workspace.WorkspaceID(),
		Name:             data.Name.ValueString(),
		ResourceSelector: resourceSelector,
		JobAgentSelector: data.JobAgentSelector.ValueStringPointer(),
		JobAgentConfig:   jobAgentConfig,
		Metadata:         metadata,
	}))
	if err != nil {
		addConnectError(&resp.Diagnostics, "Failed to create deployment", err)
		return
	}

	deploymentId := created.Msg.GetId()
	if deploymentId == "" {
		resp.Diagnostics.AddError("Failed to create deployment", "Empty deployment ID in response")
		return
	}

	data.ID = types.StringValue(deploymentId)

	got, err := r.workspace.Deployment.GetDeployment(ctx, connect.NewRequest(&apiv1.GetDeploymentRequest{
		WorkspaceId:  r.workspace.WorkspaceID(),
		DeploymentId: deploymentId,
	}))
	if err != nil {
		addConnectError(&resp.Diagnostics, "Failed to read deployment after create", err)
		return
	}

	dep := got.Msg.GetDeployment()
	if dep == nil || dep.GetId() == "" {
		resp.Diagnostics.AddError("Failed to read deployment after create", "Empty response from server")
		return
	}

	r.applyDeployment(&data, dep)

	resp.Diagnostics.Append(resp.State.Set(ctx, data)...)
}

// applyDeployment hydrates the Terraform model from the authoritative
// *apiv1.Deployment returned by GetDeployment.
func (r *DeploymentResource) applyDeployment(data *DeploymentResourceModel, dep *apiv1.Deployment) {
	data.ID = types.StringValue(dep.GetId())
	data.Name = types.StringValue(dep.GetName())
	data.Metadata = metadataMapValue(dep.GetMetadata())
	data.ResourceSelector = optionalString(dep.GetResourceSelector())
	data.JobAgentSelector = optionalSelector(dep.GetJobAgentSelector())

	var jobAgentConfig map[string]interface{}
	if cfg := dep.GetJobAgentConfig(); cfg != nil {
		jobAgentConfig = cfg.AsMap()
	}
	setDeploymentBlocksFromConfig(data, jobAgentConfig)
}

func (r *DeploymentResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data DeploymentResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	got, err := r.workspace.Deployment.GetDeployment(ctx, connect.NewRequest(&apiv1.GetDeploymentRequest{
		WorkspaceId:  r.workspace.WorkspaceID(),
		DeploymentId: data.ID.ValueString(),
	}))
	if err != nil {
		if isNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		addConnectError(&resp.Diagnostics, "Failed to read deployment", err)
		return
	}

	dep := got.Msg.GetDeployment()
	if dep == nil || dep.GetId() == "" {
		resp.Diagnostics.AddError("Failed to read deployment", "Empty response from server")
		return
	}

	r.applyDeployment(&data, dep)

	resp.Diagnostics.Append(resp.State.Set(ctx, data)...)
}

func (r *DeploymentResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data DeploymentResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resourceSelector := optionalStringPtr(data.ResourceSelector)

	jobAgentConfig, err := deploymentJobAgentConfigStruct(&data)
	if err != nil {
		resp.Diagnostics.AddError("Failed to update deployment", err.Error())
		return
	}

	var metadata map[string]string
	if p := stringMapPointer(data.Metadata); p != nil {
		metadata = *p
	}

	upserted, err := r.workspace.Deployment.UpsertDeployment(ctx, connect.NewRequest(&apiv1.UpsertDeploymentRequest{
		WorkspaceId:      r.workspace.WorkspaceID(),
		DeploymentId:     data.ID.ValueString(),
		Name:             data.Name.ValueString(),
		ResourceSelector: resourceSelector,
		JobAgentSelector: data.JobAgentSelector.ValueStringPointer(),
		JobAgentConfig:   jobAgentConfig,
		Metadata:         metadata,
	}))
	if err != nil {
		addConnectError(&resp.Diagnostics, "Failed to update deployment", err)
		return
	}

	deploymentId := data.ID.ValueString()
	if id := upserted.Msg.GetId(); id != "" {
		deploymentId = id
		data.ID = types.StringValue(id)
	}

	got, err := r.workspace.Deployment.GetDeployment(ctx, connect.NewRequest(&apiv1.GetDeploymentRequest{
		WorkspaceId:  r.workspace.WorkspaceID(),
		DeploymentId: deploymentId,
	}))
	if err != nil {
		addConnectError(&resp.Diagnostics, "Failed to read deployment after update", err)
		return
	}

	dep := got.Msg.GetDeployment()
	if dep == nil || dep.GetId() == "" {
		resp.Diagnostics.AddError("Failed to read deployment after update", "Empty response from server")
		return
	}

	r.applyDeployment(&data, dep)

	resp.Diagnostics.Append(resp.State.Set(ctx, data)...)
}

func (r *DeploymentResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data DeploymentResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	_, err := r.workspace.Deployment.DeleteDeployment(ctx, connect.NewRequest(&apiv1.DeleteDeploymentRequest{
		WorkspaceId:  r.workspace.WorkspaceID(),
		DeploymentId: data.ID.ValueString(),
	}))
	if err != nil && !isNotFound(err) {
		addConnectError(&resp.Diagnostics, "Failed to delete deployment", err)
		return
	}
}

type DeploymentResourceModel struct {
	ID               types.String `tfsdk:"id"`
	Name             types.String `tfsdk:"name"`
	Metadata         types.Map    `tfsdk:"metadata"`
	ResourceSelector types.String `tfsdk:"resource_selector"`
	JobAgentSelector types.String `tfsdk:"job_agent_selector"`

	ArgoCD         *DeploymentArgoCDModel       `tfsdk:"argocd"`
	ArgoWorkflow   *DeploymentArgoWorkflowModel `tfsdk:"argo_workflow"`
	GitHub         *DeploymentGitHubModel       `tfsdk:"github"`
	TerraformCloud *DeploymentTFCModel          `tfsdk:"terraform_cloud"`
	TestRunner     *DeploymentTestRunnerModel   `tfsdk:"test_runner"`
}

type DeploymentArgoCDModel struct {
	ApiKey    types.String `tfsdk:"api_key"`
	ServerUrl types.String `tfsdk:"server_url"`
	Template  types.String `tfsdk:"template"`
}

type DeploymentArgoWorkflowModel struct {
	ApiKey        types.String `tfsdk:"api_key"`
	WebhookSecret types.String `tfsdk:"webhook_secret"`
	ServerUrl     types.String `tfsdk:"server_url"`
	Template      types.String `tfsdk:"template"`
	Name          types.String `tfsdk:"name"`
	HttpInsecure  types.Bool   `tfsdk:"http_insecure"`
}

type DeploymentGitHubModel struct {
	InstallationId types.Int64  `tfsdk:"installation_id"`
	Owner          types.String `tfsdk:"owner"`
	Ref            types.String `tfsdk:"ref"`
	Repo           types.String `tfsdk:"repo"`
	WorkflowId     types.Int64  `tfsdk:"workflow_id"`
}

type DeploymentTFCModel struct {
	Address            types.String `tfsdk:"address"`
	Organization       types.String `tfsdk:"organization"`
	Template           types.String `tfsdk:"template"`
	Token              types.String `tfsdk:"token"`
	TriggerRunOnChange types.Bool   `tfsdk:"trigger_run_on_change"`
}

type DeploymentTestRunnerModel struct {
	DelaySeconds types.Int64  `tfsdk:"delay_seconds"`
	Message      types.String `tfsdk:"message"`
	Status       types.String `tfsdk:"status"`
}

// deploymentJobAgentConfigStruct extracts the typed block into a
// *structpb.Struct suitable for the proto JobAgentConfig field. A nil result
// (no block set or empty config) maps to an absent config.
func deploymentJobAgentConfigStruct(data *DeploymentResourceModel) (*structpb.Struct, error) {
	cfg := deploymentJobAgentConfigFromModel(data)
	if cfg == nil {
		return nil, nil
	}
	return structpb.NewStruct(*cfg)
}

// deploymentJobAgentConfigFromModel extracts the typed block into a
// map[string]interface{} suitable for the API's JobAgentConfig field.
func deploymentJobAgentConfigFromModel(data *DeploymentResourceModel) *map[string]interface{} {
	switch {
	case data.ArgoCD != nil:
		cfg := map[string]any{}
		setStringIfSet(cfg, "apiKey", data.ArgoCD.ApiKey)
		setStringIfSet(cfg, "serverUrl", data.ArgoCD.ServerUrl)
		setStringIfSet(cfg, "template", data.ArgoCD.Template)
		if len(cfg) == 0 {
			return nil
		}
		return &cfg
	case data.ArgoWorkflow != nil:
		cfg := map[string]any{}
		setStringIfSet(cfg, "apiKey", data.ArgoWorkflow.ApiKey)
		setStringIfSet(cfg, "webhookSecret", data.ArgoWorkflow.WebhookSecret)
		setStringIfSet(cfg, "serverUrl", data.ArgoWorkflow.ServerUrl)
		setStringIfSet(cfg, "template", data.ArgoWorkflow.Template)
		setStringIfSet(cfg, "name", data.ArgoWorkflow.Name)
		if !data.ArgoWorkflow.HttpInsecure.IsNull() && !data.ArgoWorkflow.HttpInsecure.IsUnknown() {
			cfg["httpInsecure"] = data.ArgoWorkflow.HttpInsecure.ValueBool()
		}
		if len(cfg) == 0 {
			return nil
		}
		return &cfg
	case data.GitHub != nil:
		cfg := map[string]any{}
		if !data.GitHub.InstallationId.IsNull() && !data.GitHub.InstallationId.IsUnknown() {
			cfg["installationId"] = data.GitHub.InstallationId.ValueInt64()
		}
		setStringIfSet(cfg, "owner", data.GitHub.Owner)
		setStringIfSet(cfg, "repo", data.GitHub.Repo)
		setStringIfSet(cfg, "ref", data.GitHub.Ref)
		if !data.GitHub.WorkflowId.IsNull() && !data.GitHub.WorkflowId.IsUnknown() {
			cfg["workflowId"] = data.GitHub.WorkflowId.ValueInt64()
		}
		if len(cfg) == 0 {
			return nil
		}
		return &cfg
	case data.TerraformCloud != nil:
		cfg := map[string]any{}
		setStringIfSet(cfg, "address", data.TerraformCloud.Address)
		setStringIfSet(cfg, "organization", data.TerraformCloud.Organization)
		setStringIfSet(cfg, "template", data.TerraformCloud.Template)
		setStringIfSet(cfg, "token", data.TerraformCloud.Token)
		if !data.TerraformCloud.TriggerRunOnChange.IsNull() && !data.TerraformCloud.TriggerRunOnChange.IsUnknown() {
			cfg["triggerRunOnChange"] = data.TerraformCloud.TriggerRunOnChange.ValueBool()
		}
		if len(cfg) == 0 {
			return nil
		}
		return &cfg
	case data.TestRunner != nil:
		cfg := map[string]any{}
		if !data.TestRunner.DelaySeconds.IsNull() && !data.TestRunner.DelaySeconds.IsUnknown() {
			cfg["delaySeconds"] = data.TestRunner.DelaySeconds.ValueInt64()
		}
		setStringIfSet(cfg, "message", data.TestRunner.Message)
		setStringIfSet(cfg, "status", data.TestRunner.Status)
		if len(cfg) == 0 {
			return nil
		}
		return &cfg
	default:
		return nil
	}
}

func setStringIfSet(cfg map[string]any, key string, val types.String) {
	if !val.IsNull() && !val.IsUnknown() && val.ValueString() != "" {
		cfg[key] = val.ValueString()
	}
}

// setDeploymentBlocksFromConfig populates the typed block on the model from
// the API's JobAgentConfig map. It uses the prior state block type to decide
// which block to populate so that reads are stable.
func setDeploymentBlocksFromConfig(data *DeploymentResourceModel, config map[string]interface{}) {
	blockType := deploymentBlockType(data)

	// Preserve sensitive fields from prior state before clearing blocks.
	priorArgoCD := data.ArgoCD
	priorArgoWorkflow := data.ArgoWorkflow
	priorTFC := data.TerraformCloud

	data.ArgoCD = nil
	data.ArgoWorkflow = nil
	data.GitHub = nil
	data.TerraformCloud = nil
	data.TestRunner = nil

	if len(config) == 0 {
		return
	}

	if blockType == "" {
		blockType = inferBlockTypeFromConfig(config)
	}
	if blockType == "" {
		return
	}

	switch blockType {
	case "argocd":
		data.ArgoCD = &DeploymentArgoCDModel{
			ApiKey:    stringValueOrNull(config["apiKey"]),
			ServerUrl: stringValueOrNull(config["serverUrl"]),
			Template:  stringValueOrNull(config["template"]),
		}
		if data.ArgoCD.ApiKey.IsNull() && priorArgoCD != nil && !priorArgoCD.ApiKey.IsNull() {
			data.ArgoCD.ApiKey = priorArgoCD.ApiKey
		}
	case "argo_workflow":
		data.ArgoWorkflow = &DeploymentArgoWorkflowModel{
			ApiKey:        stringValueOrNull(config["apiKey"]),
			WebhookSecret: stringValueOrNull(config["webhookSecret"]),
			ServerUrl:     stringValueOrNull(config["serverUrl"]),
			Template:      stringValueOrNull(config["template"]),
			Name:          stringValueOrNull(config["name"]),
			HttpInsecure:  boolValueOrNull(config["httpInsecure"]),
		}
		if data.ArgoWorkflow.ApiKey.IsNull() && priorArgoWorkflow != nil && !priorArgoWorkflow.ApiKey.IsNull() {
			data.ArgoWorkflow.ApiKey = priorArgoWorkflow.ApiKey
		}
		if data.ArgoWorkflow.WebhookSecret.IsNull() && priorArgoWorkflow != nil && !priorArgoWorkflow.WebhookSecret.IsNull() {
			data.ArgoWorkflow.WebhookSecret = priorArgoWorkflow.WebhookSecret
		}
	case "github":
		gh := DeploymentGitHubModel{
			InstallationId: types.Int64Null(),
			Owner:          types.StringNull(),
			Ref:            types.StringNull(),
			Repo:           types.StringNull(),
			WorkflowId:     types.Int64Null(),
		}
		if v, ok := config["installationId"]; ok && v != nil {
			gh.InstallationId = types.Int64Value(toInt64(v))
		}
		if v, ok := config["owner"]; ok && v != nil && fmt.Sprint(v) != "" {
			gh.Owner = types.StringValue(fmt.Sprint(v))
		}
		if v, ok := config["repo"]; ok && v != nil && fmt.Sprint(v) != "" {
			gh.Repo = types.StringValue(fmt.Sprint(v))
		}
		if v, ok := config["ref"]; ok && v != nil && fmt.Sprint(v) != "" {
			gh.Ref = types.StringValue(fmt.Sprint(v))
		}
		if v, ok := config["workflowId"]; ok && v != nil {
			gh.WorkflowId = types.Int64Value(toInt64(v))
		}
		data.GitHub = &gh
	case "terraform_cloud":
		data.TerraformCloud = &DeploymentTFCModel{
			Address:            stringValueOrNull(config["address"]),
			Organization:       stringValueOrNull(config["organization"]),
			Template:           stringValueOrNull(config["template"]),
			Token:              stringValueOrNull(config["token"]),
			TriggerRunOnChange: boolValueOrNull(config["triggerRunOnChange"]),
		}
		if data.TerraformCloud.Token.IsNull() && priorTFC != nil && !priorTFC.Token.IsNull() {
			data.TerraformCloud.Token = priorTFC.Token
		}
	case "test_runner":
		tr := DeploymentTestRunnerModel{
			DelaySeconds: types.Int64Null(),
			Message:      types.StringNull(),
			Status:       types.StringNull(),
		}
		if v, ok := config["delaySeconds"]; ok && v != nil {
			tr.DelaySeconds = types.Int64Value(toInt64(v))
		}
		if v, ok := config["message"]; ok && v != nil && fmt.Sprint(v) != "" {
			tr.Message = types.StringValue(fmt.Sprint(v))
		}
		if v, ok := config["status"]; ok && v != nil && fmt.Sprint(v) != "" {
			tr.Status = types.StringValue(fmt.Sprint(v))
		}
		data.TestRunner = &tr
	}
}

type argoCDConfig struct {
	ApiKey    string `json:"apiKey"`
	ServerUrl string `json:"serverUrl"`
	Template  string `json:"template"`
}

type argoWorkflowConfig struct {
	ApiKey        string `json:"apiKey"`
	WebhookSecret string `json:"webhookSecret"`
	ServerUrl     string `json:"serverUrl"`
	Template      string `json:"template"`
	Name          string `json:"name"`
	HttpInsecure  *bool  `json:"httpInsecure"`
}

type githubConfig struct {
	InstallationId *int64 `json:"installationId"`
	Owner          string `json:"owner"`
	Ref            string `json:"ref"`
	Repo           string `json:"repo"`
	WorkflowId     *int64 `json:"workflowId"`
}

type terraformCloudConfig struct {
	Address            string `json:"address"`
	Organization       string `json:"organization"`
	Template           string `json:"template"`
	Token              string `json:"token"`
	TriggerRunOnChange *bool  `json:"triggerRunOnChange"`
}

type testRunnerConfig struct {
	DelaySeconds *int64 `json:"delaySeconds"`
	Message      string `json:"message"`
	Status       string `json:"status"`
}

func inferBlockTypeFromConfig(config map[string]interface{}) string {
	data, err := json.Marshal(config)
	if err != nil {
		return ""
	}

	var gh githubConfig
	_ = json.Unmarshal(data, &gh)
	if gh.Owner != "" || gh.Repo != "" || gh.InstallationId != nil || gh.WorkflowId != nil {
		return "github"
	}

	var tfc terraformCloudConfig
	_ = json.Unmarshal(data, &tfc)
	if tfc.Organization != "" || tfc.Address != "" || tfc.TriggerRunOnChange != nil {
		return "terraform_cloud"
	}

	var tr testRunnerConfig
	_ = json.Unmarshal(data, &tr)
	if tr.DelaySeconds != nil || tr.Status != "" {
		return "test_runner"
	}

	var aw argoWorkflowConfig
	_ = json.Unmarshal(data, &aw)
	if aw.WebhookSecret != "" || aw.HttpInsecure != nil || aw.Name != "" {
		return "argo_workflow"
	}

	var ac argoCDConfig
	_ = json.Unmarshal(data, &ac)
	if ac.ServerUrl != "" || ac.Template != "" || ac.ApiKey != "" {
		return "argocd"
	}

	return ""
}

func deploymentBlockType(data *DeploymentResourceModel) string {
	switch {
	case data.ArgoCD != nil:
		return "argocd"
	case data.ArgoWorkflow != nil:
		return "argo_workflow"
	case data.GitHub != nil:
		return "github"
	case data.TerraformCloud != nil:
		return "terraform_cloud"
	case data.TestRunner != nil:
		return "test_runner"
	default:
		return ""
	}
}

func stringValueOrNull(value interface{}) types.String {
	if value == nil {
		return types.StringNull()
	}
	return types.StringValue(fmt.Sprint(value))
}

func boolValueOrNull(value interface{}) types.Bool {
	if value == nil {
		return types.BoolNull()
	}
	if b, ok := value.(bool); ok {
		return types.BoolValue(b)
	}
	return types.BoolNull()
}

func stringInterfaceMapPointer(value types.Map) *map[string]interface{} {
	if value.IsNull() || value.IsUnknown() {
		return nil
	}

	var decoded map[string]string
	diags := value.ElementsAs(context.Background(), &decoded, false)
	if diags.HasError() {
		return nil
	}

	result := make(map[string]interface{}, len(decoded))
	for k, v := range decoded {
		result[k] = v
	}

	return &result
}

func interfaceMapStringValue(value map[string]interface{}) types.Map {
	if value == nil {
		return types.MapNull(types.StringType)
	}

	result := make(map[string]string, len(value))
	for k, v := range value {
		result[k] = fmt.Sprint(v)
	}

	mapped, _ := types.MapValueFrom(context.Background(), types.StringType, result)
	return mapped
}
