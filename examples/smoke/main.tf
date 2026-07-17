terraform {
  required_providers {
    claude-enterprise = {
      source = "JeongJaeSoon/claude-enterprise"
    }
  }
}

provider "claude-enterprise" {
  base_url = var.mock_url
}

variable "mock_url" { type = string }

data "claude-enterprise_members" "all" {}

resource "claude-enterprise_spend_limit" "alice" {
  user_email = "alice@example.com"
  amount     = "50000"
}

output "members" {
  value = keys(data.claude-enterprise_members.all.by_email)
}
