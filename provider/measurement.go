package provider

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	atlasapi "github.com/supabase/atlasctl/pkg/atlasapi"
	"github.com/supabase/atlasctl/pkg/config"
	"github.com/supabase/atlasctl/pkg/plan"
	"github.com/supabase/atlasctl/pkg/selection"
	"github.com/supabase/atlasctl/pkg/snapshot"
)

const h3Resolution = 3

var _ resource.Resource = &measurementResource{}
var _ resource.ResourceWithModifyPlan = &measurementResource{}

type measurementResource struct {
	clients *providerClients
}

func NewMeasurementResource() resource.Resource {
	return &measurementResource{}
}

func (r *measurementResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_measurement"
}

func (r *measurementResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			"name": schema.StringAttribute{
				Required: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"target": schema.StringAttribute{
				Required: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"msm_type": schema.StringAttribute{
				Required: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"af": schema.Int64Attribute{
				Optional: true,
				Computed: true,
				Default:  int64default.StaticInt64(4),
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.RequiresReplace(),
				},
			},
			"snapshot": schema.StringAttribute{
				Required: true,
			},
			"exclude_tags": schema.ListAttribute{
				Optional:    true,
				ElementType: types.StringType,
			},
			"msm_id": schema.Int64Attribute{
				Computed: true,
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.UseStateForUnknown(),
				},
			},
			"probe_ids": schema.SetAttribute{
				Computed:    true,
				ElementType: types.Int64Type,
			},
			"cohort": schema.SingleNestedAttribute{
				Required: true,
				Attributes: map[string]schema.Attribute{
					"name": schema.StringAttribute{
						Required: true,
						PlanModifiers: []planmodifier.String{
							stringplanmodifier.RequiresReplace(),
						},
					},
					"probe_count": schema.Int64Attribute{
						Required: true,
					},
					"max_probes_per_cell": schema.Int64Attribute{
						Required: true,
					},
					"interval_seconds": schema.Int64Attribute{
						Required: true,
						PlanModifiers: []planmodifier.Int64{
							int64planmodifier.RequiresReplace(),
						},
					},
					"include_probe_ids": schema.SetAttribute{
						Optional:    true,
						ElementType: types.Int64Type,
					},
					"exclude_probe_ids": schema.SetAttribute{
						Optional:    true,
						ElementType: types.Int64Type,
					},
					"cfg": schema.SingleNestedAttribute{
						Optional: true,
						Attributes: map[string]schema.Attribute{
							"asn": schema.MapAttribute{
								Optional:    true,
								ElementType: types.Int64Type,
							},
							"tags": schema.MapAttribute{
								Optional:    true,
								ElementType: types.Int64Type,
							},
							"countries": schema.MapAttribute{
								Optional:    true,
								ElementType: types.Int64Type,
							},
							"stability": schema.MapAttribute{
								Optional:    true,
								ElementType: types.Int64Type,
							},
						},
					},
				},
			},
		},
	}
}

type cfgModel struct {
	ASN       types.Map `tfsdk:"asn"`
	Tags      types.Map `tfsdk:"tags"`
	Countries types.Map `tfsdk:"countries"`
	Stability types.Map `tfsdk:"stability"`
}

type cohortModel struct {
	Name             types.String `tfsdk:"name"`
	ProbeCount       types.Int64  `tfsdk:"probe_count"`
	MaxProbesPerCell types.Int64  `tfsdk:"max_probes_per_cell"`
	IntervalSeconds  types.Int64  `tfsdk:"interval_seconds"`
	IncludeProbeIDs  types.Set    `tfsdk:"include_probe_ids"`
	ExcludeProbeIDs  types.Set    `tfsdk:"exclude_probe_ids"`
	Cfg              *cfgModel    `tfsdk:"cfg"`
}

