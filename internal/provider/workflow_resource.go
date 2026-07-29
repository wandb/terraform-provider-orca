// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"

	apiv1 "buf.build/gen/go/ctrlplane/ctrlplane/protocolbuffers/go/ctrlplane/api/v1"
	connect "connectrpc.com/connect"
	"github.com/ctrlplanedev/terraform-provider-ctrlplane/internal/api"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	structpb "google.golang.org/protobuf/types/known/structpb"
)

var _ resource.Resource = &WorkflowResource{}
var _ resource.ResourceWithImportState = &WorkflowResource{}
var _ resource.ResourceWithConfigure = &WorkflowResource{}

func NewWorkflowResource() resource.Resource {
	return &WorkflowResource{}
}

type WorkflowResource struct {
	workspace *api.WorkspaceClient
}

type WorkflowResourceModel struct {
	ID        types.String            `tfsdk:"id"`
	Name      types.String            `tfsdk:"name"`
	Slug      types.String            `tfsdk:"slug"`
	Inputs    types.String            `tfsdk:"inputs"`
	JobAgents []WorkflowJobAgentModel `tfsdk:"job_agent"`
}

type WorkflowJobAgentModel struct {
	Name       types.String `tfsdk:"name"`
	Ref        types.String `tfsdk:"ref"`
	Config     types.Map    `tfsdk:"config"`
	ConfigJSON types.String `tfsdk:"config_json"`
	Selector   types.String `tfsdk:"selector"`
}

func (r *WorkflowResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_workflow"
}

func (r *WorkflowResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func (r *WorkflowResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *WorkflowResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a workflow in Ctrlplane.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:    true,
				Description: "The ID of the workflow.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				Required:    true,
				Description: "The name of the workflow.",
			},
			"slug": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "URL-safe identifier unique within the workspace. Derived from name if omitted; sticky once set.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"inputs": schema.StringAttribute{
				Optional:    true,
				Description: "JSON-encoded array of workflow input definitions.",
			},
		},
		Blocks: map[string]schema.Block{
			"job_agent": schema.ListNestedBlock{
				Description: "Job agents to dispatch when the workflow runs.",
				NestedObject: schema.NestedBlockObject{
					Attributes: map[string]schema.Attribute{
						"name": schema.StringAttribute{
							Required:    true,
							Description: "Name of the job agent entry.",
						},
						"ref": schema.StringAttribute{
							Required:    true,
							Description: "ID of the job agent to reference.",
						},
						"config": schema.MapAttribute{
							Optional:    true,
							Description: "String configuration for the job agent. Conflicts with config_json.",
							ElementType: types.StringType,
						},
						"config_json": schema.StringAttribute{
							Optional:    true,
							Description: "JSON-encoded configuration for the job agent. Use this for numbers, booleans, lists, or nested objects. Conflicts with config.",
						},
						"selector": schema.StringAttribute{
							Required:    true,
							Description: "CEL expression to determine if the job agent should dispatch. Use \"true\" to always dispatch.",
						},
					},
				},
			},
		},
	}
}

func (r *WorkflowResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data WorkflowResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	inputs, err := workflowInputsToValue(data.Inputs)
	if err != nil {
		resp.Diagnostics.AddError("Invalid inputs", err.Error())
		return
	}

	jobAgents, err := workflowJobAgentsToValue(data.JobAgents)
	if err != nil {
		resp.Diagnostics.AddError("Invalid job agents", err.Error())
		return
	}

	created, err := r.workspace.Workflow.CreateWorkflow(ctx, connect.NewRequest(&apiv1.CreateWorkflowRequest{
		WorkspaceId: r.workspace.WorkspaceID(),
		Name:        data.Name.ValueString(),
		Slug:        optionalStringPtr(data.Slug),
		Inputs:      inputs,
		JobAgents:   jobAgents,
	}))
	if err != nil {
		addConnectError(&resp.Diagnostics, "Failed to create workflow", err)
		return
	}

	setWorkflowModelFromProto(&data, created.Msg)
	resp.Diagnostics.Append(resp.State.Set(ctx, data)...)
}

