// Copyright IBM Corp. 2021, 2026

package provider

import (
	"context"
	"fmt"
	"time"

	apiv1 "buf.build/gen/go/ctrlplane/ctrlplane/protocolbuffers/go/ctrlplane/api/v1"
	connect "connectrpc.com/connect"
	"github.com/ctrlplanedev/terraform-provider-ctrlplane/internal/api"
	"github.com/google/uuid"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/defaults"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/mapdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

var _ resource.Resource = &PolicyResource{}
var _ resource.ResourceWithImportState = &PolicyResource{}
var _ resource.ResourceWithConfigure = &PolicyResource{}
var _ resource.ResourceWithValidateConfig = &PolicyResource{}

func NewPolicyResource() resource.Resource {
	return &PolicyResource{}
}

type PolicyResource struct {
	workspace *api.WorkspaceClient
}

func (r *PolicyResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_policy"
}

func (r *PolicyResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func (r *PolicyResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *PolicyResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:    true,
				Description: "The ID of the policy",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				Required:    true,
				Description: "The name of the policy",
			},
			"description": schema.StringAttribute{
				Optional:    true,
				Description: "The description of the policy",
			},
			"metadata": schema.MapAttribute{
				Optional:    true,
				Computed:    true,
				Description: "The metadata of the policy",
				ElementType: types.StringType,
				Default: func() defaults.Map {
					empty, _ := types.MapValueFrom(context.Background(), types.StringType, map[string]string{})
					return mapdefault.StaticValue(empty)
				}(),
			},
			"priority": schema.Int64Attribute{
				Optional:    true,
				Computed:    true,
				Description: "The priority of the policy (higher is evaluated first)",
				Default:     int64default.StaticInt64(0),
			},
			"enabled": schema.BoolAttribute{
				Optional:    true,
				Computed:    true,
				Description: "Whether the policy is enabled",
				Default:     booldefault.StaticBool(true),
			},
			"selector": schema.StringAttribute{
				Required:    true,
				Description: "CEL expression for matching release targets. Use \"true\" to match all targets.",
			},
		},
		Blocks: map[string]schema.Block{
			"version_selector": schema.ListNestedBlock{
				Description: "Version selector rules to filter which deployment versions are allowed",
				NestedObject: schema.NestedBlockObject{
					Attributes: map[string]schema.Attribute{
						"created_at": schema.StringAttribute{
							Computed:    true,
							Description: "Rule creation timestamp",
							PlanModifiers: []planmodifier.String{
								stringplanmodifier.UseStateForUnknown(),
							},
						},
						"id": schema.StringAttribute{
							Optional:    true,
							Computed:    true,
							Description: "Rule ID",
							PlanModifiers: []planmodifier.String{
								stringplanmodifier.UseStateForUnknown(),
							},
						},
						"selector": schema.StringAttribute{
							Required:    true,
							Description: "CEL expression to match allowed versions (has access to version, environment, resource, and deployment variables)",
							PlanModifiers: []planmodifier.String{
								celNormalized(),
							},
						},
						"description": schema.StringAttribute{
							Optional:    true,
							Description: "Human-readable explanation of the rule, shown when a version is blocked",
						},
					},
				},
			},
			"version_cooldown": schema.ListNestedBlock{
				Description: "Version cooldown rules",
				NestedObject: schema.NestedBlockObject{
					Attributes: map[string]schema.Attribute{
						"created_at": schema.StringAttribute{
							Computed:    true,
							Description: "Rule creation timestamp",
							PlanModifiers: []planmodifier.String{
								stringplanmodifier.UseStateForUnknown(),
							},
						},
						"id": schema.StringAttribute{
							Optional:    true,
							Computed:    true,
							Description: "Rule ID",
							PlanModifiers: []planmodifier.String{
								stringplanmodifier.UseStateForUnknown(),
							},
						},
						"duration": schema.StringAttribute{
							Required:    true,
							Description: "Minimum duration between deployments (e.g., \"1h\")",
						},
					},
				},
			},
			"deployment_window": schema.ListNestedBlock{
				Description: "Deployment window rules",
				NestedObject: schema.NestedBlockObject{
					Attributes: map[string]schema.Attribute{
						"created_at": schema.StringAttribute{
							Computed:    true,
							Description: "Rule creation timestamp",
							PlanModifiers: []planmodifier.String{
								stringplanmodifier.UseStateForUnknown(),
							},
						},
						"id": schema.StringAttribute{
							Computed:    true,
							Description: "Rule ID",
							PlanModifiers: []planmodifier.String{
								stringplanmodifier.UseStateForUnknown(),
							},
						},
						"duration_minutes": schema.Int64Attribute{
							Required:    true,
							Description: "Duration of each window in minutes",
						},
						"rrule": schema.StringAttribute{
							Required:    true,
							Description: "RFC 5545 recurrence rule for window starts",
						},
						"timezone": schema.StringAttribute{
							Optional:    true,
							Description: "IANA timezone for the recurrence rule",
						},
						"allow_window": schema.BoolAttribute{
							Optional:    true,
							Computed:    true,
							Description: "Allow deployments during the window (deny when false)",
							Default:     booldefault.StaticBool(true),
						},
					},
				},
			},
			"deployment_dependency": schema.ListNestedBlock{
				Description: "Deployment dependency rules",
				NestedObject: schema.NestedBlockObject{
					Attributes: map[string]schema.Attribute{
						"created_at": schema.StringAttribute{
							Computed:    true,
							Description: "Rule creation timestamp",
							PlanModifiers: []planmodifier.String{
								stringplanmodifier.UseStateForUnknown(),
							},
						},
						"id": schema.StringAttribute{
							Optional:    true,
							Computed:    true,
							Description: "Rule ID",
							PlanModifiers: []planmodifier.String{
								stringplanmodifier.UseStateForUnknown(),
							},
						},
						"depends_on_selector": schema.StringAttribute{
							Required:    true,
							Description: "CEL expression to match upstream deployment(s) that must have a successful release before this deployment can proceed",
						},
					},
				},
			},
			"verification": schema.ListNestedBlock{
				Description: "Verification rules",
				NestedObject: schema.NestedBlockObject{
					Attributes: map[string]schema.Attribute{
						"created_at": schema.StringAttribute{
							Computed:    true,
							Description: "Rule creation timestamp",
							PlanModifiers: []planmodifier.String{
								stringplanmodifier.UseStateForUnknown(),
							},
						},
						"id": schema.StringAttribute{
							Optional:    true,
							Computed:    true,
							Description: "Rule ID",
							PlanModifiers: []planmodifier.String{
								stringplanmodifier.UseStateForUnknown(),
							},
						},
						"trigger_on": schema.StringAttribute{
							Optional:    true,
							Description: "When to trigger verification (e.g., \"jobSuccess\")",
						},
					},
					Blocks: map[string]schema.Block{
						"metric": schema.ListNestedBlock{
							Description: "Verification metrics",
							NestedObject: schema.NestedBlockObject{
								Attributes: map[string]schema.Attribute{
									"name": schema.StringAttribute{
										Required:    true,
										Description: "Metric name",
									},
									"interval": schema.StringAttribute{
										Required:    true,
										Description: "Interval between measurements (e.g., \"30s\")",
									},
									"count": schema.Int64Attribute{
										Required:    true,
										Description: "Number of measurements to take",
									},
								},
								Blocks: map[string]schema.Block{
									"success": schema.SingleNestedBlock{
										Description: "Success condition",
										Attributes: map[string]schema.Attribute{
											"condition": schema.StringAttribute{
												Required:    true,
												Description: "CEL expression to evaluate success",
											},
											"threshold": schema.Int64Attribute{
												Optional:    true,
												Description: "Minimum consecutive successes required",
											},
										},
									},
									"failure": schema.SingleNestedBlock{
										Description: "Failure condition",
										Attributes: map[string]schema.Attribute{
											"condition": schema.StringAttribute{
												Optional:    true,
												Description: "CEL expression to evaluate failure",
											},
											"threshold": schema.Int64Attribute{
												Optional:    true,
												Description: "Consecutive failures before failing",
											},
										},
									},
									"sleep": schema.SingleNestedBlock{
										Description: "Sleep metric provider configuration",
										Attributes: map[string]schema.Attribute{
											"duration_seconds": schema.Int64Attribute{
												Optional:    true,
												Computed:    true,
												Description: "Duration to sleep in seconds (1-3600, default 30)",
												Default:     int64default.StaticInt64(30),
											},
										},
									},
									"datadog": schema.SingleNestedBlock{
										Description: "Datadog metric provider configuration",
										Attributes: map[string]schema.Attribute{
											"site": schema.StringAttribute{
												Optional:    true,
												Description: "Datadog site URL (e.g., us5.datadoghq.com)",
											},
											"interval": schema.StringAttribute{
												Optional:    true,
												Description: "Provider interval (e.g., \"1m\")",
											},
											"queries": schema.MapAttribute{
												Optional:    true,
												Description: "Datadog metric queries",
												ElementType: types.StringType,
											},
											"api_key": schema.StringAttribute{
												Optional:    true,
												Description: "Datadog API key",
												Sensitive:   true,
											},
											"app_key": schema.StringAttribute{
												Optional:    true,
												Description: "Datadog application key",
												Sensitive:   true,
											},
											"aggregator": schema.StringAttribute{
												Optional:    true,
												Description: "Datadog aggregator (e.g., \"avg\")",
											},
											"formula": schema.StringAttribute{
												Optional:    true,
												Description: "Datadog formula",
											},
										},
									},
								},
							},
						},
					},
				},
			},
			"gradual_rollout": schema.ListNestedBlock{
				Description: "Gradual rollout rules",
				NestedObject: schema.NestedBlockObject{
					Attributes: map[string]schema.Attribute{
						"created_at": schema.StringAttribute{
							Computed:    true,
							Description: "Rule creation timestamp",
							PlanModifiers: []planmodifier.String{
								stringplanmodifier.UseStateForUnknown(),
							},
						},
						"id": schema.StringAttribute{
							Computed:    true,
							Description: "Rule ID",
							PlanModifiers: []planmodifier.String{
								stringplanmodifier.UseStateForUnknown(),
							},
						},
						"rollout_type": schema.StringAttribute{
							Required:    true,
							Description: "Rollout strategy: \"linear\" or \"linear-normalized\"",
						},
						"time_scale_interval": schema.Int64Attribute{
							Required:    true,
							Description: "Base time interval in seconds used to compute delay between deployments",
						},
					},
				},
			},
			"any_approval": schema.ListNestedBlock{
				Description: "Any approval rules",
				NestedObject: schema.NestedBlockObject{
					Attributes: map[string]schema.Attribute{
						"created_at": schema.StringAttribute{
							Computed:    true,
							Description: "Rule creation timestamp",
							PlanModifiers: []planmodifier.String{
								stringplanmodifier.UseStateForUnknown(),
							},
						},
						"id": schema.StringAttribute{
							Computed:    true,
							Description: "Rule ID",
							PlanModifiers: []planmodifier.String{
								stringplanmodifier.UseStateForUnknown(),
							},
						},
						"min_approvals": schema.Int64Attribute{
							Required:    true,
							Description: "Minimum number of approvals required",
						},
					},
				},
			},
			"environment_progression": schema.ListNestedBlock{
				Description: "Environment progression rules",
				NestedObject: schema.NestedBlockObject{
					Attributes: map[string]schema.Attribute{
						"created_at": schema.StringAttribute{
							Computed:    true,
							Description: "Rule creation timestamp",
							PlanModifiers: []planmodifier.String{
								stringplanmodifier.UseStateForUnknown(),
							},
						},
						"id": schema.StringAttribute{
							Computed:    true,
							Description: "Rule ID",
							PlanModifiers: []planmodifier.String{
								stringplanmodifier.UseStateForUnknown(),
							},
						},
						"depends_on_environment_selector": schema.StringAttribute{
							Required:    true,
							Description: "CEL expression to match the environment that must have a successful release before this environment can proceed",
							PlanModifiers: []planmodifier.String{
								celNormalized(),
							},
						},
						"minimum_success_percentage": schema.Float64Attribute{
							Optional:    true,
							Description: "Minimum percentage of successful deployments required",
						},
						"minimum_soak_time_minutes": schema.Int64Attribute{
							Optional:    true,
							Computed:    true,
							Description: "Minimum time in minutes to wait after the dependency environment is in a success state",
							Default:     int64default.StaticInt64(0),
						},
						"maximum_age_hours": schema.Int64Attribute{
							Optional:    true,
							Description: "Maximum age in hours of dependency deployment before blocking progression",
						},
					},
				},
			},
			"plan_validation_opa": schema.ListNestedBlock{
				Description: "OPA-based plan validation rules. Each rule must define a `deny` rule set following the Conftest convention.",
				NestedObject: schema.NestedBlockObject{
					Attributes: map[string]schema.Attribute{
						"created_at": schema.StringAttribute{
							Computed:    true,
							Description: "Rule creation timestamp",
							PlanModifiers: []planmodifier.String{
								stringplanmodifier.UseStateForUnknown(),
							},
						},
						"id": schema.StringAttribute{
							Computed:    true,
							Description: "Rule ID",
							PlanModifiers: []planmodifier.String{
								stringplanmodifier.UseStateForUnknown(),
							},
						},
						"name": schema.StringAttribute{
							Required:    true,
							Description: "Human-readable rule name; used in check output to identify which rule produced a violation.",
						},
						"description": schema.StringAttribute{
							Optional:    true,
							Description: "Optional human-readable explanation of the rule.",
						},
						"rego": schema.StringAttribute{
							Required:    true,
							Description: "Rego source code. Follows Conftest conventions for emitting violations.",
						},
					},
				},
			},
		},
	}
}

