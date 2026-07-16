package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// claudeEnterpriseProvider implements a Terraform provider for the Claude
// Enterprise Admin API (spend limits).
type claudeEnterpriseProvider struct {
	version string
}

type providerModel struct {
	AdminAPIKey types.String `tfsdk:"admin_api_key"`
	BaseURL     types.String `tfsdk:"base_url"`
}

// New returns a provider factory. version is set by goreleaser at build time.
func New(version string) func() provider.Provider {
	return func() provider.Provider {
		return &claudeEnterpriseProvider{version: version}
	}
}

func (p *claudeEnterpriseProvider) Metadata(_ context.Context, _ provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "claude-enterprise"
	resp.Version = p.version
}

func (p *claudeEnterpriseProvider) Schema(_ context.Context, _ provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manage Claude Enterprise per-user spend limits via the Admin API. " +
			"This is a community provider, not an official Anthropic product.",
		Attributes: map[string]schema.Attribute{
			"admin_api_key": schema.StringAttribute{
				Optional:  true,
				Sensitive: true,
				MarkdownDescription: "Scoped Admin API key (`sk-ant-admin...`) with `read:spend_limits` " +
					"and `write:spend_limits` scopes. Defaults to the `ANTHROPIC_ADMIN_KEY` environment " +
					"variable. Prefer the environment variable so the key never lands in configuration or state.",
			},
			"base_url": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "API base URL. Defaults to `https://api.anthropic.com`. Override for testing or proxies.",
			},
		},
	}
}

// Configure is completed in Task 6; for now it does nothing so the provider compiles.
func (p *claudeEnterpriseProvider) Configure(_ context.Context, _ provider.ConfigureRequest, _ *provider.ConfigureResponse) {
}

func (p *claudeEnterpriseProvider) Resources(_ context.Context) []func() resource.Resource {
	return nil
}

func (p *claudeEnterpriseProvider) DataSources(_ context.Context) []func() datasource.DataSource {
	return nil
}
