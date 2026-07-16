package provider_test

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"testing"

	fwprovider "github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"

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

// protoV6Factories wires the in-process provider into terraform-plugin-testing.
func protoV6Factories() map[string]func() (tfprotov6.ProviderServer, error) {
	return map[string]func() (tfprotov6.ProviderServer, error){
		"claude-enterprise": providerserver.NewProtocol6WithError(provider.New("test")()),
	}
}

// providerConfig points the provider at a mock server.
func providerConfig(mockURL string) string {
	return fmt.Sprintf(`
provider "claude-enterprise" {
  admin_api_key = "test-key"
  base_url      = %q
}
`, mockURL)
}

func TestAccProviderMissingAPIKey(t *testing.T) {
	t.Skip("enabled in Task 7")
	if os.Getenv("TF_ACC") == "" {
		t.Skip("TF_ACC not set")
	}
	t.Setenv("ANTHROPIC_ADMIN_KEY", "")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: protoV6Factories(),
		Steps: []resource.TestStep{
			{
				Config: `
provider "claude-enterprise" {}

data "claude-enterprise_members" "all" {}
`,
				ExpectError: regexp.MustCompile(`(?s)Missing Admin API key`),
			},
		},
	})
}

func TestConfigureResolvesAPIKeyFromEnv(t *testing.T) {
	// Unit-level check of the key resolution helper (no Terraform involved).
	t.Setenv("ANTHROPIC_ADMIN_KEY", "env-key")
	if got := provider.ResolveAPIKeyForTest("", ""); got != "env-key" {
		t.Fatalf("expected env fallback, got %q", got)
	}
	if got := provider.ResolveAPIKeyForTest("explicit", "ignored"); got != "explicit" {
		t.Fatalf("config value must win, got %q", got)
	}
}