type measurementModel struct {
	Name        types.String `tfsdk:"name"`
	Target      types.String `tfsdk:"target"`
	MsmType     types.String `tfsdk:"msm_type"`
	AF          types.Int64  `tfsdk:"af"`
	Snapshot    types.String `tfsdk:"snapshot"`
	ExcludeTags types.List   `tfsdk:"exclude_tags"`
	Cohort      *cohortModel `tfsdk:"cohort"`
	MsmID       types.Int64  `tfsdk:"msm_id"`
	ProbeIDs    types.Set    `tfsdk:"probe_ids"`
}

func (r *measurementResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	clients, ok := req.ProviderData.(*providerClients)
	if !ok {
		resp.Diagnostics.AddError("Unexpected provider data type", "Expected *providerClients")
		return
	}
	r.clients = clients
}

// ModifyPlan validates config and runs probe selection at plan time so probe_ids
// is known before apply.
func (r *measurementResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if req.Plan.Raw.IsNull() {
		// Destroy plan: nothing to compute.
		return
	}

	var planData measurementModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &planData)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if planData.Cohort == nil {
		resp.Diagnostics.AddError("Missing cohort", "A cohort = { } attribute is required.")
		return
	}

	resp.Diagnostics.Append(validateMeasurement(planData)...)
	if resp.Diagnostics.HasError() {
		return
	}

	ids, err := runSelection(ctx, planData)
	if err != nil {
		// Best-effort: leave probe_ids unknown if selection cannot run at plan time.
		// The error will surface during apply.
		return
	}

	// Dry-run the goat spec builder to catch structural errors (wrong af, empty
	// target, invalid interval, etc.) before they reach the API at apply time.
	spec := plan.MsmSpec{
		Key:      plan.MsmKey{Name: planData.Name.ValueString(), Cohort: planData.Cohort.Name.ValueString()},
		Target:   planData.Target.ValueString(),
		Type:     plan.MsmType(planData.MsmType.ValueString()),
		AF:       int(planData.AF.ValueInt64()),
		Interval: int(planData.Cohort.IntervalSeconds.ValueInt64()),
		ProbeIDs: ids,
	}
	if err := atlasapi.ValidateMsmSpec(spec); err != nil {
		resp.Diagnostics.AddError("Invalid measurement spec", err.Error())
		return
	}

	planData.ProbeIDs = probeIDsToSet(ids)
	resp.Diagnostics.Append(resp.Plan.Set(ctx, &planData)...)
}

func (r *measurementResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var state measurementModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if state.Cohort == nil {
		resp.Diagnostics.AddError("Missing cohort block", "A cohort { } block is required.")
		return
	}

	ids, err := runSelection(ctx, state)
	if err != nil {
		resp.Diagnostics.AddError("Failed to select probes", err.Error())
		return
	}

	spec := plan.MsmSpec{
		Key:      plan.MsmKey{Name: state.Name.ValueString(), Cohort: state.Cohort.Name.ValueString()},
		Target:   state.Target.ValueString(),
		Type:     plan.MsmType(state.MsmType.ValueString()),
		AF:       int(state.AF.ValueInt64()),
		Interval: int(state.Cohort.IntervalSeconds.ValueInt64()),
		ProbeIDs: ids,
	}

	msmID, err := r.clients.apply.CreateMeasurement(ctx, spec)
	if err != nil {
		resp.Diagnostics.AddError("Failed to create measurement", err.Error())
		return
	}

	state.MsmID = types.Int64Value(int64(msmID))
	state.ProbeIDs = probeIDsToSet(ids)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *measurementResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state measurementModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	info, err := r.clients.msm.GetMeasurement(ctx, uint64(state.MsmID.ValueInt64()))
	if err != nil {
		if errors.Is(err, plan.ErrMsmNotFound) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Failed to read measurement", err.Error())
		return
	}

	state.ProbeIDs = probeIDsToSet(info.ProbeIDs)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *measurementResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var state, planData measurementModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	resp.Diagnostics.Append(req.Plan.Get(ctx, &planData)...)
	if resp.Diagnostics.HasError() {
		return
	}

	ids, err := runSelection(ctx, planData)
	if err != nil {
		resp.Diagnostics.AddError("Failed to select probes", err.Error())
		return
	}

	msmID := uint64(state.MsmID.ValueInt64())

	oldIDs := make(map[uint32]struct{})
	for _, e := range state.ProbeIDs.Elements() {
		oldIDs[uint32(e.(types.Int64).ValueInt64())] = struct{}{}
	}
	newIDs := make(map[uint32]struct{})
	for _, id := range ids {
		newIDs[id] = struct{}{}
	}

	var added, removed []uint32
	for id := range newIDs {
		if _, ok := oldIDs[id]; !ok {
			added = append(added, id)
		}
	}
	for id := range oldIDs {
		if _, ok := newIDs[id]; !ok {
			removed = append(removed, id)
		}
	}

	if len(added) > 0 {
		if err := r.clients.apply.AddParticipants(ctx, msmID, added); err != nil {
			resp.Diagnostics.AddError("Failed to add participants", err.Error())
			return
		}
	}
	if len(removed) > 0 {
		if err := r.clients.apply.RemoveParticipants(ctx, msmID, removed); err != nil {
			resp.Diagnostics.AddError("Failed to remove participants", err.Error())
			return
		}
	}

	planData.MsmID = state.MsmID
	planData.ProbeIDs = probeIDsToSet(ids)
	resp.Diagnostics.Append(resp.State.Set(ctx, &planData)...)
}

