package provider

import (
	"context"
	"fmt"
	"strings"

	"github.com/JakeNeyer/ipam-go/ipam"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

var _ resource.Resource = &AllocationResource{}
var _ resource.ResourceWithImportState = &AllocationResource{}

func NewAllocationResource() resource.Resource {
	return &AllocationResource{}
}

type AllocationResource struct {
	api *ipam.Client
}

type AllocationResourceModel struct {
	Id           types.String `tfsdk:"id"`
	Name         types.String `tfsdk:"name"`
	BlockName    types.String `tfsdk:"block_name"`
	Cidr         types.String `tfsdk:"cidr"`
	PrefixLength types.Int64  `tfsdk:"prefix_length"`
}

func (r *AllocationResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_allocation"
}

func (r *AllocationResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: `IPAM allocation. An allocation is a subnet within a network block (e.g. a VPC or region).

Provide either **cidr** (explicit) or **prefix_length** (auto-allocate the next available CIDR in the block using bin-packing).`,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Allocation UUID.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"name": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Allocation name.",
			},
			"block_name": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Name of the parent network block.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"cidr": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "CIDR for this allocation. If omitted, set `prefix_length` to auto-allocate the next available CIDR.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace(), stringplanmodifier.UseStateForUnknown()},
			},
			"prefix_length": schema.Int64Attribute{
				Optional:            true,
				MarkdownDescription: "Desired prefix length (e.g. 24 for /24). When set without `cidr`, the API finds the next available CIDR in the block using bin-packing.",
				PlanModifiers:       []planmodifier.Int64{int64planmodifier.RequiresReplace()},
			},
		},
	}
}

func (r *AllocationResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *AllocationResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan AllocationResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	name := plan.Name.ValueString()
	blockName := plan.BlockName.ValueString()
	hasCidr := !plan.Cidr.IsNull() && !plan.Cidr.IsUnknown()
	hasPrefix := !plan.PrefixLength.IsNull() && !plan.PrefixLength.IsUnknown()

	if !hasCidr && !hasPrefix {
		resp.Diagnostics.AddError("Missing required attribute", "Either cidr or prefix_length must be specified.")
		return
	}
	if hasCidr && hasPrefix {
		resp.Diagnostics.AddError("Conflicting attributes", "Specify either cidr or prefix_length, not both.")
		return
	}

	var out *ipam.Allocation
	var err error

	if hasPrefix {
		out, err = r.api.AutoAllocate(ctx, ipam.AutoAllocateInput{
			Name:         name,
			BlockName:    blockName,
			PrefixLength: int(plan.PrefixLength.ValueInt64()),
		})
	} else {
		out, err = r.api.CreateAllocation(ctx, ipam.CreateAllocationInput{
			Name:      name,
			BlockName: blockName,
			CIDR:      plan.Cidr.ValueString(),
		})
	}

	if err != nil {
		resp.Diagnostics.AddError("API error", err.Error())
		return
	}
	plan.Id = types.StringValue(strings.ToLower(out.ID.String()))
	plan.Name = types.StringValue(out.Name)
	plan.BlockName = types.StringValue(out.BlockName)
	plan.Cidr = types.StringValue(out.CIDR)
	tflog.Trace(ctx, "created ipam_allocation", map[string]interface{}{"id": plan.Id.ValueString(), "cidr": out.CIDR})
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *AllocationResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state AllocationResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	id, diags := parseID("allocation id", state.Id.ValueString())
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	out, err := r.api.GetAllocation(ctx, id)
	if err != nil {
		if !ipam.IsNotFound(err) {
			resp.Diagnostics.AddError("API error", err.Error())
			return
		}
		// Fallback: some IPAM APIs do not support GET /api/allocations/{id}; find by block_name + name.
		list, listErr := r.api.ListAllocations(ctx, &ipam.ListAllocationsOptions{
			ListOptions: ipam.ListOptions{Name: state.Name.ValueString()},
			BlockName:   state.BlockName.ValueString(),
		})
		if listErr != nil {
			resp.Diagnostics.AddError("API error", listErr.Error())
			return
		}
		switch n := len(list); n {
		case 0:
			resp.State.RemoveResource(ctx)
			return
		case 1:
			out = &list[0]
		default:
			for i := range list {
				if strings.EqualFold(list[i].ID.String(), state.Id.ValueString()) {
					out = &list[i]
					break
				}
			}
			if out == nil {
				out = &list[0]
			}
		}
	}
	state.Id = types.StringValue(strings.ToLower(out.ID.String()))
	state.Name = types.StringValue(out.Name)
	state.BlockName = types.StringValue(out.BlockName)
	state.Cidr = types.StringValue(out.CIDR)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *AllocationResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan AllocationResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var state AllocationResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	id, diags := parseID("allocation id", plan.Id.ValueString())
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	out, err := r.api.UpdateAllocation(ctx, id, plan.Name.ValueString())
	if err != nil && ipam.IsNotFound(err) {
		// Fallback: resolve allocation by block_name + prior name (current name on server before update).
		list, listErr := r.api.ListAllocations(ctx, &ipam.ListAllocationsOptions{
			ListOptions: ipam.ListOptions{Name: state.Name.ValueString()},
			BlockName:   plan.BlockName.ValueString(),
		})
		if listErr != nil {
			resp.Diagnostics.AddError("API error", listErr.Error())
			return
		}
		if len(list) != 1 {
			resp.Diagnostics.AddError("API error", err.Error())
			return
		}
		id = list[0].ID
		out, err = r.api.UpdateAllocation(ctx, id, plan.Name.ValueString())
	}
	if err != nil {
		resp.Diagnostics.AddError("API error", err.Error())
		return
	}
	plan.Id = types.StringValue(strings.ToLower(out.ID.String()))
	plan.Name = types.StringValue(out.Name)
	plan.BlockName = types.StringValue(out.BlockName)
	plan.Cidr = types.StringValue(out.CIDR)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *AllocationResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state AllocationResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	id, diags := parseID("allocation id", state.Id.ValueString())
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.api.DeleteAllocation(ctx, id); err != nil {
		if ipam.IsNotFound(err) {
			return
		}
		resp.Diagnostics.AddError("API error", err.Error())
	}
}

func (r *AllocationResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