func (r *PolicyResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var data PolicyResourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if data.Selector.IsUnknown() {
		return
	}

	if data.Selector.IsNull() || data.Selector.ValueString() == "" {
		resp.Diagnostics.AddError("Invalid policy configuration", "The selector attribute must be set to a CEL expression.")
		return
	}
}

func (r *PolicyResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data PolicyResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Assign stable rule IDs and creation timestamps before building the proto
	// rules so the values persist into state and match what is sent.
	ensurePolicyIDs(&data, nil)
	ensurePolicyRuleCreatedAt(&data, nil)

	rules, diags := policyRulesFromModel(data)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	priority := int32(defaultInt64(data.Priority, 0))
	enabled := defaultBool(data.Enabled, true)
	selector := data.Selector.ValueString()

	var metadata map[string]string
	if p := stringMapPointer(data.Metadata); p != nil {
		metadata = *p
	}

	created, err := r.workspace.Policy.CreatePolicy(ctx, connect.NewRequest(&apiv1.CreatePolicyRequest{
		WorkspaceId: r.workspace.WorkspaceID(),
		Name:        data.Name.ValueString(),
		Description: data.Description.ValueStringPointer(),
		Priority:    &priority,
		Enabled:     &enabled,
		Selector:    &selector,
		Metadata:    metadata,
		Rules:       rules,
	}))
	if err != nil {
		addConnectError(&resp.Diagnostics, "Failed to create policy", err)
		return
	}

	policy := created.Msg
	if policy.GetId() == "" {
		resp.Diagnostics.AddError("Failed to create policy", "Empty policy ID in response")
		return
	}

	diags = applyPolicyToModel(&data, policy)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, data)...)
}