func (r *measurementResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state measurementModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.clients.apply.StopMeasurement(ctx, uint64(state.MsmID.ValueInt64())); err != nil {
		resp.Diagnostics.AddError("Failed to stop measurement", err.Error())
	}
}

var validMsmTypes = map[string]bool{
	"dns": true, "ping": true, "tls": true, "traceroute": true,
}

// validateMeasurement returns diagnostics for config values that would cause
// cryptic API errors at apply time.
func validateMeasurement(m measurementModel) diag.Diagnostics {
	var diags diag.Diagnostics

	if !validMsmTypes[m.MsmType.ValueString()] {
		diags.AddAttributeError(
			path.Root("msm_type"),
			"Invalid msm_type",
			fmt.Sprintf("msm_type must be one of: dns, ping, tls, traceroute. Got %q.", m.MsmType.ValueString()),
		)
	}

	af := m.AF.ValueInt64()
	if af != 4 && af != 6 {
		diags.AddAttributeError(
			path.Root("af"),
			"Invalid af",
			fmt.Sprintf("af must be 4 or 6. Got %d.", af),
		)
	}

	if m.Target.ValueString() == "" {
		diags.AddAttributeError(
			path.Root("target"),
			"Empty target",
			"target must not be empty.",
		)
	}

	if m.Cohort != nil {
		interval := m.Cohort.IntervalSeconds.ValueInt64()
		if interval < 60 {
			diags.AddAttributeError(
				path.Root("cohort").AtName("interval_seconds"),
				"Invalid interval_seconds",
				fmt.Sprintf("interval_seconds must be at least 60. Got %d.", interval),
			)
		}
		if m.Cohort.ProbeCount.ValueInt64() <= 0 {
			diags.AddAttributeError(
				path.Root("cohort").AtName("probe_count"),
				"Invalid probe_count",
				fmt.Sprintf("probe_count must be positive. Got %d.", m.Cohort.ProbeCount.ValueInt64()),
			)
		}
		if m.Cohort.MaxProbesPerCell.ValueInt64() <= 0 {
			diags.AddAttributeError(
				path.Root("cohort").AtName("max_probes_per_cell"),
				"Invalid max_probes_per_cell",
				fmt.Sprintf("max_probes_per_cell must be positive. Got %d.", m.Cohort.MaxProbesPerCell.ValueInt64()),
			)
		}
	}

	return diags
}

