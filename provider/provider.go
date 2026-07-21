package provider

import (
	"context"
	"os"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	atlasapi "github.com/supabase/atlasctl/pkg/atlasapi"
	"github.com/supabase/atlasctl/pkg/plan"
)

var _ provider.Provider = &ripeAtlasProvider{}

type ripeAtlasProvider struct{}

// providerClients holds API clients and shared config passed to each resource.
type providerClients struct {
	apply    *atlasapi.ApplyClient
	msm      *atlasapi.MsmClient
	snapshot string
}

func New() provider.Provider {
	return &ripeAtlasProvider{}
}

func (p *ripeAtlasProvider) Metadata(_ context.Context, _ provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "ripeatlas"
}

func (p *ripeAtlasProvider) Schema(_ context.Context, _ provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			"api_key": schema.StringAttribute{
				Optional:  true,
				Sensitive: true,
			},
			"snapshot": schema.StringAttribute{
				Optional:    true,
				Description: "Path to the probe snapshot JSON file. Falls back to RIPE_ATLAS_SNAPSHOT env var.",
			},
		},
	}
}

type providerModel struct {
	APIKey   types.String `tfsdk:"api_key"`
	Snapshot types.String `tfsdk:"snapshot"`
}

func (p *ripeAtlasProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var config providerModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	apiKey := config.APIKey.ValueString()
	if apiKey == "" {
		apiKey = os.Getenv("RIPE_ATLAS_API_KEY")
	}
	if apiKey == "" {
		resp.Diagnostics.AddError("Missing API key",
			"Set api_key in provider config or RIPE_ATLAS_API_KEY environment variable.")
		return
	}

	snapshotPath := config.Snapshot.ValueString()
	if snapshotPath == "" {
		snapshotPath = os.Getenv("RIPE_ATLAS_SNAPSHOT")
	}
	if snapshotPath == "" {
		resp.Diagnostics.AddError("Missing snapshot",
			"Set snapshot in provider config or RIPE_ATLAS_SNAPSHOT environment variable.")
		return
	}

	codec := plan.NewTagCodec(plan.DefaultTagPrefix)
	verbose := os.Getenv("RIPE_ATLAS_DEBUG") != ""

	applyClient, err := atlasapi.NewApplyClient(apiKey, verbose, codec)
	if err != nil {
		resp.Diagnostics.AddError("Failed to create RIPE Atlas apply client", err.Error())
		return
	}

	msmClient, err := atlasapi.NewMsmClient(apiKey, verbose, codec)
	if err != nil {
		resp.Diagnostics.AddError("Failed to create RIPE Atlas measurement client", err.Error())
		return
	}

	clients := &providerClients{
		apply:    applyClient,
		msm:      msmClient,
		snapshot: snapshotPath,
	}
	resp.ResourceData = clients
}

func (p *ripeAtlasProvider) Resources(_ context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		NewMeasurementResource,
	}
}

func (p *ripeAtlasProvider) DataSources(_ context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{}
}