func (r *PolicyResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data PolicyResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	got, err := r.workspace.Policy.GetPolicy(ctx, connect.NewRequest(&apiv1.GetPolicyRequest{
		WorkspaceId: r.workspace.WorkspaceID(),
		PolicyId:    data.ID.ValueString(),
	}))
	if err != nil {
		if isNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		addConnectError(&resp.Diagnostics, "Failed to read policy", err)
		return
	}

	policy := got.Msg
	if policy.GetId() == "" {
		resp.Diagnostics.AddError("Failed to read policy", "Empty policy ID in response")
		return
	}

	diags := applyPolicyToModel(&data, policy)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *PolicyResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data PolicyResourceModel
	var state PolicyResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	data.ID = state.ID
	ensurePolicyIDs(&data, &state)
	ensurePolicyRuleCreatedAt(&data, &state)

	rules, diags := policyRulesFromModel(data)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	priority := int32(defaultInt64(data.Priority, 0))
	enabled := defaultBool(data.Enabled, true)
	selector := data.Selector.ValueString()

	var metadata map[string]string
	if p := stringMapPointer(data.Metadata); p != nil {
		metadata = *p
	}

	upserted, err := r.workspace.Policy.UpsertPolicy(ctx, connect.NewRequest(&apiv1.UpsertPolicyRequest{
		WorkspaceId: r.workspace.WorkspaceID(),
		PolicyId:    data.ID.ValueString(),
		Name:        data.Name.ValueString(),
		Description: data.Description.ValueStringPointer(),
		Priority:    priority,
		Enabled:     enabled,
		Selector:    selector,
		Metadata:    metadata,
		Rules:       rules,
	}))
	if err != nil {
		addConnectError(&resp.Diagnostics, "Failed to update policy", err)
		return
	}

	policy := upserted.Msg
	if policy.GetId() == "" {
		resp.Diagnostics.AddError("Failed to update policy", "Empty policy ID in response")
		return
	}

	diags = applyPolicyToModel(&data, policy)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, data)...)
}

func (r *PolicyResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data PolicyResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	_, err := r.workspace.Policy.DeletePolicy(ctx, connect.NewRequest(&apiv1.DeletePolicyRequest{
		WorkspaceId: r.workspace.WorkspaceID(),
		PolicyId:    data.ID.ValueString(),
	}))
	if err != nil && !isNotFound(err) {
		addConnectError(&resp.Diagnostics, "Failed to delete policy", err)
		return
	}
}

// applyPolicyToModel maps a proto Policy returned by the API into the Terraform
// model, including the rule blocks. The schema/attribute names and state shape
// are preserved exactly; only the transport changed.
func applyPolicyToModel(data *PolicyResourceModel, policy *apiv1.Policy) diag.Diagnostics {
	data.ID = types.StringValue(policy.GetId())
	data.Name = types.StringValue(policy.GetName())
	data.Description = optionalString(policy.GetDescription())
	data.Metadata = metadataMapValue(policy.GetMetadata())
	data.Priority = types.Int64Value(int64(policy.GetPriority()))
	data.Enabled = types.BoolValue(policy.GetEnabled())
	data.Selector = types.StringValue(policy.GetSelector())

	rules, diags := policyRulesToModel(policy.GetRules())
	if diags.HasError() {
		return diags
	}
	data.VersionSelector = rules.VersionSelector
	data.VersionCooldown = rules.VersionCooldown
	data.DeploymentWindow = rules.DeploymentWindow
	data.DeploymentDependency = rules.DeploymentDependency
	data.Verification = rules.Verification
	data.GradualRollout = rules.GradualRollout
	data.AnyApproval = rules.AnyApproval
	data.EnvironmentProgression = rules.EnvironmentProgression
	data.PlanValidationOpa = rules.PlanValidationOpa
	return diags
}

type PolicyResourceModel struct {
	ID                     types.String                   `tfsdk:"id"`
	Name                   types.String                   `tfsdk:"name"`
	Description            types.String                   `tfsdk:"description"`
	Metadata               types.Map                      `tfsdk:"metadata"`
	Priority               types.Int64                    `tfsdk:"priority"`
	Enabled                types.Bool                     `tfsdk:"enabled"`
	Selector               types.String                   `tfsdk:"selector"`
	VersionSelector        []PolicyVersionSelector        `tfsdk:"version_selector"`
	VersionCooldown        []PolicyVersionCooldown        `tfsdk:"version_cooldown"`
	DeploymentWindow       []PolicyDeploymentWindow       `tfsdk:"deployment_window"`
	DeploymentDependency   []PolicyDeploymentDependency   `tfsdk:"deployment_dependency"`
	Verification           []PolicyVerificationRule       `tfsdk:"verification"`
	GradualRollout         []PolicyGradualRollout         `tfsdk:"gradual_rollout"`
	AnyApproval            []PolicyAnyApproval            `tfsdk:"any_approval"`
	EnvironmentProgression []PolicyEnvironmentProgression `tfsdk:"environment_progression"`
	PlanValidationOpa      []PolicyPlanValidationOpa      `tfsdk:"plan_validation_opa"`
}

type PolicyVersionSelector struct {
	CreatedAt   types.String `tfsdk:"created_at"`
	ID          types.String `tfsdk:"id"`
	Selector    types.String `tfsdk:"selector"`
	Description types.String `tfsdk:"description"`
}

type PolicyVersionCooldown struct {
	CreatedAt types.String `tfsdk:"created_at"`
	ID        types.String `tfsdk:"id"`
	Duration  types.String `tfsdk:"duration"`
}

type PolicyDeploymentWindow struct {
	CreatedAt       types.String `tfsdk:"created_at"`
	ID              types.String `tfsdk:"id"`
	DurationMinutes types.Int64  `tfsdk:"duration_minutes"`
	Rrule           types.String `tfsdk:"rrule"`
	Timezone        types.String `tfsdk:"timezone"`
	AllowWindow     types.Bool   `tfsdk:"allow_window"`
}

type PolicyDeploymentDependency struct {
	CreatedAt         types.String `tfsdk:"created_at"`
	ID                types.String `tfsdk:"id"`
	DependsOnSelector types.String `tfsdk:"depends_on_selector"`
}

type PolicyGradualRollout struct {
	CreatedAt         types.String `tfsdk:"created_at"`
	ID                types.String `tfsdk:"id"`
	RolloutType       types.String `tfsdk:"rollout_type"`
	TimeScaleInterval types.Int64  `tfsdk:"time_scale_interval"`
}

type PolicyAnyApproval struct {
	CreatedAt    types.String `tfsdk:"created_at"`
	ID           types.String `tfsdk:"id"`
	MinApprovals types.Int64  `tfsdk:"min_approvals"`
}

