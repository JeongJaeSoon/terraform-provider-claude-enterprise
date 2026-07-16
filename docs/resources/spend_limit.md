---
page_title: "claude-enterprise_spend_limit Resource - claude-enterprise"
subcategory: ""
description: |-
  Manages one member's per-user spend limit override.
---

# claude-enterprise_spend_limit (Resource)

Manages one member's per-user spend limit override. Creating adopts any
override that already exists for the same user and period (the API is an
upsert); deleting returns the member to their inherited limit.

## Example Usage

```hcl
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
```

## Schema

### Required

- `amount` (String) Spend limit in minor units of the billing currency, as a
  string (`"50000"` is 500.00 USD; `"0"` allows included usage only).
  Updated in place.

### Optional

- `user_id` (String) Member user ID (`user_01...`). Exactly one of
  `user_id` and `user_email` must be set. Changing the target user replaces
  the resource.
- `user_email` (String) Member email address, resolved to `user_id` at plan
  time. Exactly one of `user_id` and `user_email` must be set. If the same
  email later maps to a different user (member removed and re-invited), the
  resource is replaced.
- `period` (String) Limit period. Currently only `monthly` is supported by
  the API. Defaults to `"monthly"`. Changing this replaces the resource.

### Read-Only

- `id` (String) Spend limit ID (`spl_...`).
- `currency` (String) Billing currency code, e.g. `USD`.

## Replacement Rules

- Changing `user_id` or `user_email` (when it resolves to a different
  `user_id`) forces replacement — a spend limit belongs to one user, so
  retargeting it means creating a new override and deleting the old one.
- Changing `period` forces replacement, since the API does not support
  migrating an existing override to a different period.
- Changing `amount` updates the existing override in place (an upsert
  call), with no replacement.

## Import

Import by spend limit ID:

```shell
terraform import claude-enterprise_spend_limit.x spl_01AbC...
```

Or by user ID, which resolves that user's current per-user override. This
errors if the user has no per-user override — create the resource with
Terraform instead of importing in that case:

```shell
terraform import claude-enterprise_spend_limit.x user_01AbC...
```
