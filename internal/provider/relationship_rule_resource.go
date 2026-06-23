// Copyright IBM Corp. 2021, 2026

package provider

import (
	"context"

	apiv1 "buf.build/gen/go/ctrlplane/ctrlplane/protocolbuffers/go/ctrlplane/api/v1"
	connect "connectrpc.com/connect"
	"github.com/ctrlplanedev/terraform-provider-ctrlplane/internal/api"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/defaults"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/mapdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ resource.Resource = &RelationshipRuleResource{}
var _ resource.ResourceWithImportState = &RelationshipRuleResource{}
var _ resource.ResourceWithConfigure = &RelationshipRuleResource{}

func NewRelationshipRuleResource() resource.Resource {
	return &RelationshipRuleResource{}
}

type RelationshipRuleResource struct {
	workspace *api.WorkspaceClient
}

type RelationshipRuleResourceModel struct {
	ID          types.String `tfsdk:"id"`
	Name        types.String `tfsdk:"name"`
	Reference   types.String `tfsdk:"reference"`
	Description types.String `tfsdk:"description"`
	Cel         types.String `tfsdk:"matcher"`
	Metadata    types.Map    `tfsdk:"metadata"`
}

func (r *RelationshipRuleResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_relationship_rule"
}

func (r *RelationshipRuleResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func (r *RelationshipRuleResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *RelationshipRuleResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a relationship rule in Ctrlplane. Relationship rules define how entities (resources, deployments, environments) are related to each other.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:    true,
				Description: "The unique identifier of the relationship rule",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				Required:    true,
				Description: "The name of the relationship rule",
			},
			"reference": schema.StringAttribute{
				Required:    true,
				Description: "A unique reference identifier for the relationship rule",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"description": schema.StringAttribute{
				Optional:    true,
				Description: "A description of the relationship rule",
			},
			"matcher": schema.StringAttribute{
				Required:    true,
				Description: "A CEL expression that defines the relationship rule",
				PlanModifiers: []planmodifier.String{
					celNormalized(),
				},
			},
			"metadata": schema.MapAttribute{
				Optional:    true,
				Computed:    true,
				Description: "Metadata key-value pairs for the relationship rule",
				ElementType: types.StringType,
				Default: func() defaults.Map {
					empty, _ := types.MapValueFrom(context.Background(), types.StringType, map[string]string{})
					return mapdefault.StaticValue(empty)
				}(),
			},
		},
	}
}

func (r *RelationshipRuleResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data RelationshipRuleResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	cel := normalizeCEL(data.Cel)

	var metadata map[string]string
	if p := stringMapPointer(data.Metadata); p != nil {
		metadata = *p
	}

	created, err := r.workspace.Policy.CreateRelationshipRule(ctx, connect.NewRequest(&apiv1.CreateRelationshipRuleRequest{
		WorkspaceId: r.workspace.WorkspaceID(),
		Name:        data.Name.ValueString(),
		Description: data.Description.ValueStringPointer(),
		Reference:   data.Reference.ValueString(),
		Cel:         cel,
		Metadata:    metadata,
	}))
	if err != nil {
		addConnectError(&resp.Diagnostics, "Failed to create relationship rule", err)
		return
	}

	rule := created.Msg
	data.ID = types.StringValue(rule.GetId())
	data.Name = types.StringValue(rule.GetName())
	data.Reference = types.StringValue(rule.GetReference())
	data.Description = optionalString(rule.GetDescription())
	data.Cel = types.StringValue(rule.GetCel())
	data.Metadata = metadataMapValue(rule.GetMetadata())

	resp.Diagnostics.Append(resp.State.Set(ctx, data)...)
}

func (r *RelationshipRuleResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data RelationshipRuleResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	got, err := r.workspace.Policy.GetRelationshipRule(ctx, connect.NewRequest(&apiv1.GetRelationshipRuleRequest{
		WorkspaceId:        r.workspace.WorkspaceID(),
		RelationshipRuleId: data.ID.ValueString(),
	}))
	if err != nil {
		if isNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		addConnectError(&resp.Diagnostics, "Failed to read relationship rule", err)
		return
	}

	rule := got.Msg
	data.ID = types.StringValue(rule.GetId())
	data.Name = types.StringValue(rule.GetName())
	data.Reference = types.StringValue(rule.GetReference())
	data.Description = optionalString(rule.GetDescription())
	data.Cel = types.StringValue(rule.GetCel())
	data.Metadata = metadataMapValue(rule.GetMetadata())

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *RelationshipRuleResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data RelationshipRuleResourceModel
	var state RelationshipRuleResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	data.ID = state.ID

	cel := normalizeCEL(data.Cel)

	var metadata map[string]string
	if p := stringMapPointer(data.Metadata); p != nil {
		metadata = *p
	}

	upserted, err := r.workspace.Policy.UpsertRelationshipRule(ctx, connect.NewRequest(&apiv1.UpsertRelationshipRuleRequest{
		WorkspaceId:        r.workspace.WorkspaceID(),
		RelationshipRuleId: data.ID.ValueString(),
		Name:               data.Name.ValueString(),
		Description:        data.Description.ValueStringPointer(),
		Reference:          data.Reference.ValueString(),
		Cel:                cel,
		Metadata:           metadata,
	}))
	if err != nil {
		addConnectError(&resp.Diagnostics, "Failed to update relationship rule", err)
		return
	}

	rule := upserted.Msg
	if id := rule.GetId(); id != "" {
		data.ID = types.StringValue(id)
	}
	data.Name = types.StringValue(rule.GetName())
	data.Reference = types.StringValue(rule.GetReference())
	data.Description = optionalString(rule.GetDescription())
	data.Cel = types.StringValue(rule.GetCel())
	data.Metadata = metadataMapValue(rule.GetMetadata())

	resp.Diagnostics.Append(resp.State.Set(ctx, data)...)
}

func (r *RelationshipRuleResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data RelationshipRuleResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	_, err := r.workspace.Policy.DeleteRelationshipRule(ctx, connect.NewRequest(&apiv1.DeleteRelationshipRuleRequest{
		WorkspaceId:        r.workspace.WorkspaceID(),
		RelationshipRuleId: data.ID.ValueString(),
	}))
	if err != nil && !isNotFound(err) {
		addConnectError(&resp.Diagnostics, "Failed to delete relationship rule", err)
		return
	}
}
