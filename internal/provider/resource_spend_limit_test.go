package provider_test

import (
	"fmt"
	"os"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"github.com/JeongJaeSoon/terraform-provider-claude-enterprise/internal/provider/testutil"
)

func mustCompile(t *testing.T, pattern string) *regexp.Regexp {
	t.Helper()
	return regexp.MustCompile(`(?s)` + pattern)
}

func newResourceMock(t *testing.T) *testutil.MockAdminAPI {
	t.Helper()
	mock := testutil.NewMockAdminAPI("test-key", []testutil.Member{
		{UserID: "user_01A", Email: "alice@example.com", Name: "Alice"},
		{UserID: "user_01B", Email: "bob@example.com", Name: "Bob"},
	})
	t.Cleanup(mock.Close)
	return mock
}

func checkNoOverridesLeft(mock *testutil.MockAdminAPI) resource.TestCheckFunc {
	return func(_ *terraform.State) error {
		if n := mock.OverrideCount(); n != 0 {
			return fmt.Errorf("expected 0 overrides after destroy, found %d", n)
		}
		return nil
	}
}

func TestAccSpendLimitLifecycleByUserID(t *testing.T) {
	if os.Getenv("TF_ACC") == "" {
		t.Skip("TF_ACC not set")
	}
	mock := newResourceMock(t)

	configFor := func(amount string) string {
		return providerConfig(mock.URL()) + fmt.Sprintf(`
resource "claude-enterprise_spend_limit" "test" {
  user_id = "user_01A"
  amount  = %q
}
`, amount)
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: protoV6Factories(),
		CheckDestroy:             checkNoOverridesLeft(mock),
		Steps: []resource.TestStep{
			{ // Create
				Config: configFor("50000"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("claude-enterprise_spend_limit.test", "id"),
					resource.TestCheckResourceAttr("claude-enterprise_spend_limit.test", "user_id", "user_01A"),
					resource.TestCheckResourceAttr("claude-enterprise_spend_limit.test", "amount", "50000"),
					resource.TestCheckResourceAttr("claude-enterprise_spend_limit.test", "period", "monthly"),
					resource.TestCheckResourceAttr("claude-enterprise_spend_limit.test", "currency", "USD"),
					resource.TestCheckNoResourceAttr("claude-enterprise_spend_limit.test", "user_email"),
				),
			},
			{ // Update amount in place
				Config: configFor("75000"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("claude-enterprise_spend_limit.test", "amount", "75000"),
				),
			},
			{ // Import by spend_limit_id
				ResourceName:      "claude-enterprise_spend_limit.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

func TestAccSpendLimitDisappearsOutOfBand(t *testing.T) {
	if os.Getenv("TF_ACC") == "" {
		t.Skip("TF_ACC not set")
	}
	mock := newResourceMock(t)

	config := providerConfig(mock.URL()) + `
resource "claude-enterprise_spend_limit" "test" {
  user_id = "user_01B"
  amount  = "10000"
}
`
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: protoV6Factories(),
		Steps: []resource.TestStep{
			{Config: config},
			{ // Someone deletes the override in the console; refresh must plan recreation.
				PreConfig: func() {
					// Simulate out-of-band deletion by clearing the stored override.
					mock.DeleteAllOverrides()
				},
				Config:             config,
				ExpectNonEmptyPlan: true,
				PlanOnly:           true,
			},
		},
	})
}

func TestAccSpendLimitRequiresExactlyOneIdentity(t *testing.T) {
	if os.Getenv("TF_ACC") == "" {
		t.Skip("TF_ACC not set")
	}
	mock := newResourceMock(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: protoV6Factories(),
		Steps: []resource.TestStep{
			{
				Config: providerConfig(mock.URL()) + `
resource "claude-enterprise_spend_limit" "test" {
  user_id    = "user_01A"
  user_email = "alice@example.com"
  amount     = "1"
}
`,
				ExpectError: mustCompile(t, `Exactly one of`),
			},
			{
				Config: providerConfig(mock.URL()) + `
resource "claude-enterprise_spend_limit" "test" {
  amount = "1"
}
`,
				ExpectError: mustCompile(t, `Exactly one of`),
			},
		},
	})
}
