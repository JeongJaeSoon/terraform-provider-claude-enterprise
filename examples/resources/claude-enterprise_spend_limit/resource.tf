# Address a member directly by email; the provider resolves the user id.
resource "claude-enterprise_spend_limit" "alice" {
  user_email = "alice@example.com"
  amount     = "75000" # 750.00 USD in cents
}

# Or pin by user id.
resource "claude-enterprise_spend_limit" "bob" {
  user_id = "user_01AbCdEfGh"
  amount  = "0" # included plan usage only
}
