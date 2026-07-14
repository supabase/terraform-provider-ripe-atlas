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
		},
	}
}

type providerModel struct {
	APIKey types.String `tfsdk:"api_key"`
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

	codec := plan.NewTagCodec(plan.DefaultTagPrefix)
	client, err := atlasapi.NewApplyClient(apiKey, false, codec)
	if err != nil {
		resp.Diagnostics.AddError("Failed to create RIPE Atlas client", err.Error())
		return
	}

	resp.ResourceData = client
	resp.DataSourceData = client
}

func (p *ripeAtlasProvider) Resources(_ context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		NewMeasurementResource,
	}
}

func (p *ripeAtlasProvider) DataSources(_ context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{
		NewProbeSelectionDataSource,
	}
}
