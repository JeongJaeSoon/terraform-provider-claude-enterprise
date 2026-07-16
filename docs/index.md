---
page_title: "claude-enterprise Provider"
subcategory: ""
description: |-
  Manage Claude Enterprise per-user spend limits via the Admin API.
---

# claude-enterprise Provider

Manage Claude Enterprise per-user spend limits via the Admin API.

This is a community-maintained provider and is not an official Anthropic
product.

## Requirements

- A Claude Enterprise organization with usage credits enabled.
- An Admin API key (`sk-ant-admin...`) scoped with `read:spend_limits` and
  `write:spend_limits`.

## Example Usage

```hcl
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
```

## Authentication

The provider needs a scoped Admin API key. Prefer setting the
`ANTHROPIC_ADMIN_KEY` environment variable over the `admin_api_key`
attribute so the key never lands in configuration or state:

```shell
export ANTHROPIC_ADMIN_KEY="sk-ant-admin..."
```

If both are set, the `admin_api_key` attribute in configuration takes
precedence over the environment variable.

## Rate Limits

The Admin API allows 60 requests per minute per organization. The provider's
client throttles itself to 50 requests per minute by default and retries
`429` (and `5xx`) responses with exponential backoff, honoring any
`Retry-After` header the API returns. Large organizations or highly parallel
`terraform apply` runs may still want to lower `-parallelism` to stay well
under the limit.

## Schema

### Optional

- `admin_api_key` (String, Sensitive) Scoped Admin API key
  (`sk-ant-admin...`) with `read:spend_limits` and `write:spend_limits`
  scopes. Defaults to the `ANTHROPIC_ADMIN_KEY` environment variable. Prefer
  the environment variable so the key never lands in configuration or state.
- `base_url` (String) API base URL. Defaults to `https://api.anthropic.com`.
  Override for testing or proxies.