type PolicyEnvironmentProgression struct {
	CreatedAt                    types.String  `tfsdk:"created_at"`
	ID                           types.String  `tfsdk:"id"`
	DependsOnEnvironmentSelector types.String  `tfsdk:"depends_on_environment_selector"`
	MinimumSuccessPercentage     types.Float64 `tfsdk:"minimum_success_percentage"`
	MinimumSoakTimeMinutes       types.Int64   `tfsdk:"minimum_soak_time_minutes"`
	MaximumAgeHours              types.Int64   `tfsdk:"maximum_age_hours"`
}

type PolicyVerificationRule struct {
	CreatedAt types.String               `tfsdk:"created_at"`
	ID        types.String               `tfsdk:"id"`
	TriggerOn types.String               `tfsdk:"trigger_on"`
	Metric    []PolicyVerificationMetric `tfsdk:"metric"`
}

type PolicyPlanValidationOpa struct {
	CreatedAt   types.String `tfsdk:"created_at"`
	ID          types.String `tfsdk:"id"`
	Name        types.String `tfsdk:"name"`
	Description types.String `tfsdk:"description"`
	Rego        types.String `tfsdk:"rego"`
}

type PolicyVerificationMetric struct {
	Name     types.String                 `tfsdk:"name"`
	Interval types.String                 `tfsdk:"interval"`
	Count    types.Int64                  `tfsdk:"count"`
	Success  *PolicyVerificationCondition `tfsdk:"success"`
	Failure  *PolicyVerificationCondition `tfsdk:"failure"`
	Sleep    *PolicySleepProvider         `tfsdk:"sleep"`
	Datadog  *PolicyDatadogProvider       `tfsdk:"datadog"`
}

type PolicySleepProvider struct {
	DurationSeconds types.Int64 `tfsdk:"duration_seconds"`
}

type PolicyVerificationCondition struct {
	Condition types.String `tfsdk:"condition"`
	Threshold types.Int64  `tfsdk:"threshold"`
}

type PolicyDatadogProvider struct {
	Site       types.String `tfsdk:"site"`
	Interval   types.String `tfsdk:"interval"`
	Queries    types.Map    `tfsdk:"queries"`
	ApiKey     types.String `tfsdk:"api_key"`
	AppKey     types.String `tfsdk:"app_key"`
	Aggregator types.String `tfsdk:"aggregator"`
	Formula    types.String `tfsdk:"formula"`
}

type policyRulesModel struct {
	VersionSelector        []PolicyVersionSelector
	VersionCooldown        []PolicyVersionCooldown
	DeploymentWindow       []PolicyDeploymentWindow
	DeploymentDependency   []PolicyDeploymentDependency
	Verification           []PolicyVerificationRule
	GradualRollout         []PolicyGradualRollout
	AnyApproval            []PolicyAnyApproval
	EnvironmentProgression []PolicyEnvironmentProgression
	PlanValidationOpa      []PolicyPlanValidationOpa
}

func selectorValueSet(value types.String) bool {
	return !value.IsNull() && !value.IsUnknown() && value.ValueString() != ""
}

func selectorIDValue(value types.String) string {
	if selectorValueSet(value) {
		return value.ValueString()
	}
	return uuid.NewString()
}

func createdAtTimestamp(value types.String) *timestamppb.Timestamp {
	if selectorValueSet(value) {
		if t, err := time.Parse(time.RFC3339, value.ValueString()); err == nil {
			return timestamppb.New(t)
		}
	}
	return timestamppb.New(time.Now().UTC())
}

func formatDuration(value time.Duration) string {
	if value%time.Hour == 0 {
		return fmt.Sprintf("%dh", int64(value/time.Hour))
	}
	if value%time.Minute == 0 {
		return fmt.Sprintf("%dm", int64(value/time.Minute))
	}
	if value%time.Second == 0 {
		return fmt.Sprintf("%ds", int64(value/time.Second))
	}
	return value.String()
}

func int64ValueSet(value types.Int64) bool {
	return !value.IsNull() && !value.IsUnknown()
}

func float64ValueSet(value types.Float64) bool {
	return !value.IsNull() && !value.IsUnknown()
}

func defaultInt64(value types.Int64, fallback int64) int64 {
	if value.IsNull() || value.IsUnknown() {
		return fallback
	}
	return value.ValueInt64()
}

func defaultBool(value types.Bool, fallback bool) bool {
	if value.IsNull() || value.IsUnknown() {
		return fallback
	}
	return value.ValueBool()
}

// policyRulesFromModel builds the proto PolicyRule list from the Terraform
// model. Each rule kind is attached to its own *PolicyRule carrying the rule
// id/policy_id/created_at envelope.
func policyRulesFromModel(data PolicyResourceModel) ([]*apiv1.PolicyRule, diag.Diagnostics) {
	var diags diag.Diagnostics
	rules := make([]*apiv1.PolicyRule, 0)

	for _, vs := range data.VersionSelector {
		cel := normalizeCEL(vs.Selector)
		if cel == "" {
			diags.AddError("Invalid version selector", "selector must be set")
			continue
		}
		rule := &apiv1.VersionSelectorRule{
			Selector: cel,
		}
		if selectorValueSet(vs.Description) {
			rule.Description = vs.Description.ValueStringPointer()
		}
		rules = append(rules, &apiv1.PolicyRule{
			Id:              selectorIDValue(vs.ID),
			CreatedAt:       createdAtTimestamp(vs.CreatedAt),
			VersionSelector: rule,
		})
	}

	for _, cooldown := range data.VersionCooldown {
		seconds, err := parseDurationSeconds(cooldown.Duration)
		if err != nil {
			diags.AddError("Invalid version cooldown duration", err.Error())
			continue
		}
		rules = append(rules, &apiv1.PolicyRule{
			Id:        selectorIDValue(cooldown.ID),
			CreatedAt: createdAtTimestamp(cooldown.CreatedAt),
			VersionCooldown: &apiv1.VersionCooldownRule{
				IntervalSeconds: int32(seconds),
			},
		})
	}

	for _, window := range data.DeploymentWindow {
		rule := &apiv1.DeploymentWindowRule{
			AllowWindow:     defaultBool(window.AllowWindow, true),
			DurationMinutes: int32(window.DurationMinutes.ValueInt64()),
			Rrule:           window.Rrule.ValueString(),
		}
		if selectorValueSet(window.Timezone) {
			rule.Timezone = window.Timezone.ValueStringPointer()
		}
		rules = append(rules, &apiv1.PolicyRule{
			Id:               selectorIDValue(window.ID),
			CreatedAt:        createdAtTimestamp(window.CreatedAt),
			DeploymentWindow: rule,
		})
	}

	for _, dep := range data.DeploymentDependency {
		rules = append(rules, &apiv1.PolicyRule{
			Id:        selectorIDValue(dep.ID),
			CreatedAt: createdAtTimestamp(dep.CreatedAt),
			DeploymentDependency: &apiv1.DeploymentDependencyRule{
				DependsOn: dep.DependsOnSelector.ValueString(),
			},
		})
	}

	for _, verification := range data.Verification {
		verificationRule, err := policyVerificationRuleFromModel(verification)
		if err != nil {
			diags.AddError("Invalid verification rule", err.Error())
			continue
		}
		rules = append(rules, &apiv1.PolicyRule{
			Id:           selectorIDValue(verification.ID),
			CreatedAt:    createdAtTimestamp(verification.CreatedAt),
			Verification: verificationRule,
		})
	}

	for _, rollout := range data.GradualRollout {
		rules = append(rules, &apiv1.PolicyRule{
			Id:        selectorIDValue(rollout.ID),
			CreatedAt: createdAtTimestamp(rollout.CreatedAt),
			GradualRollout: &apiv1.GradualRolloutRule{
				RolloutType:       rollout.RolloutType.ValueString(),
				TimeScaleInterval: int32(rollout.TimeScaleInterval.ValueInt64()),
			},
		})
	}

	for _, approval := range data.AnyApproval {
		rules = append(rules, &apiv1.PolicyRule{
			Id:        selectorIDValue(approval.ID),
			CreatedAt: createdAtTimestamp(approval.CreatedAt),
			AnyApproval: &apiv1.AnyApprovalRule{
				MinApprovals: int32(approval.MinApprovals.ValueInt64()),
			},
		})
	}

	for _, progression := range data.EnvironmentProgression {
		cel := normalizeCEL(progression.DependsOnEnvironmentSelector)
		if cel == "" {
			diags.AddError("Invalid environment progression selector", "depends_on_environment_selector must be set")
			continue
		}
		rule := &apiv1.EnvironmentProgressionRule{
			DependsOnEnvironmentSelector: cel,
		}
		if float64ValueSet(progression.MinimumSuccessPercentage) {
			val := float32(progression.MinimumSuccessPercentage.ValueFloat64())
			rule.MinimumSuccessPercentage = &val
		}
		if int64ValueSet(progression.MinimumSoakTimeMinutes) {
			rule.MinimumSoakTimeMinutes = int32(progression.MinimumSoakTimeMinutes.ValueInt64())
		}
		if int64ValueSet(progression.MaximumAgeHours) {
			val := int32(progression.MaximumAgeHours.ValueInt64())
			rule.MaximumAgeHours = &val
		}
		rules = append(rules, &apiv1.PolicyRule{
			Id:                     selectorIDValue(progression.ID),
			CreatedAt:              createdAtTimestamp(progression.CreatedAt),
			EnvironmentProgression: rule,
		})
	}

	for _, opa := range data.PlanValidationOpa {
		name := opa.Name.ValueString()
		rego := opa.Rego.ValueString()
		if name == "" || rego == "" {
			diags.AddError("Invalid plan validation rule", "name and rego must be set")
			continue
		}
		rule := &apiv1.PlanValidationOpaRule{
			Name: name,
			Rego: rego,
		}
		if selectorValueSet(opa.Description) {
			rule.Description = opa.Description.ValueStringPointer()
		}
		rules = append(rules, &apiv1.PolicyRule{
			Id:                selectorIDValue(opa.ID),
			CreatedAt:         createdAtTimestamp(opa.CreatedAt),
			PlanValidationOpa: rule,
		})
	}

	return rules, diags
}