// runSelection loads the snapshot, applies exclude_tags hard-exclusion, then runs
// probe selection for the cohort block, returning the selected probe IDs.
func runSelection(ctx context.Context, m measurementModel) ([]uint32, error) {
	snap, err := snapshot.Load(m.Snapshot.ValueString())
	if err != nil {
		return nil, fmt.Errorf("loading snapshot: %w", err)
	}

	var excludeTags []string
	if !m.ExcludeTags.IsNull() && !m.ExcludeTags.IsUnknown() {
		for _, v := range m.ExcludeTags.Elements() {
			excludeTags = append(excludeTags, v.(types.String).ValueString())
		}
	}

	probes := selection.NewProbes(len(snap.Probes))
	for _, p := range snap.Probes {
		if !selection.HardExcluded(p, excludeTags) {
			probes.Append(p)
		}
	}
	probes.Close()

	cohort := buildMeasurementCohort(*m.Cohort)
	orderer := selection.NewDefaultOrderer(h3Resolution)
	selected, err := selection.Select(ctx, probes, []config.MeasurementCohort{cohort}, orderer, h3Resolution)
	if err != nil {
		return nil, fmt.Errorf("selecting probes: %w", err)
	}
	if len(selected) == 0 {
		return nil, fmt.Errorf("selection returned no cohorts")
	}

	ids := make([]uint32, len(selected[0].Probes))
	for i, p := range selected[0].Probes {
		ids[i] = p.ID
	}
	return ids, nil
}

func buildMeasurementCohort(c cohortModel) config.MeasurementCohort {
	cohort := config.MeasurementCohort{
		Name:             c.Name.ValueString(),
		ProbeCount:       int(c.ProbeCount.ValueInt64()),
		MaxProbesPerCell: int(c.MaxProbesPerCell.ValueInt64()),
		IntervalSeconds:  int(c.IntervalSeconds.ValueInt64()),
	}

	if !c.IncludeProbeIDs.IsNull() && !c.IncludeProbeIDs.IsUnknown() {
		for _, v := range c.IncludeProbeIDs.Elements() {
			cohort.IncludeProbeIDs = append(cohort.IncludeProbeIDs, uint32(v.(types.Int64).ValueInt64()))
		}
	}

	if !c.ExcludeProbeIDs.IsNull() && !c.ExcludeProbeIDs.IsUnknown() {
		for _, v := range c.ExcludeProbeIDs.Elements() {
			cohort.ExcludeProbeIDs = append(cohort.ExcludeProbeIDs, uint32(v.(types.Int64).ValueInt64()))
		}
	}

	if c.Cfg != nil {
		cohort.Cfg = buildCohortCfg(*c.Cfg)
	}

	return cohort
}

func buildCohortCfg(c cfgModel) config.CohortCfg {
	var cfg config.CohortCfg

	if !c.ASN.IsNull() && !c.ASN.IsUnknown() {
		cfg.ASN = make(map[uint32]int, len(c.ASN.Elements()))
		for k, v := range c.ASN.Elements() {
			n, _ := strconv.ParseUint(k, 10, 32)
			cfg.ASN[uint32(n)] = int(v.(types.Int64).ValueInt64())
		}
	}

	if !c.Tags.IsNull() && !c.Tags.IsUnknown() {
		cfg.Tags = make(map[string]int, len(c.Tags.Elements()))
		for k, v := range c.Tags.Elements() {
			cfg.Tags[k] = int(v.(types.Int64).ValueInt64())
		}
	}

	if !c.Countries.IsNull() && !c.Countries.IsUnknown() {
		cfg.Countries = make(map[string]int, len(c.Countries.Elements()))
		for k, v := range c.Countries.Elements() {
			cfg.Countries[k] = int(v.(types.Int64).ValueInt64())
		}
	}

	if !c.Stability.IsNull() && !c.Stability.IsUnknown() {
		cfg.Stability = make(map[string]int, len(c.Stability.Elements()))
		for k, v := range c.Stability.Elements() {
			cfg.Stability[k] = int(v.(types.Int64).ValueInt64())
		}
	}

	return cfg
}

func probeIDsToSet(ids []uint32) types.Set {
	elems := make([]attr.Value, len(ids))
	for i, id := range ids {
		elems[i] = types.Int64Value(int64(id))
	}
	return types.SetValueMust(types.Int64Type, elems)
}
