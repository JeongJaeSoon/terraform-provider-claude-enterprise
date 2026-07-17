package provider

import (
	"context"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// configureRequest builds a ConfigureRequest whose config carries the given
// raw attribute values, using the provider's real schema.
func configureRequest(t *testing.T, adminAPIKey, baseURL tftypes.Value) provider.ConfigureRequest {
	t.Helper()

	p := &claudeEnterpriseProvider{version: "test"}
	schemaResp := &provider.SchemaResponse{}
	p.Schema(context.Background(), provider.SchemaRequest{}, schemaResp)
	if schemaResp.Diagnostics.HasError() {
		t.Fatalf("provider schema diagnostics: %v", schemaResp.Diagnostics)
	}

	objType := tftypes.Object{
		AttributeTypes: map[string]tftypes.Type{
			"admin_api_key": tftypes.String,
			"base_url":      tftypes.String,
		},
	}
	return provider.ConfigureRequest{
		Config: tfsdk.Config{
			Raw: tftypes.NewValue(objType, map[string]tftypes.Value{
				"admin_api_key": adminAPIKey,
				"base_url":      baseURL,
			}),
			Schema: schemaResp.Schema,
		},
	}
}

// requireAttributeError asserts that diagnostics contain exactly one error
// whose summary matches wantSummary.
func requireAttributeError(t *testing.T, resp *provider.ConfigureResponse, wantSummary string) {
	t.Helper()

	if !resp.Diagnostics.HasError() {
		t.Fatalf("expected an error diagnostic with summary %q, got none", wantSummary)
	}
	for _, d := range resp.Diagnostics.Errors() {
		if d.Summary() == wantSummary {
			return
		}
	}
	var summaries []string
	for _, d := range resp.Diagnostics.Errors() {
		summaries = append(summaries, d.Summary())
	}
	t.Fatalf("expected error summary %q, got %q", wantSummary, strings.Join(summaries, "; "))
}

// TestConfigureDirect exercises Configure without a Terraform binary by
// constructing tfsdk.Config values against the real provider schema.
func TestConfigureDirect(t *testing.T) {
	nullString := tftypes.NewValue(tftypes.String, nil)
	unknownString := tftypes.NewValue(tftypes.String, tftypes.UnknownValue)

	t.Run("success wires client into resource and data source data", func(t *testing.T) {
		t.Setenv("ANTHROPIC_ADMIN_KEY", "")
		p := &claudeEnterpriseProvider{version: "test"}
		req := configureRequest(t,
			tftypes.NewValue(tftypes.String, "sk-ant-admin-test"),
			tftypes.NewValue(tftypes.String, "https://mock.invalid"),
		)
		resp := &provider.ConfigureResponse{}

		p.Configure(context.Background(), req, resp)

		if resp.Diagnostics.HasError() {
			t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
		}
		if resp.ResourceData == nil || resp.DataSourceData == nil {
			t.Fatalf("expected ResourceData and DataSourceData to be set, got %v / %v",
				resp.ResourceData, resp.DataSourceData)
		}
		if resp.ResourceData != resp.DataSourceData {
			t.Fatal("expected ResourceData and DataSourceData to be the same *providerData")
		}
		data, ok := resp.ResourceData.(*providerData)
		if !ok {
			t.Fatalf("expected ResourceData to be *providerData, got %T", resp.ResourceData)
		}
		if data.Client == nil {
			t.Fatal("expected providerData.Client to be non-nil")
		}
	})

	t.Run("env fallback when config key is null", func(t *testing.T) {
		t.Setenv("ANTHROPIC_ADMIN_KEY", "sk-ant-admin-env")
		p := &claudeEnterpriseProvider{version: "test"}
		req := configureRequest(t, nullString, nullString)
		resp := &provider.ConfigureResponse{}

		p.Configure(context.Background(), req, resp)

		if resp.Diagnostics.HasError() {
			t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
		}
		if data, ok := resp.ResourceData.(*providerData); !ok || data.Client == nil {
			t.Fatalf("expected *providerData with non-nil Client, got %#v", resp.ResourceData)
		}
	})

	t.Run("missing key errors on admin_api_key", func(t *testing.T) {
		t.Setenv("ANTHROPIC_ADMIN_KEY", "")
		p := &claudeEnterpriseProvider{version: "test"}
		req := configureRequest(t, nullString, nullString)
		resp := &provider.ConfigureResponse{}

		p.Configure(context.Background(), req, resp)

		requireAttributeError(t, resp, "Missing Admin API key")
		if resp.ResourceData != nil || resp.DataSourceData != nil {
			t.Fatal("expected no provider data on error")
		}
	})

	t.Run("unknown admin_api_key errors before client creation", func(t *testing.T) {
		t.Setenv("ANTHROPIC_ADMIN_KEY", "")
		p := &claudeEnterpriseProvider{version: "test"}
		req := configureRequest(t, unknownString, nullString)
		resp := &provider.ConfigureResponse{}

		p.Configure(context.Background(), req, resp)

		requireAttributeError(t, resp, "Unknown Admin API key")
		if resp.ResourceData != nil || resp.DataSourceData != nil {
			t.Fatal("expected no provider data on error")
		}
	})

	t.Run("unknown base_url errors before client creation", func(t *testing.T) {
		t.Setenv("ANTHROPIC_ADMIN_KEY", "")
		p := &claudeEnterpriseProvider{version: "test"}
		req := configureRequest(t,
			tftypes.NewValue(tftypes.String, "sk-ant-admin-test"),
			unknownString,
		)
		resp := &provider.ConfigureResponse{}

		p.Configure(context.Background(), req, resp)

		requireAttributeError(t, resp, "Unknown API base URL")
		if resp.ResourceData != nil || resp.DataSourceData != nil {
			t.Fatal("expected no provider data on error")
		}
	})
}