// policyVerificationRuleFromModel builds a proto VerificationRule. Proto carries
// the metrics as a list of structpb.Struct values rather than typed messages, so
// each metric is encoded as the same JSON object the engine accepts
// (VerificationMetricSpec shape).
func policyVerificationRuleFromModel(model PolicyVerificationRule) (*apiv1.VerificationRule, error) {
	if len(model.Metric) == 0 {
		return nil, fmt.Errorf("verification rule must define at least one metric")
	}

	metrics := make([]*structpb.Struct, 0, len(model.Metric))
	for _, metric := range model.Metric {
		spec, err := policyVerificationMetricStruct(metric)
		if err != nil {
			return nil, err
		}
		metrics = append(metrics, spec)
	}

	rule := &apiv1.VerificationRule{
		Metrics: metrics,
	}

	if selectorValueSet(model.TriggerOn) {
		rule.TriggerOn = model.TriggerOn.ValueStringPointer()
	}

	return rule, nil
}

// policyVerificationMetricStruct encodes a single verification metric as a
// structpb.Struct matching the engine's VerificationMetricSpec JSON contract.
func policyVerificationMetricStruct(model PolicyVerificationMetric) (*structpb.Struct, error) {
	if model.Success == nil {
		return nil, fmt.Errorf("metric success block is required")
	}

	hasSleep := model.Sleep != nil
	hasDatadog := model.Datadog != nil
	if !hasSleep && !hasDatadog {
		return nil, fmt.Errorf("exactly one of sleep or datadog provider block is required")
	}
	if hasSleep && hasDatadog {
		return nil, fmt.Errorf("only one of sleep or datadog provider block can be set")
	}

	intervalSeconds, err := parseDurationSeconds(model.Interval)
	if err != nil {
		return nil, err
	}

	count := model.Count.ValueInt64()
	if count <= 0 {
		return nil, fmt.Errorf("metric count must be greater than zero")
	}

	successCondition := model.Success.Condition.ValueString()
	if successCondition == "" {
		return nil, fmt.Errorf("success condition must be set")
	}

	var provider map[string]any
	if hasSleep {
		provider, err = policySleepProviderMap(*model.Sleep)
	} else {
		provider, err = policyDatadogProviderMap(*model.Datadog)
	}
	if err != nil {
		return nil, err
	}

	spec := map[string]any{
		"name":             model.Name.ValueString(),
		"intervalSeconds":  float64(intervalSeconds),
		"count":            float64(count),
		"successCondition": successCondition,
		"provider":         provider,
	}

	if int64ValueSet(model.Success.Threshold) {
		spec["successThreshold"] = float64(model.Success.Threshold.ValueInt64())
	}
	if model.Failure != nil && selectorValueSet(model.Failure.Condition) {
		spec["failureCondition"] = model.Failure.Condition.ValueString()
	}
	if model.Failure != nil && int64ValueSet(model.Failure.Threshold) {
		spec["failureThreshold"] = float64(model.Failure.Threshold.ValueInt64())
	}

	return structpb.NewStruct(spec)
}

func policySleepProviderMap(model PolicySleepProvider) (map[string]any, error) {
	durationSeconds := defaultInt64(model.DurationSeconds, 30)
	if durationSeconds < 1 || durationSeconds > 3600 {
		return nil, fmt.Errorf("sleep duration_seconds must be between 1 and 3600, got %d", durationSeconds)
	}

	return map[string]any{
		"type":            "sleep",
		"durationSeconds": float64(durationSeconds),
	}, nil
}

func policyDatadogProviderMap(model PolicyDatadogProvider) (map[string]any, error) {
	if !selectorValueSet(model.ApiKey) {
		return nil, fmt.Errorf("datadog api_key is required")
	}
	if !selectorValueSet(model.AppKey) {
		return nil, fmt.Errorf("datadog app_key is required")
	}
	if model.Queries.IsNull() || model.Queries.IsUnknown() {
		return nil, fmt.Errorf("datadog queries is required")
	}

	queries, err := mapStringValue(model.Queries)
	if err != nil {
		return nil, fmt.Errorf("invalid provider queries: %w", err)
	}

	queriesAny := make(map[string]any, len(queries))
	for k, v := range queries {
		queriesAny[k] = v
	}

	provider := map[string]any{
		"type":    "datadog",
		"apiKey":  model.ApiKey.ValueString(),
		"appKey":  model.AppKey.ValueString(),
		"queries": queriesAny,
	}

	if selectorValueSet(model.Site) {
		provider["site"] = model.Site.ValueString()
	}
	if selectorValueSet(model.Interval) {
		intervalSeconds, err := parseDurationSeconds(model.Interval)
		if err != nil {
			return nil, err
		}
		provider["intervalSeconds"] = float64(intervalSeconds)
	}
	if selectorValueSet(model.Aggregator) {
		provider["aggregator"] = model.Aggregator.ValueString()
	}
	if selectorValueSet(model.Formula) {
		provider["formula"] = model.Formula.ValueString()
	}

	return provider, nil
}

