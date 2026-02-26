package provider

import (
	"context"
	"fmt"
	"strings"

	"github.com/JakeNeyer/terraform-provider-ipam/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ datasource.DataSource = &AllocationDataSource{}

func NewAllocationDataSource() datasource.DataSource {
	return &AllocationDataSource{}
}

type AllocationDataSource struct {
	api *client.Client
}

type AllocationDataSourceModel struct {
	Id        types.String `tfsdk:"id"`
	Name      types.String `tfsdk:"name"`
	BlockName types.String `tfsdk:"block_name"`
	Cidr      types.String `tfsdk:"cidr"`
}

func (d *AllocationDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_allocation"
}

func (d *AllocationDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Fetch a single allocation by ID or by block name and allocation name.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Allocation UUID. Provide either `id` or both `block_name` and `name`.",
			},
			"name": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Allocation name. Use with `block_name` when `id` is not set or when the API does not support GET by id.",
			},
			"block_name": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Parent block name. Use with `name` when `id` is not set or when the API does not support GET by id.",
			},
			"cidr": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "CIDR range.",
			},
		},
	}
}

func (d *AllocationDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	api, ok := req.ProviderData.(*client.Client)
	if !ok {
		resp.Diagnostics.AddError("Unexpected provider type", fmt.Sprintf("Expected *client.Client, got %T", req.ProviderData))
		return
	}
	d.api = api
}

func (d *AllocationDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config AllocationDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	idSet := !config.Id.IsNull() && config.Id.ValueString() != ""
	nameSet := !config.Name.IsNull() && config.Name.ValueString() != ""
	blockSet := !config.BlockName.IsNull() && config.BlockName.ValueString() != ""
	if !idSet && !(nameSet && blockSet) {
		resp.Diagnostics.AddError("Invalid configuration", "Provide either `id` or both `block_name` and `name`.")
		return
	}
	var out *client.AllocationResponse
	if idSet {
		var err error
		out, err = d.api.GetAllocation(strings.ToLower(config.Id.ValueString()))
		if err != nil {
			if strings.Contains(strings.ToLower(err.Error()), "not found") && nameSet && blockSet {
				list, listErr := d.api.ListAllocations(config.Name.ValueString(), config.BlockName.ValueString(), 0, 0)
				if listErr != nil {
					resp.Diagnostics.AddError("API error", listErr.Error())
					return
				}
				if len(list.Allocations) != 1 {
					resp.Diagnostics.AddError("API error", err.Error())
					return
				}
				out = &list.Allocations[0]
			} else {
				resp.Diagnostics.AddError("API error", err.Error())
				return
			}
		}
	} else {
		list, err := d.api.ListAllocations(config.Name.ValueString(), config.BlockName.ValueString(), 0, 0)
		if err != nil {
			resp.Diagnostics.AddError("API error", err.Error())
			return
		}
		if len(list.Allocations) != 1 {
			resp.Diagnostics.AddError("No allocation found", "List by block_name and name did not return exactly one allocation.")
			return
		}
		out = &list.Allocations[0]
	}
	config.Id = types.StringValue(strings.ToLower(out.Id))
	config.Name = types.StringValue(out.Name)
	config.BlockName = types.StringValue(out.BlockName)
	config.Cidr = types.StringValue(out.CIDR)
	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}
