package provider

import (
	"context"
	"errors"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	atlasapi "github.com/supabase/atlasctl/pkg/atlasapi"
	"github.com/supabase/atlasctl/pkg/plan"
)

var _ resource.Resource = &measurementResource{}

type measurementResource struct {
	client *atlasapi.ApplyClient
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
			"cohort": schema.StringAttribute{
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
			"interval_seconds": schema.Int64Attribute{
				Required: true,
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.RequiresReplace(),
				},
			},
			"probe_ids": schema.SetAttribute{
				Required:    true,
				ElementType: types.Int64Type,
			},
			"msm_id": schema.Int64Attribute{
				Computed: true,
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.UseStateForUnknown(),
				},
			},
		},
	}
}

func (r *measurementResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	client, ok := req.ProviderData.(*atlasapi.ApplyClient)
	if !ok {
		resp.Diagnostics.AddError("Unexpected provider data type", "Expected *atlasapi.ApplyClient")
		return
	}
	r.client = client
}

type measurementModel struct {
	Name            types.String `tfsdk:"name"`
	Cohort          types.String `tfsdk:"cohort"`
	Target          types.String `tfsdk:"target"`
	MsmType         types.String `tfsdk:"msm_type"`
	AF              types.Int64  `tfsdk:"af"`
	IntervalSeconds types.Int64  `tfsdk:"interval_seconds"`
	ProbeIDs        types.Set    `tfsdk:"probe_ids"`
	MsmID           types.Int64  `tfsdk:"msm_id"`
}

func probeIDsFromSet(s types.Set) []uint32 {
	elems := s.Elements()
	ids := make([]uint32, len(elems))
	for i, e := range elems {
		ids[i] = uint32(e.(types.Int64).ValueInt64())
	}
	return ids
}

func probeIDsToSet(ids []uint32) types.Set {
	elems := make([]attr.Value, len(ids))
	for i, id := range ids {
		elems[i] = types.Int64Value(int64(id))
	}
	return types.SetValueMust(types.Int64Type, elems)
}

func (r *measurementResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var state measurementModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	spec := plan.MsmSpec{
		Key:      plan.MsmKey{Name: state.Name.ValueString(), Cohort: state.Cohort.ValueString()},
		Target:   state.Target.ValueString(),
		Type:     plan.MsmType(state.MsmType.ValueString()),
		AF:       int(state.AF.ValueInt64()),
		Interval: int(state.IntervalSeconds.ValueInt64()),
		ProbeIDs: probeIDsFromSet(state.ProbeIDs),
	}

	msmID, err := r.client.CreateMeasurement(ctx, spec)
	if err != nil {
		resp.Diagnostics.AddError("Failed to create measurement", err.Error())
		return
	}

	state.MsmID = types.Int64Value(int64(msmID))
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *measurementResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state measurementModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	info, err := r.client.GetMeasurement(ctx, uint64(state.MsmID.ValueInt64()))
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
	var state, plan measurementModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	msmID := uint64(state.MsmID.ValueInt64())

	oldIDs := make(map[uint32]struct{})
	for _, e := range state.ProbeIDs.Elements() {
		oldIDs[uint32(e.(types.Int64).ValueInt64())] = struct{}{}
	}
	newIDs := make(map[uint32]struct{})
	for _, e := range plan.ProbeIDs.Elements() {
		newIDs[uint32(e.(types.Int64).ValueInt64())] = struct{}{}
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
		if err := r.client.AddParticipants(ctx, msmID, added); err != nil {
			resp.Diagnostics.AddError("Failed to add participants", err.Error())
			return
		}
	}
	if len(removed) > 0 {
		if err := r.client.RemoveParticipants(ctx, msmID, removed); err != nil {
			resp.Diagnostics.AddError("Failed to remove participants", err.Error())
			return
		}
	}

	plan.MsmID = state.MsmID
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *measurementResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state measurementModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.client.StopMeasurement(ctx, uint64(state.MsmID.ValueInt64())); err != nil {
		resp.Diagnostics.AddError("Failed to stop measurement", err.Error())
	}
}
