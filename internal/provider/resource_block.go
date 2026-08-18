package provider

import (
	"context"
	"fmt"

	"github.com/JakeNeyer/ipam-go/ipam"
	"github.com/google/uuid"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

var _ resource.Resource = &BlockResource{}
var _ resource.ResourceWithImportState = &BlockResource{}

func NewBlockResource() resource.Resource {
	return &BlockResource{}
}

type BlockResource struct {
	api *ipam.Client
}

type BlockResourceModel struct {
	Id            types.String `tfsdk:"id"`
	Name          types.String `tfsdk:"name"`
	Cidr          types.String `tfsdk:"cidr"`
	TotalIps      types.String `tfsdk:"total_ips"` // string: derive-only, supports IPv6 /64 etc.
	UsedIps       types.String `tfsdk:"used_ips"`
	AvailableIps  types.String `tfsdk:"available_ips"`
	EnvironmentId types.String `tfsdk:"environment_id"`
	PoolId        types.String `tfsdk:"pool_id"`
}

func (r *BlockResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_block"
}

func (r *BlockResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "IPAM network block. A block is a CIDR range assigned to an environment; allocations are subnets within a block.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Block UUID.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"name": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Block name.",
			},
			"cidr": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "CIDR range (e.g. 10.0.0.0/8).",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"environment_id": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Environment UUID. Omit for orphaned blocks.",
			},
			"pool_id": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Pool UUID. Block CIDR must be contained in the pool's CIDR. Only for blocks in an environment.",
			},
			"total_ips": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Total IP count in the block (string; supports IPv6 /64 etc.).",
			},
			"used_ips": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "IPs used by allocations.",
			},
			"available_ips": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Available IPs.",
			},
		},
	}
}

func (r *BlockResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	api, ok := req.ProviderData.(*ipam.Client)
	if !ok {
		resp.Diagnostics.AddError("Unexpected provider type", fmt.Sprintf("Expected *ipam.Client, got %T", req.ProviderData))
		return
	}
	r.api = api
}

func (r *BlockResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan BlockResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	in := ipam.CreateBlockInput{
		Name: plan.Name.ValueString(),
		CIDR: plan.Cidr.ValueString(),
	}
	if envID := plan.EnvironmentId.ValueString(); envID != "" {
		id, diags := parseID("environment_id", envID)
		resp.Diagnostics.Append(diags...)
		in.EnvironmentID = id
	}
	if poolID := plan.PoolId.ValueString(); poolID != "" {
		id, diags := parseID("pool_id", poolID)
		resp.Diagnostics.Append(diags...)
		in.PoolID = id
	}
	if resp.Diagnostics.HasError() {
		return
	}
	out, err := r.api.CreateBlock(ctx, in)
	if err != nil {
		resp.Diagnostics.AddError("API error", err.Error())
		return
	}
	r.setModelFromAPI(&plan, out)
	tflog.Trace(ctx, "created ipam_block", map[string]interface{}{"id": out.ID.String()})
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *BlockResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state BlockResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	id, diags := parseID("block id", state.Id.ValueString())
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	out, err := r.api.GetBlock(ctx, id)
	if err != nil {
		resp.Diagnostics.AddError("API error", err.Error())
		return
	}
	r.setModelFromAPI(&state, out)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *BlockResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan BlockResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	id, diags := parseID("block id", plan.Id.ValueString())
	resp.Diagnostics.Append(diags...)
	in := ipam.UpdateBlockInput{Name: plan.Name.ValueString()}
	if envID := plan.EnvironmentId.ValueString(); envID != "" {
		eid, d := parseID("environment_id", envID)
		resp.Diagnostics.Append(d...)
		in.EnvironmentID = eid
	}
	if poolID := plan.PoolId.ValueString(); !plan.PoolId.IsUnknown() && poolID != "" {
		pid, d := parseID("pool_id", poolID)
		resp.Diagnostics.Append(d...)
		in.PoolID = pid
	}
	if resp.Diagnostics.HasError() {
		return
	}
	out, err := r.api.UpdateBlock(ctx, id, in)
	if err != nil {
		resp.Diagnostics.AddError("API error", err.Error())
		return
	}
	r.setModelFromAPI(&plan, out)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *BlockResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state BlockResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	id, diags := parseID("block id", state.Id.ValueString())
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.api.DeleteBlock(ctx, id); err != nil {
		resp.Diagnostics.AddError("API error", err.Error())
	}
}

func (r *BlockResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func (r *BlockResource) setModelFromAPI(m *BlockResourceModel, out *ipam.Block) {
	m.Id = types.StringValue(out.ID.String())
	m.Name = types.StringValue(out.Name)
	m.Cidr = types.StringValue(out.CIDR)
	m.EnvironmentId = types.StringValue(idString(out.EnvironmentID))
	if out.PoolID != uuid.Nil {
		m.PoolId = types.StringValue(out.PoolID.String())
	} else {
		m.PoolId = types.StringNull()
	}
	m.TotalIps = types.StringValue(out.TotalIPs)
	m.UsedIps = types.StringValue(out.UsedIPs)
	m.AvailableIps = types.StringValue(out.AvailableIPs)
}