// policyRulesToModel maps the proto PolicyRule list back into the Terraform
// model by inspecting which rule sub-message is non-nil on each rule.
func policyRulesToModel(rules []*apiv1.PolicyRule) (policyRulesModel, diag.Diagnostics) {
	var diags diag.Diagnostics
	result := policyRulesModel{}

	for _, rule := range rules {
		if rule == nil {
			continue
		}
		createdAt := optionalString(rfc3339(rule.GetCreatedAt()))
		id := types.StringValue(rule.GetId())

		switch {
		case rule.GetVersionSelector() != nil:
			vs := rule.GetVersionSelector()
			model := PolicyVersionSelector{
				CreatedAt:   createdAt,
				ID:          id,
				Selector:    types.StringValue(vs.GetSelector()),
				Description: optionalString(vs.GetDescription()),
			}
			result.VersionSelector = append(result.VersionSelector, model)
		case rule.GetVersionCooldown() != nil:
			duration := time.Duration(rule.GetVersionCooldown().GetIntervalSeconds()) * time.Second
			result.VersionCooldown = append(result.VersionCooldown, PolicyVersionCooldown{
				CreatedAt: createdAt,
				ID:        id,
				Duration:  types.StringValue(formatDuration(duration)),
			})
		case rule.GetDeploymentWindow() != nil:
			window := rule.GetDeploymentWindow()
			model := PolicyDeploymentWindow{
				CreatedAt:       createdAt,
				ID:              id,
				DurationMinutes: types.Int64Value(int64(window.GetDurationMinutes())),
				Rrule:           types.StringValue(window.GetRrule()),
				Timezone:        optionalString(window.GetTimezone()),
				AllowWindow:     types.BoolValue(window.GetAllowWindow()),
			}
			result.DeploymentWindow = append(result.DeploymentWindow, model)
		case rule.GetDeploymentDependency() != nil:
			result.DeploymentDependency = append(result.DeploymentDependency, PolicyDeploymentDependency{
				CreatedAt:         createdAt,
				ID:                id,
				DependsOnSelector: types.StringValue(rule.GetDeploymentDependency().GetDependsOn()),
			})
		case rule.GetVerification() != nil:
			verification, err := policyVerificationRuleToModel(rule.GetVerification())
			if err != nil {
				diags.AddError("Invalid verification rule", err.Error())
				continue
			}
			verification.CreatedAt = createdAt
			verification.ID = id
			result.Verification = append(result.Verification, verification)
		case rule.GetGradualRollout() != nil:
			rollout := rule.GetGradualRollout()
			result.GradualRollout = append(result.GradualRollout, PolicyGradualRollout{
				CreatedAt:         createdAt,
				ID:                id,
				RolloutType:       types.StringValue(rollout.GetRolloutType()),
				TimeScaleInterval: types.Int64Value(int64(rollout.GetTimeScaleInterval())),
			})
		case rule.GetAnyApproval() != nil:
			result.AnyApproval = append(result.AnyApproval, PolicyAnyApproval{
				CreatedAt:    createdAt,
				ID:           id,
				MinApprovals: types.Int64Value(int64(rule.GetAnyApproval().GetMinApprovals())),
			})
		case rule.GetEnvironmentProgression() != nil:
			progression := rule.GetEnvironmentProgression()
			model := PolicyEnvironmentProgression{
				CreatedAt:                    createdAt,
				ID:                           id,
				DependsOnEnvironmentSelector: types.StringValue(progression.GetDependsOnEnvironmentSelector()),
				MinimumSuccessPercentage:     types.Float64Null(),
				MinimumSoakTimeMinutes:       types.Int64Value(int64(progression.GetMinimumSoakTimeMinutes())),
				MaximumAgeHours:              types.Int64Null(),
			}
			if progression.MinimumSuccessPercentage != nil {
				model.MinimumSuccessPercentage = types.Float64Value(float64(progression.GetMinimumSuccessPercentage()))
			}
			if progression.MaximumAgeHours != nil {
				model.MaximumAgeHours = types.Int64Value(int64(progression.GetMaximumAgeHours()))
			}
			result.EnvironmentProgression = append(result.EnvironmentProgression, model)
		case rule.GetPlanValidationOpa() != nil:
			opa := rule.GetPlanValidationOpa()
			model := PolicyPlanValidationOpa{
				CreatedAt:   createdAt,
				ID:          id,
				Name:        types.StringValue(opa.GetName()),
				Description: optionalString(opa.GetDescription()),
				Rego:        types.StringValue(opa.GetRego()),
			}
			result.PlanValidationOpa = append(result.PlanValidationOpa, model)
		}
	}

	return result, diags
}

func ensurePolicyIDs(plan *PolicyResourceModel, state *PolicyResourceModel) {
	mergeVersionSelectorIDs(plan.VersionSelector, versionSelectorListFromState(state))
	mergeCooldownIDs(plan.VersionCooldown, cooldownListFromState(state))
	mergeWindowIDs(plan.DeploymentWindow, windowListFromState(state))
	mergeDeploymentDependencyIDs(plan.DeploymentDependency, deploymentDependencyListFromState(state))
	mergeVerificationIDs(plan.Verification, verificationListFromState(state))
	mergeGradualRolloutIDs(plan.GradualRollout, gradualRolloutListFromState(state))
	mergeAnyApprovalIDs(plan.AnyApproval, anyApprovalListFromState(state))
	mergeEnvironmentProgressionIDs(plan.EnvironmentProgression, environmentProgressionListFromState(state))
	mergePlanValidationOpaIDs(plan.PlanValidationOpa, planValidationOpaListFromState(state))
}

func ensurePolicyRuleCreatedAt(plan *PolicyResourceModel, state *PolicyResourceModel) {
	mergeVersionSelectorCreatedAt(plan.VersionSelector, versionSelectorListFromState(state))
	mergeCooldownCreatedAt(plan.VersionCooldown, cooldownListFromState(state))
	mergeWindowCreatedAt(plan.DeploymentWindow, windowListFromState(state))
	mergeDeploymentDependencyCreatedAt(plan.DeploymentDependency, deploymentDependencyListFromState(state))
	mergeVerificationCreatedAt(plan.Verification, verificationListFromState(state))
	mergeGradualRolloutCreatedAt(plan.GradualRollout, gradualRolloutListFromState(state))
	mergeAnyApprovalCreatedAt(plan.AnyApproval, anyApprovalListFromState(state))
	mergeEnvironmentProgressionCreatedAt(plan.EnvironmentProgression, environmentProgressionListFromState(state))
	mergePlanValidationOpaCreatedAt(plan.PlanValidationOpa, planValidationOpaListFromState(state))
}

func versionSelectorListFromState(state *PolicyResourceModel) []PolicyVersionSelector {
	if state == nil {
		return nil
	}
	return state.VersionSelector
}

