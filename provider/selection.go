package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/supabase/atlasctl/pkg/config"
	"github.com/supabase/atlasctl/pkg/selection"
	"github.com/supabase/atlasctl/pkg/snapshot"
)

var _ datasource.DataSource = &probeSelectionDataSource{}

type probeSelectionDataSource struct{}

func NewProbeSelectionDataSource() datasource.DataSource {
	return &probeSelectionDataSource{}
}

func (d *probeSelectionDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_probe_selection"
}

func (d *probeSelectionDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			"snapshot": schema.StringAttribute{
				Required: true,
			},
			"config": schema.StringAttribute{
				Required: true,
			},
			"probe_ids": schema.MapAttribute{
				Computed:    true,
				ElementType: types.SetType{ElemType: types.Int64Type},
			},
		},
	}
}

type probeSelectionModel struct {
	Snapshot types.String `tfsdk:"snapshot"`
	Config   types.String `tfsdk:"config"`
	ProbeIDs types.Map    `tfsdk:"probe_ids"`
}

func (d *probeSelectionDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var state probeSelectionModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	cfg, err := config.Load(state.Config.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Failed to load config", err.Error())
		return
	}

	snap, err := snapshot.Load(state.Snapshot.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Failed to load snapshot", err.Error())
		return
	}

	cohorts, err := selection.Select(ctx, snap, *cfg)
	if err != nil {
		resp.Diagnostics.AddError("Failed to select probes", err.Error())
		return
	}

	probeIDs := make(map[string]attr.Value, len(cohorts))
	for _, c := range cohorts {
		ids := make([]attr.Value, len(c.Probes))
		for i, probe := range c.Probes {
			ids[i] = types.Int64Value(int64(probe.ID))
		}
		probeIDs[c.Cohort.Name] = types.SetValueMust(types.Int64Type, ids)
	}

	result, diags := types.MapValue(types.SetType{ElemType: types.Int64Type}, probeIDs)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	state.ProbeIDs = result
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}
