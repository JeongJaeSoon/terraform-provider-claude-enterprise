package provider_test

import (
	"context"
	"testing"

	fwprovider "github.com/hashicorp/terraform-plugin-framework/provider"

	"github.com/JeongJaeSoon/terraform-provider-claude-enterprise/internal/provider"
)

func TestProviderMetadata(t *testing.T) {
	p := provider.New("test")()

	resp := &fwprovider.MetadataResponse{}
	p.Metadata(context.Background(), fwprovider.MetadataRequest{}, resp)

	if resp.TypeName != "claude-enterprise" {
		t.Fatalf("expected type name %q, got %q", "claude-enterprise", resp.TypeName)
	}
	if resp.Version != "test" {
		t.Fatalf("expected version %q, got %q", "test", resp.Version)
	}
}

func TestProviderSchemaValid(t *testing.T) {
	p := provider.New("test")()

	resp := &fwprovider.SchemaResponse{}
	p.Schema(context.Background(), fwprovider.SchemaRequest{}, resp)

	if diags := resp.Schema.ValidateImplementation(context.Background()); diags.HasError() {
		t.Fatalf("provider schema invalid: %v", diags)
	}
	for _, name := range []string{"admin_api_key", "base_url"} {
		if _, ok := resp.Schema.Attributes[name]; !ok {
			t.Fatalf("expected schema attribute %q", name)
		}
	}
}