func mergeVersionSelectorIDs(plan []PolicyVersionSelector, state []PolicyVersionSelector) {
	for i := range plan {
		if selectorValueSet(plan[i].ID) {
			continue
		}
		if i < len(state) && selectorValueSet(state[i].ID) {
			plan[i].ID = state[i].ID
			continue
		}
		plan[i].ID = types.StringValue(uuid.NewString())
	}
}

func mergeVersionSelectorCreatedAt(plan []PolicyVersionSelector, state []PolicyVersionSelector) {
	for i := range plan {
		if selectorValueSet(plan[i].CreatedAt) {
			continue
		}
		if i < len(state) && selectorValueSet(state[i].CreatedAt) {
			plan[i].CreatedAt = state[i].CreatedAt
			continue
		}
		plan[i].CreatedAt = types.StringValue(time.Now().UTC().Format(time.RFC3339))
	}
}

func cooldownListFromState(state *PolicyResourceModel) []PolicyVersionCooldown {
	if state == nil {
		return nil
	}
	return state.VersionCooldown
}

func windowListFromState(state *PolicyResourceModel) []PolicyDeploymentWindow {
	if state == nil {
		return nil
	}
	return state.DeploymentWindow
}

func verificationListFromState(state *PolicyResourceModel) []PolicyVerificationRule {
	if state == nil {
		return nil
	}
	return state.Verification
}

func deploymentDependencyListFromState(state *PolicyResourceModel) []PolicyDeploymentDependency {
	if state == nil {
		return nil
	}
	return state.DeploymentDependency
}

func mergeCooldownIDs(plan []PolicyVersionCooldown, state []PolicyVersionCooldown) {
	for i := range plan {
		if selectorValueSet(plan[i].ID) {
			continue
		}
		if i < len(state) && selectorValueSet(state[i].ID) {
			plan[i].ID = state[i].ID
			continue
		}
		plan[i].ID = types.StringValue(uuid.NewString())
	}
}

func mergeCooldownCreatedAt(plan []PolicyVersionCooldown, state []PolicyVersionCooldown) {
	for i := range plan {
		if selectorValueSet(plan[i].CreatedAt) {
			continue
		}
		if i < len(state) && selectorValueSet(state[i].CreatedAt) {
			plan[i].CreatedAt = state[i].CreatedAt
			continue
		}
		plan[i].CreatedAt = types.StringValue(time.Now().UTC().Format(time.RFC3339))
	}
}

func mergeWindowIDs(plan []PolicyDeploymentWindow, state []PolicyDeploymentWindow) {
	for i := range plan {
		if selectorValueSet(plan[i].ID) {
			continue
		}
		if i < len(state) && selectorValueSet(state[i].ID) {
			plan[i].ID = state[i].ID
			continue
		}
		plan[i].ID = types.StringValue(uuid.NewString())
	}
}

func mergeWindowCreatedAt(plan []PolicyDeploymentWindow, state []PolicyDeploymentWindow) {
	for i := range plan {
		if selectorValueSet(plan[i].CreatedAt) {
			continue
		}
		if i < len(state) && selectorValueSet(state[i].CreatedAt) {
			plan[i].CreatedAt = state[i].CreatedAt
			continue
		}
		plan[i].CreatedAt = types.StringValue(time.Now().UTC().Format(time.RFC3339))
	}
}

func mergeDeploymentDependencyIDs(plan []PolicyDeploymentDependency, state []PolicyDeploymentDependency) {
	for i := range plan {
		if selectorValueSet(plan[i].ID) {
			continue
		}
		if i < len(state) && selectorValueSet(state[i].ID) {
			plan[i].ID = state[i].ID
			continue
		}
		plan[i].ID = types.StringValue(uuid.NewString())
	}
}

func mergeDeploymentDependencyCreatedAt(plan []PolicyDeploymentDependency, state []PolicyDeploymentDependency) {
	for i := range plan {
		if selectorValueSet(plan[i].CreatedAt) {
			continue
		}
		if i < len(state) && selectorValueSet(state[i].CreatedAt) {
			plan[i].CreatedAt = state[i].CreatedAt
			continue
		}
		plan[i].CreatedAt = types.StringValue(time.Now().UTC().Format(time.RFC3339))
	}
}

func mergeVerificationIDs(plan []PolicyVerificationRule, state []PolicyVerificationRule) {
	for i := range plan {
		if selectorValueSet(plan[i].ID) {
			continue
		}
		if i < len(state) && selectorValueSet(state[i].ID) {
			plan[i].ID = state[i].ID
			continue
		}
		plan[i].ID = types.StringValue(uuid.NewString())
	}
}

func mergeVerificationCreatedAt(plan []PolicyVerificationRule, state []PolicyVerificationRule) {
	for i := range plan {
		if selectorValueSet(plan[i].CreatedAt) {
			continue
		}
		if i < len(state) && selectorValueSet(state[i].CreatedAt) {
			plan[i].CreatedAt = state[i].CreatedAt
			continue
		}
		plan[i].CreatedAt = types.StringValue(time.Now().UTC().Format(time.RFC3339))
	}
}

func gradualRolloutListFromState(state *PolicyResourceModel) []PolicyGradualRollout {
	if state == nil {
		return nil
	}
	return state.GradualRollout
}

func anyApprovalListFromState(state *PolicyResourceModel) []PolicyAnyApproval {
	if state == nil {
		return nil
	}
	return state.AnyApproval
}

func environmentProgressionListFromState(state *PolicyResourceModel) []PolicyEnvironmentProgression {
	if state == nil {
		return nil
	}
	return state.EnvironmentProgression
}

func mergeGradualRolloutIDs(plan []PolicyGradualRollout, state []PolicyGradualRollout) {
	for i := range plan {
		if selectorValueSet(plan[i].ID) {
			continue
		}
		if i < len(state) && selectorValueSet(state[i].ID) {
			plan[i].ID = state[i].ID
			continue
		}
		plan[i].ID = types.StringValue(uuid.NewString())
	}
}

func mergeGradualRolloutCreatedAt(plan []PolicyGradualRollout, state []PolicyGradualRollout) {
	for i := range plan {
		if selectorValueSet(plan[i].CreatedAt) {
			continue
		}
		if i < len(state) && selectorValueSet(state[i].CreatedAt) {
			plan[i].CreatedAt = state[i].CreatedAt
			continue
		}
		plan[i].CreatedAt = types.StringValue(time.Now().UTC().Format(time.RFC3339))
	}
}

func mergeAnyApprovalIDs(plan []PolicyAnyApproval, state []PolicyAnyApproval) {
	for i := range plan {
		if selectorValueSet(plan[i].ID) {
			continue
		}
		if i < len(state) && selectorValueSet(state[i].ID) {
			plan[i].ID = state[i].ID
			continue
		}
		plan[i].ID = types.StringValue(uuid.NewString())
	}
}

func mergeAnyApprovalCreatedAt(plan []PolicyAnyApproval, state []PolicyAnyApproval) {
	for i := range plan {
		if selectorValueSet(plan[i].CreatedAt) {
			continue
		}
		if i < len(state) && selectorValueSet(state[i].CreatedAt) {
			plan[i].CreatedAt = state[i].CreatedAt
			continue
		}
		plan[i].CreatedAt = types.StringValue(time.Now().UTC().Format(time.RFC3339))
	}
}

