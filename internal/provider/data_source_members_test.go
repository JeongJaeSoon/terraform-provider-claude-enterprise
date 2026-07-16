package provider_test

import (
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"

	"github.com/JeongJaeSoon/terraform-provider-claude-enterprise/internal/provider/testutil"
)

func TestAccMembersDataSource(t *testing.T) {
	if os.Getenv("TF_ACC") == "" {
		t.Skip("TF_ACC not set")
	}
	mock := testutil.NewMockAdminAPI("test-key", []testutil.Member{
		{UserID: "user_01A", Email: "Alice@Example.com", Name: "Alice"},
		{UserID: "user_01B", Email: "bob@example.com", Name: "Bob"},
		{UserID: "user_01C", Email: "carol@example.com", Name: "Carol", Deleted: true},
		{UserID: "user_01D", Name: "NoMail"},
	})
	defer mock.Close()
	mock.SeedOverride("user_01B", "50000")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: protoV6Factories(),
		Steps: []resource.TestStep{
			{
				Config: providerConfig(mock.URL()) + `
data "claude-enterprise_members" "all" {}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					// Emails are normalized to lower case as map keys.
					resource.TestCheckResourceAttr("data.claude-enterprise_members.all", "by_email.alice@example.com.user_id", "user_01A"),
					resource.TestCheckResourceAttr("data.claude-enterprise_members.all", "by_email.alice@example.com.name", "Alice"),
					// Inherited (no override): effective_amount is null, source seat_tier.
					resource.TestCheckNoResourceAttr("data.claude-enterprise_members.all", "by_email.alice@example.com.effective_amount"),
					resource.TestCheckResourceAttr("data.claude-enterprise_members.all", "by_email.alice@example.com.source_type", "seat_tier"),
					// Overridden member.
					resource.TestCheckResourceAttr("data.claude-enterprise_members.all", "by_email.bob@example.com.effective_amount", "50000"),
					resource.TestCheckResourceAttr("data.claude-enterprise_members.all", "by_email.bob@example.com.source_type", "user"),
					resource.TestCheckResourceAttr("data.claude-enterprise_members.all", "by_user_id.user_01B.email", "bob@example.com"),
					// Deleted members are excluded.
					resource.TestCheckNoResourceAttr("data.claude-enterprise_members.all", "by_email.carol@example.com"),
					// Members without an email appear only in by_user_id, with a null email field.
					resource.TestCheckResourceAttr("data.claude-enterprise_members.all", "by_user_id.user_01D.user_id", "user_01D"),
					resource.TestCheckNoResourceAttr("data.claude-enterprise_members.all", "by_user_id.user_01D.email"),
					resource.TestCheckResourceAttr("data.claude-enterprise_members.all", "by_email.%", "2"),
					resource.TestCheckResourceAttr("data.claude-enterprise_members.all", "by_user_id.%", "3"),
				),
			},
		},
	})
}