func (r *WorkflowResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data WorkflowResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	got, err := r.workspace.Workflow.GetWorkflow(ctx, connect.NewRequest(&apiv1.GetWorkflowRequest{
		WorkspaceId: r.workspace.WorkspaceID(),
		WorkflowId:  data.ID.ValueString(),
	}))
	if err != nil {
		if isNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		addConnectError(&resp.Diagnostics, "Failed to read workflow", err)
		return
	}

	setWorkflowModelFromProto(&data, got.Msg)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *WorkflowResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data WorkflowResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	inputs, err := workflowInputsToValue(data.Inputs)
	if err != nil {
		resp.Diagnostics.AddError("Invalid inputs", err.Error())
		return
	}

	jobAgents, err := workflowJobAgentsToValue(data.JobAgents)
	if err != nil {
		resp.Diagnostics.AddError("Invalid job agents", err.Error())
		return
	}

	updated, err := r.workspace.Workflow.UpdateWorkflow(ctx, connect.NewRequest(&apiv1.UpdateWorkflowRequest{
		WorkspaceId: r.workspace.WorkspaceID(),
		WorkflowId:  data.ID.ValueString(),
		Name:        data.Name.ValueString(),
		Slug:        optionalStringPtr(data.Slug),
		Inputs:      inputs,
		JobAgents:   jobAgents,
	}))
	if err != nil {
		addConnectError(&resp.Diagnostics, "Failed to update workflow", err)
		return
	}

	setWorkflowModelFromProto(&data, updated.Msg)
	resp.Diagnostics.Append(resp.State.Set(ctx, data)...)
}

func (r *WorkflowResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data WorkflowResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	_, err := r.workspace.Workflow.DeleteWorkflow(ctx, connect.NewRequest(&apiv1.DeleteWorkflowRequest{
		WorkspaceId: r.workspace.WorkspaceID(),
		WorkflowId:  data.ID.ValueString(),
	}))
	if err != nil && !isNotFound(err) {
		addConnectError(&resp.Diagnostics, "Failed to delete workflow", err)
		return
	}
}

// --- helpers ---

// workflowInputsToValue converts the model's JSON-encoded inputs string into a
// *structpb.Value for transport. An empty/null/unknown value or "[]" yields an
// empty-list Value so the server receives a deterministic, non-nil inputs field.
func workflowInputsToValue(raw types.String) (*structpb.Value, error) {
	str := ""
	if !raw.IsNull() && !raw.IsUnknown() {
		str = raw.ValueString()
	}
	if str == "" || str == "[]" {
		return structpb.NewValue([]any{})
	}
	var v any
	if err := json.Unmarshal([]byte(str), &v); err != nil {
		return nil, fmt.Errorf("failed to parse inputs JSON: %w", err)
	}
	return structpb.NewValue(v)
}

// workflowInputsFromValue renders the proto inputs Value back into the model's
// normalized JSON string. nil or a non-list Value yields "[]", preserving the
// prior representation where Inputs is always a JSON array string.
func workflowInputsFromValue(val *structpb.Value) types.String {
	if val == nil {
		return types.StringValue("[]")
	}
	return types.StringValue(normalizeInputsValue(val.AsInterface()))
}

// normalizeInputsValue re-marshals the decoded inputs through a []map[string]any
// so JSON key order is deterministic (Go sorts map keys alphabetically). This
// prevents Terraform from detecting spurious diffs due to key ordering and
// mirrors the previous normalizeInputsJSON behavior.
func normalizeInputsValue(v any) string {
	raw, err := json.Marshal(v)
	if err != nil {
		return "[]"
	}
	var normalized []map[string]interface{}
	if err := json.Unmarshal(raw, &normalized); err != nil {
		return "[]"
	}
	out, err := json.Marshal(normalized)
	if err != nil {
		return "[]"
	}
	return string(out)
}

// workflowJobAgentsToValue converts the typed job-agent blocks into a
// *structpb.Value wrapping a list of objects for transport.
func workflowJobAgentsToValue(agents []WorkflowJobAgentModel) (*structpb.Value, error) {
	list := make([]any, len(agents))
	for i, a := range agents {
		config, err := workflowJobAgentConfigToMap(a)
		if err != nil {
			return nil, fmt.Errorf("job agent %q: %w", a.Name.ValueString(), err)
		}
		list[i] = map[string]any{
			"name":     a.Name.ValueString(),
			"ref":      a.Ref.ValueString(),
			"config":   config,
			"selector": a.Selector.ValueString(),
		}
	}
	return structpb.NewValue(list)
}