func mergeEnvironmentProgressionIDs(plan []PolicyEnvironmentProgression, state []PolicyEnvironmentProgression) {
	for i := range plan {
		if selectorValueSet(plan[i].ID) {
			continue
		}
		if i < len(state) && selectorValueSet(state[i].ID) {
			plan[i].ID = state[i].ID
			continue
		}
		plan[i].ID = types.StringValue(uuid.NewString())
	}
}

func mergeEnvironmentProgressionCreatedAt(plan []PolicyEnvironmentProgression, state []PolicyEnvironmentProgression) {
	for i := range plan {
		if selectorValueSet(plan[i].CreatedAt) {
			continue
		}
		if i < len(state) && selectorValueSet(state[i].CreatedAt) {
			plan[i].CreatedAt = state[i].CreatedAt
			continue
		}
		plan[i].CreatedAt = types.StringValue(time.Now().UTC().Format(time.RFC3339))
	}
}

func planValidationOpaListFromState(state *PolicyResourceModel) []PolicyPlanValidationOpa {
	if state == nil {
		return nil
	}
	return state.PlanValidationOpa
}

func mergePlanValidationOpaIDs(plan []PolicyPlanValidationOpa, state []PolicyPlanValidationOpa) {
	for i := range plan {
		if selectorValueSet(plan[i].ID) {
			continue
		}
		if i < len(state) && selectorValueSet(state[i].ID) {
			plan[i].ID = state[i].ID
			continue
		}
		plan[i].ID = types.StringValue(uuid.NewString())
	}
}

func mergePlanValidationOpaCreatedAt(plan []PolicyPlanValidationOpa, state []PolicyPlanValidationOpa) {
	for i := range plan {
		if selectorValueSet(plan[i].CreatedAt) {
			continue
		}
		if i < len(state) && selectorValueSet(state[i].CreatedAt) {
			plan[i].CreatedAt = state[i].CreatedAt
			continue
		}
		plan[i].CreatedAt = types.StringValue(time.Now().UTC().Format(time.RFC3339))
	}
}

// policyVerificationRuleToModel converts a proto VerificationRule back into the
// Terraform model. Each metric is a structpb.Struct that follows the engine's
// VerificationMetricSpec JSON contract.
func policyVerificationRuleToModel(rule *apiv1.VerificationRule) (PolicyVerificationRule, error) {
	model := PolicyVerificationRule{
		TriggerOn: types.StringNull(),
		Metric:    make([]PolicyVerificationMetric, 0, len(rule.GetMetrics())),
	}

	if rule.TriggerOn != nil {
		model.TriggerOn = types.StringValue(rule.GetTriggerOn())
	}

	for _, metric := range rule.GetMetrics() {
		m, err := policyVerificationMetricToModel(metric)
		if err != nil {
			return PolicyVerificationRule{}, err
		}
		model.Metric = append(model.Metric, m)
	}

	return model, nil
}

func policyVerificationMetricToModel(metric *structpb.Struct) (PolicyVerificationMetric, error) {
	if metric == nil {
		return PolicyVerificationMetric{}, fmt.Errorf("metric struct is nil")
	}
	raw := metric.AsMap()

	model := PolicyVerificationMetric{
		Name:     types.StringValue(structString(raw, "name")),
		Interval: types.StringValue((time.Duration(structInt(raw, "intervalSeconds")) * time.Second).String()),
		Count:    types.Int64Value(structInt(raw, "count")),
		Success: &PolicyVerificationCondition{
			Condition: types.StringValue(structString(raw, "successCondition")),
			Threshold: types.Int64Null(),
		},
		Failure: nil,
		Sleep:   nil,
		Datadog: nil,
	}

	if _, ok := raw["successThreshold"]; ok {
		model.Success.Threshold = types.Int64Value(structInt(raw, "successThreshold"))
	}
	_, hasFailureCondition := raw["failureCondition"]
	_, hasFailureThreshold := raw["failureThreshold"]
	if hasFailureCondition || hasFailureThreshold {
		model.Failure = &PolicyVerificationCondition{
			Condition: types.StringNull(),
			Threshold: types.Int64Null(),
		}
		if hasFailureCondition {
			model.Failure.Condition = types.StringValue(structString(raw, "failureCondition"))
		}
		if hasFailureThreshold {
			model.Failure.Threshold = types.Int64Value(structInt(raw, "failureThreshold"))
		}
	}

	provider, ok := raw["provider"].(map[string]any)
	if !ok {
		return PolicyVerificationMetric{}, fmt.Errorf("metric provider is missing or malformed")
	}

	providerType := structString(provider, "type")
	switch providerType {
	case "sleep":
		model.Sleep = &PolicySleepProvider{
			DurationSeconds: types.Int64Value(structInt(provider, "durationSeconds")),
		}
		return model, nil
	case "datadog":
	default:
		return PolicyVerificationMetric{}, fmt.Errorf("unsupported metric provider type: %q", providerType)
	}

	datadog := &PolicyDatadogProvider{
		Site:       types.StringNull(),
		Interval:   types.StringNull(),
		Queries:    types.MapNull(types.StringType),
		ApiKey:     types.StringValue(structString(provider, "apiKey")),
		AppKey:     types.StringValue(structString(provider, "appKey")),
		Aggregator: types.StringNull(),
		Formula:    types.StringNull(),
	}
	if _, ok := provider["site"]; ok {
		datadog.Site = types.StringValue(structString(provider, "site"))
	}
	if _, ok := provider["intervalSeconds"]; ok {
		datadog.Interval = types.StringValue((time.Duration(structInt(provider, "intervalSeconds")) * time.Second).String())
	}
	if q, ok := provider["queries"].(map[string]any); ok && len(q) > 0 {
		queries := make(map[string]string, len(q))
		for k, v := range q {
			if s, ok := v.(string); ok {
				queries[k] = s
			}
		}
		result, _ := types.MapValueFrom(context.Background(), types.StringType, queries)
		datadog.Queries = result
	}
	if _, ok := provider["aggregator"]; ok {
		datadog.Aggregator = types.StringValue(structString(provider, "aggregator"))
	}
	if _, ok := provider["formula"]; ok {
		datadog.Formula = types.StringValue(structString(provider, "formula"))
	}
	model.Datadog = datadog

	return model, nil
}

// structString reads a string value from a decoded structpb map.
func structString(m map[string]any, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}

// structInt reads a numeric value from a decoded structpb map. structpb decodes
// all JSON numbers as float64, so the value is rounded to the nearest int64.
func structInt(m map[string]any, key string) int64 {
	if v, ok := m[key].(float64); ok {
		return int64(v)
	}
	return 0
}

func parseDurationSeconds(value types.String) (int64, error) {
	if value.IsNull() || value.IsUnknown() {
		return 0, fmt.Errorf("duration must be set")
	}
	raw := value.ValueString()
	duration, err := time.ParseDuration(raw)
	if err != nil {
		return 0, fmt.Errorf("invalid duration %q", raw)
	}
	if duration < 0 {
		return 0, fmt.Errorf("duration must be non-negative")
	}
	if duration%time.Second != 0 {
		return 0, fmt.Errorf("duration %q must be a whole number of seconds", raw)
	}
	return int64(duration.Seconds()), nil
}

func mapStringValue(value types.Map) (map[string]string, error) {
	if value.IsNull() || value.IsUnknown() {
		return nil, fmt.Errorf("map must be set")
	}
	var decoded map[string]string
	diags := value.ElementsAs(context.Background(), &decoded, false)
	if diags.HasError() {
		return nil, fmt.Errorf("invalid map value")
	}
	return decoded, nil
}
