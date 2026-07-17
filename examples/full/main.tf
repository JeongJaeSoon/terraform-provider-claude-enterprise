terraform {
  required_providers {
    claude-enterprise = {
      source  = "JeongJaeSoon/claude-enterprise"
      version = "~> 0.1"
    }
  }
}

provider "claude-enterprise" {}

variable "spend_limit_overrides" {
  description = "Per-member spend limit overrides in cents, keyed by email."
  type        = map(string)
  default     = {}
}

data "claude-enterprise_members" "all" {}

resource "claude-enterprise_spend_limit" "override" {
  for_each   = var.spend_limit_overrides
  user_email = each.key
  amount     = each.value
}

output "members_effective" {
  description = "Current effective limits for every member (read-only smoke check)."
  value = {
    for email, m in data.claude-enterprise_members.all.by_email :
    email => { amount = m.effective_amount, source = m.source_type, spent = m.period_to_date_spend }
  }
}