func workflowJobAgentConfigToMap(agent WorkflowJobAgentModel) (map[string]any, error) {
	configKnown := !agent.Config.IsNull() && !agent.Config.IsUnknown()
	configJSONKnown := !agent.ConfigJSON.IsNull() && !agent.ConfigJSON.IsUnknown()

	if agent.Config.IsUnknown() || agent.ConfigJSON.IsUnknown() {
		return nil, fmt.Errorf("config and config_json must be known before applying")
	}
	if configKnown == configJSONKnown {
		return nil, fmt.Errorf("exactly one of config or config_json must be set")
	}

	if configKnown {
		var decoded map[string]string
		diags := agent.Config.ElementsAs(context.Background(), &decoded, false)
		if diags.HasError() {
			return nil, fmt.Errorf("failed to decode config string map: %s", diags.Errors()[0].Detail())
		}

		config := make(map[string]any, len(decoded))
		for key, value := range decoded {
			config[key] = value
		}
		return config, nil
	}

	var decoded any
	if err := json.Unmarshal([]byte(agent.ConfigJSON.ValueString()), &decoded); err != nil {
		return nil, fmt.Errorf("config_json must contain a valid JSON object: %w", err)
	}
	config, ok := decoded.(map[string]any)
	if !ok || config == nil {
		return nil, fmt.Errorf("config_json must contain a valid JSON object")
	}
	return config, nil
}

// workflowJobAgentsFromValue rebuilds the typed job-agent blocks from the proto
// JobAgents Value. The prior state selects config or config_json to avoid
// changing representations during refresh. A nil or non-list Value yields an
// empty slice.
func workflowJobAgentsFromValue(val *structpb.Value, prior []WorkflowJobAgentModel) []WorkflowJobAgentModel {
	if val == nil {
		return []WorkflowJobAgentModel{}
	}
	raw, ok := val.AsInterface().([]any)
	if !ok {
		return []WorkflowJobAgentModel{}
	}
	agents := make([]WorkflowJobAgentModel, len(raw))
	for i, item := range raw {
		obj, _ := item.(map[string]any)
		config := map[string]any{}
		if c, ok := obj["config"].(map[string]any); ok {
			config = c
		}

		name := stringFromAny(obj["name"])
		ref := stringFromAny(obj["ref"])
		agent := WorkflowJobAgentModel{
			Name:     types.StringValue(name),
			Ref:      types.StringValue(ref),
			Selector: types.StringValue(stringFromAny(obj["selector"])),
		}
		priorConfigJSON := workflowJobAgentPriorConfigJSON(prior, i, name, ref)
		if !priorConfigJSON.IsNull() || !workflowConfigContainsOnlyStrings(config) {
			agent.Config = types.MapNull(types.StringType)
			agent.ConfigJSON = workflowConfigJSONValue(config, priorConfigJSON)
		} else {
			agent.Config = interfaceMapStringValue(config)
			agent.ConfigJSON = types.StringNull()
		}
		agents[i] = agent
	}
	return agents
}

func workflowJobAgentPriorConfigJSON(prior []WorkflowJobAgentModel, index int, name, ref string) types.String {
	if index < len(prior) {
		candidate := prior[index]
		if candidate.Name.ValueString() == name && candidate.Ref.ValueString() == ref {
			if !candidate.ConfigJSON.IsNull() && !candidate.ConfigJSON.IsUnknown() {
				return candidate.ConfigJSON
			}
			return types.StringNull()
		}
	}
	for _, candidate := range prior {
		if candidate.Name.ValueString() == name && candidate.Ref.ValueString() == ref {
			if !candidate.ConfigJSON.IsNull() && !candidate.ConfigJSON.IsUnknown() {
				return candidate.ConfigJSON
			}
			return types.StringNull()
		}
	}
	return types.StringNull()
}

func workflowConfigContainsOnlyStrings(config map[string]any) bool {
	for _, value := range config {
		if _, ok := value.(string); !ok {
			return false
		}
	}
	return true
}

func workflowConfigJSONValue(config map[string]any, prior types.String) types.String {
	if !prior.IsNull() && !prior.IsUnknown() {
		var priorConfig map[string]any
		if err := json.Unmarshal([]byte(prior.ValueString()), &priorConfig); err == nil &&
			reflect.DeepEqual(priorConfig, config) {
			return prior
		}
	}

	encoded, err := json.Marshal(config)
	if err != nil {
		return types.StringValue("{}")
	}
	return types.StringValue(string(encoded))
}

func stringFromAny(v any) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return fmt.Sprint(v)
}

func setWorkflowModelFromProto(data *WorkflowResourceModel, w *apiv1.Workflow) {
	priorJobAgents := data.JobAgents
	data.ID = types.StringValue(w.GetId())
	data.Name = types.StringValue(w.GetName())
	data.Slug = types.StringValue(w.GetSlug())
	data.Inputs = workflowInputsFromValue(w.GetInputs())
	data.JobAgents = workflowJobAgentsFromValue(w.GetJobAgents(), priorJobAgents)
}
