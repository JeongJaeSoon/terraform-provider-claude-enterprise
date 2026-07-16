package provider

import (
	"context"
	"os"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/JeongJaeSoon/terraform-provider-claude-enterprise/internal/client"
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

// providerData is handed to every resource and data source via
// resp.ResourceData / resp.DataSourceData.
type providerData struct {
	Client *client.Client
}

// resolveAPIKey returns the effective key: explicit config wins over the
// ANTHROPIC_ADMIN_KEY environment variable.
func resolveAPIKey(configValue, envValue string) string {
	if configValue != "" {
		return configValue
	}
	return envValue
}

// ResolveAPIKeyForTest exposes resolveAPIKey to the external test package.
func ResolveAPIKeyForTest(configValue, envValue string) string {
	if envValue == "" {
		envValue = os.Getenv("ANTHROPIC_ADMIN_KEY")
	}
	return resolveAPIKey(configValue, envValue)
}

func (p *claudeEnterpriseProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var config providerModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Unknown values (e.g. references to attributes computed only after
	// apply) cannot be used to build the client; fail with guidance instead
	// of silently falling back.
	if config.AdminAPIKey.IsUnknown() {
		resp.Diagnostics.AddAttributeError(
			path.Root("admin_api_key"),
			"Unknown Admin API key",
			"The provider cannot create the API client because admin_api_key depends on a value "+
				"known only after apply. Set a static value or use the ANTHROPIC_ADMIN_KEY environment variable.",
		)
	}
	if config.BaseURL.IsUnknown() {
		resp.Diagnostics.AddAttributeError(
			path.Root("base_url"),
			"Unknown API base URL",
			"The provider cannot create the API client because base_url depends on a value "+
				"known only after apply. Set a static value or remove the attribute to use the default endpoint.",
		)
	}
	if resp.Diagnostics.HasError() {
		return
	}

	apiKey := resolveAPIKey(config.AdminAPIKey.ValueString(), os.Getenv("ANTHROPIC_ADMIN_KEY"))
	if apiKey == "" {
		resp.Diagnostics.AddAttributeError(
			path.Root("admin_api_key"),
			"Missing Admin API key",
			"Set the admin_api_key provider attribute or the ANTHROPIC_ADMIN_KEY environment variable. "+
				"The key must be a scoped Admin API key (sk-ant-admin...) with read:spend_limits and write:spend_limits scopes.",
		)
		return
	}

	baseURL := client.DefaultBaseURL
	if !config.BaseURL.IsNull() {
		baseURL = config.BaseURL.ValueString()
	}

	c, err := client.New(baseURL, apiKey)
	if err != nil {
		resp.Diagnostics.AddError("Failed to create API client", err.Error())
		return
	}

	data := &providerData{Client: c}
	resp.ResourceData = data
	resp.DataSourceData = data
}

func (p *claudeEnterpriseProvider) Resources(_ context.Context) []func() resource.Resource {
	return nil
}

func (p *claudeEnterpriseProvider) DataSources(_ context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{NewMembersDataSource}
}
