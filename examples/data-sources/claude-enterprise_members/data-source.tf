data "claude-enterprise_members" "all" {}

output "alice_user_id" {
  value = data.claude-enterprise_members.all.by_email["alice@example.com"].user_id
}
