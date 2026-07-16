terraform {
  required_providers {
    claude-enterprise = {
      source  = "JeongJaeSoon/claude-enterprise"
      version = "~> 0.1"
    }
  }
}

# The admin API key is read from the ANTHROPIC_ADMIN_KEY environment
# variable; avoid putting it in configuration.
provider "claude-enterprise" {}
