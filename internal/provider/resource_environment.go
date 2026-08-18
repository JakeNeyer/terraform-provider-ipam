package provider

import (
	"context"
	"fmt"

	"github.com/JakeNeyer/ipam-go/ipam"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

var _ resource.Resource = &EnvironmentResource{}
var _ resource.ResourceWithImportState = &EnvironmentResource{}

func NewEnvironmentResource() resource.Resource {
	return &EnvironmentResource{}
}

type EnvironmentResource struct {
	api *ipam.Client
}

type EnvironmentResourceModel struct {
	Id      types.String `tfsdk:"id"`
	Name    types.String `tfsdk:"name"`
	Pools   types.List   `tfsdk:"pools"`    // list of { name, cidr }
	PoolIds types.List   `tfsdk:"pool_ids"` // computed: UUIDs of created pools
}

type poolBlockModel struct {
	Name types.String `tfsdk:"name"`
	Cidr types.String `tfsdk:"cidr"`
}

func (r *EnvironmentResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_environment"
}

func (r *EnvironmentResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "IPAM environment. Environments group network blocks (e.g. prod, staging). Requires at least one pool.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Environment UUID.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"name": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Environment name.",
			},
			"pools": schema.ListNestedAttribute{
				Required:            true,
				MarkdownDescription: "At least one pool (CIDR range that blocks in this environment draw from).",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"name": schema.StringAttribute{
							Required:            true,
							MarkdownDescription: "Pool name.",
						},
						"cidr": schema.StringAttribute{
							Required:            true,
							MarkdownDescription: "Pool CIDR (e.g. 10.0.0.0/8).",
						},
					},
				},
			},
			"pool_ids": schema.ListAttribute{
				ElementType:         types.StringType,
				Computed:            true,
				MarkdownDescription: "UUIDs of pools created with this environment (same order as `pools`).",
			},
		},
	}
}

func (r *EnvironmentResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *EnvironmentResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan EnvironmentResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var poolBlocks []poolBlockModel
	resp.Diagnostics.Append(plan.Pools.ElementsAs(ctx, &poolBlocks, false)...)
	if resp.Diagnostics.HasError() {
		return
	}
	poolList := make([]ipam.PoolSpec, 0, len(poolBlocks))
	for _, pm := range poolBlocks {
		poolList = append(poolList, ipam.PoolSpec{Name: pm.Name.ValueString(), CIDR: pm.Cidr.ValueString()})
	}
	if len(poolList) == 0 {
		resp.Diagnostics.AddError("Invalid config", "at least one pool is required")
		return
	}
	out, err := r.api.CreateEnvironment(ctx, ipam.CreateEnvironmentInput{
		Name:  plan.Name.ValueString(),
		Pools: poolList,
	})
	if err != nil {
		resp.Diagnostics.AddError("API error", err.Error())
		return
	}
	plan.Id = types.StringValue(out.ID.String())
	plan.Name = types.StringValue(out.Name)
	if len(out.PoolIDs) > 0 {
		poolIdVals := make([]types.String, 0, len(out.PoolIDs))
		for _, id := range out.PoolIDs {
			poolIdVals = append(poolIdVals, types.StringValue(id.String()))
		}
		plan.PoolIds, _ = types.ListValueFrom(ctx, types.StringType, poolIdVals)
	}
	tflog.Trace(ctx, "created ipam_environment", map[string]interface{}{"id": out.ID.String()})
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *EnvironmentResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state EnvironmentResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	id, diags := parseID("environment id", state.Id.ValueString())
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	out, err := r.api.GetEnvironment(ctx, id)
	if err != nil {
		resp.Diagnostics.AddError("API error", err.Error())
		return
	}
	state.Id = types.StringValue(out.ID.String())
	state.Name = types.StringValue(out.Name)
	pools, err := r.api.ListPools(ctx, &ipam.ListPoolsOptions{EnvironmentID: id})
	if err == nil && len(pools) > 0 {
		objType := types.ObjectType{AttrTypes: map[string]attr.Type{
			"name": types.StringType,
			"cidr": types.StringType,
		}}
		elems := make([]attr.Value, 0, len(pools))
		poolIdVals := make([]types.String, 0, len(pools))
		for _, p := range pools {
			obj, _ := types.ObjectValue(objType.AttrTypes, map[string]attr.Value{
				"name": types.StringValue(p.Name),
				"cidr": types.StringValue(p.CIDR),
			})
			elems = append(elems, obj)
			poolIdVals = append(poolIdVals, types.StringValue(p.ID.String()))
		}
		state.Pools = types.ListValueMust(objType, elems)
		state.PoolIds, _ = types.ListValueFrom(ctx, types.StringType, poolIdVals)
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *EnvironmentResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state EnvironmentResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	id, diags := parseID("environment id", plan.Id.ValueString())
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	out, err := r.api.UpdateEnvironment(ctx, id, plan.Name.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("API error", err.Error())
		return
	}
	// Preserve computed pool_ids and pools from state so they remain known after apply.
	plan.Id = types.StringValue(out.ID.String())
	plan.Name = types.StringValue(out.Name)
	if !state.PoolIds.IsNull() && !state.PoolIds.IsUnknown() {
		plan.PoolIds = state.PoolIds
	}
	if !state.Pools.IsNull() && !state.Pools.IsUnknown() {
		plan.Pools = state.Pools
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *EnvironmentResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state EnvironmentResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	id, diags := parseID("environment id", state.Id.ValueString())
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.api.DeleteEnvironment(ctx, id); err != nil {
		resp.Diagnostics.AddError("API error", err.Error())
	}
}

func (r *EnvironmentResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
