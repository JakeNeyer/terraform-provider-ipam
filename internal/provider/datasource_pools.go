package provider

import (
	"context"
	"fmt"

	"github.com/JakeNeyer/ipam-go/ipam"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ datasource.DataSource = &PoolsDataSource{}

func NewPoolsDataSource() datasource.DataSource {
	return &PoolsDataSource{}
}

type PoolsDataSource struct {
	api *ipam.Client
}

type PoolsDataSourceModel struct {
	EnvironmentId types.String   `tfsdk:"environment_id"`
	Pools         []PoolRefModel `tfsdk:"pools"`
}

type PoolRefModel struct {
	Id            types.String `tfsdk:"id"`
	EnvironmentId types.String `tfsdk:"environment_id"`
	Name          types.String `tfsdk:"name"`
	Cidr          types.String `tfsdk:"cidr"`
}

func (d *PoolsDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pools"
}

func (d *PoolsDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "List pools for an environment.",
		Attributes: map[string]schema.Attribute{
			"environment_id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Environment UUID.",
			},
			"pools": schema.ListNestedAttribute{
				Computed:            true,
				MarkdownDescription: "List of pools in the environment.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "Pool UUID.",
						},
						"environment_id": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "Environment UUID.",
						},
						"name": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "Pool name.",
						},
						"cidr": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "Pool CIDR range.",
						},
					},
				},
			},
		},
	}
}

func (d *PoolsDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	api, ok := req.ProviderData.(*ipam.Client)
	if !ok {
		resp.Diagnostics.AddError("Unexpected provider type", fmt.Sprintf("Expected *ipam.Client, got %T", req.ProviderData))
		return
	}
	d.api = api
}

func (d *PoolsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config PoolsDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	envID, diags := parseID("environment_id", config.EnvironmentId.ValueString())
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	out, err := d.api.ListPools(ctx, &ipam.ListPoolsOptions{EnvironmentID: envID})
	if err != nil {
		resp.Diagnostics.AddError("API error", err.Error())
		return
	}
	config.Pools = make([]PoolRefModel, len(out))
	for i, p := range out {
		config.Pools[i] = PoolRefModel{
			Id:            types.StringValue(p.ID.String()),
			EnvironmentId: types.StringValue(idString(p.EnvironmentID)),
			Name:          types.StringValue(p.Name),
			Cidr:          types.StringValue(p.CIDR),
		}
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}
