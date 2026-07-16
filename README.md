# Terraform Provider for Claude Enterprise

This is a community-maintained provider and is not an official Anthropic
product.

It manages per-member spend limits for a [Claude Enterprise][claude-enterprise]
organization through the Admin API: a `claude-enterprise_members` data
source to look up members and their current effective limits, and a
`claude-enterprise_spend_limit` resource to set per-user overrides.

[claude-enterprise]: https://www.anthropic.com/enterprise

## Requirements

- A Claude Enterprise organization with usage credits enabled.
- An Admin API key (`sk-ant-admin...`) scoped with `read:spend_limits` and
  `write:spend_limits`.
- [Terraform](https://www.terraform.io/downloads) >= 1.0, or
  [OpenTofu](https://opentofu.org/) >= 1.6, to use the provider.
- [Go](https://go.dev/dl/) >= 1.26 to build or develop the provider itself.

## Usage

```hcl
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
```

See [`examples/`](./examples) for more, and [`docs/`](./docs) for the full
schema reference.

## Authentication

Set the `ANTHROPIC_ADMIN_KEY` environment variable rather than the
`admin_api_key` provider attribute, so the key never lands in configuration
or state:

```shell
export ANTHROPIC_ADMIN_KEY="sk-ant-admin..."
```

## Development

```shell
make test     # unit tests
make testacc  # acceptance tests against a mock Admin API server (requires a local terraform binary)
```

### Local development

```hcl
# ~/.terraformrc (or a dedicated file via TF_CLI_CONFIG_FILE)
provider_installation {
  dev_overrides {
    "JeongJaeSoon/claude-enterprise" = "/Users/<you>/go/bin"
  }
  direct {}
}
```

Run `go install .`, then `terraform plan` in any configuration that
references `JeongJaeSoon/claude-enterprise` — no `terraform init` needed
for the overridden provider.

## License

[Mozilla Public License 2.0](./LICENSE)
