package provider

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	atlasapi "github.com/supabase/atlasctl/pkg/atlasapi"
	"github.com/supabase/atlasctl/pkg/plan"
	"github.com/supabase/atlasctl/pkg/snapshot"
)

var _ provider.Provider = &ripeAtlasProvider{}

type ripeAtlasProvider struct{}

// providerClients holds API clients and shared config passed to each resource.
type providerClients struct {
	apply       *atlasapi.ApplyClient
	msm         *atlasapi.MsmClient
	probeSource snapshot.ProbeSource
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
				Description: "Path to a pre-fetched probe snapshot JSON file. When set, probes are read directly from this file with no freshness check. Falls back to RIPE_ATLAS_SNAPSHOT env var. Mutually exclusive with snapshot_cache_path.",
			},
			"snapshot_cache_path": schema.StringAttribute{
				Optional:    true,
				Description: "Path for the auto-managed probe cache file. When snapshot is not set, probes are served from this cache and refreshed from the RIPE Atlas API when stale. Defaults to " + snapshot.DefaultCachePath + ".",
			},
			"snapshot_ttl": schema.StringAttribute{
				Optional:    true,
				Description: "Maximum age of the cached probe list before it is refreshed from the RIPE Atlas API. Go duration string (e.g. \"2h\", \"30m\"). Only applies when snapshot is not set. Defaults to " + snapshot.DefaultCacheTTL.String() + ".",
			},
			"namespace": schema.StringAttribute{
				Optional:    true,
				Description: "Namespace embedded in each measurement description on the RIPE Atlas API. Used to distinguish measurements across Terraform states or workspaces. Defaults to \"terraform-provider-ripe-atlas\".",
			},
		},
	}
}

type providerModel struct {
	APIKey            types.String `tfsdk:"api_key"`
	Snapshot          types.String `tfsdk:"snapshot"`
	SnapshotCachePath types.String `tfsdk:"snapshot_cache_path"`
	SnapshotTTL       types.String `tfsdk:"snapshot_ttl"`
	Namespace         types.String `tfsdk:"namespace"`
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

	namespace := config.Namespace.ValueString()
	if namespace == "" {
		namespace = "terraform-provider-ripe-atlas"
	}
	codec := plan.NewTagCodec(namespace)
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

	probeSource, diag := buildProbeSource(config, verbose)
	if diag != "" {
		resp.Diagnostics.AddError("Invalid probe source configuration", diag)
		return
	}

	clients := &providerClients{
		apply:       applyClient,
		msm:         msmClient,
		probeSource: probeSource,
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

// buildProbeSource returns the appropriate ProbeSource based on provider config.
//
// When snapshot is set (via config or RIPE_ATLAS_SNAPSHOT env var), a
// FileProbeSource is returned: probes are read directly from that file with no
// freshness check. This preserves the existing behavior for operators who
// manage the snapshot file themselves.
//
// When snapshot is not set, a CachedProbeSource is returned: probes are served
// from the cache file at snapshot_cache_path (defaulting to DefaultCachePath)
// and automatically refreshed from the RIPE Atlas API when older than
// snapshot_ttl (defaulting to DefaultCacheTTL).
//
// Returns a non-empty diagnostic string on configuration error.
func buildProbeSource(config providerModel, verbose bool) (snapshot.ProbeSource, string) {
	snapshotPath := config.Snapshot.ValueString()
	if snapshotPath == "" {
		snapshotPath = os.Getenv("RIPE_ATLAS_SNAPSHOT")
	}

	if snapshotPath != "" {
		return &snapshot.FileProbeSource{Path: snapshotPath}, ""
	}

	var ttl time.Duration
	if s := config.SnapshotTTL.ValueString(); s != "" {
		var err error
		ttl, err = time.ParseDuration(s)
		if err != nil {
			return nil, fmt.Sprintf("invalid snapshot_ttl %q: %v", s, err)
		}
	}

	return &snapshot.CachedProbeSource{
		Path:   config.SnapshotCachePath.ValueString(),
		TTL:    ttl,
		Client: &atlasapi.ProbeClient{Verbose: verbose},
	}, ""
}
