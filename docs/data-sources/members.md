---
page_title: "claude-enterprise_members Data Source - claude-enterprise"
subcategory: ""
description: |-
  Lists every current organization member with their effective spend limit.
---

# claude-enterprise_members (Data Source)

Lists every current organization member with their effective spend limit.
Use `by_email` to resolve emails to user IDs. Deleted members are excluded.

~> **Note** This data source paginates through the organization's full
member list on every read. For large organizations this can be a
noticeably more expensive call than reading a single resource — avoid
refreshing it more often than necessary (e.g. don't reference it from
configuration that plans on every run without need).

## Example Usage

```hcl
data "claude-enterprise_members" "all" {}

output "alice_user_id" {
  value = data.claude-enterprise_members.all.by_email["alice@example.com"].user_id
}
```

## Schema

### Read-Only

- `by_email` (Attributes Map) Members keyed by lower-cased email address.
  Members with no email address on file are omitted from this map (see
  `by_user_id` for those). (see [below for nested schema](#nestedatt--by_email))
- `by_user_id` (Attributes Map) Members keyed by user ID. (see [below for nested schema](#nestedatt--by_user_id))

<a id="nestedatt--by_email"></a>
### Nested Schema for `by_email`

Read-Only:

- `user_id` (String) Member user ID (`user_01...`).
- `email` (String) Member email address (lower-cased).
- `name` (String) Member display name.
- `effective_amount` (String) Effective spend limit in minor units (cents).
  Null means unlimited; `"0"` means included usage only.
- `currency` (String) Billing currency code, e.g. `USD`.
- `period` (String) Limit period, currently `monthly`.
- `source_type` (String) Where the limit resolved from: `user`,
  `seat_tier`, `rbac_group`, or `organization` (open set — the API may add
  new source types).
- `spend_limit_id` (String) ID of the spend limit row the effective limit
  resolved from.
- `period_to_date_spend` (String) Spend accrued this period, informational
  only (may temporarily read `"0"`).

<a id="nestedatt--by_user_id"></a>
### Nested Schema for `by_user_id`

Read-Only:

- `user_id` (String) Member user ID (`user_01...`).
- `email` (String) Member email address (lower-cased). Null when the
  member has no email on file.
- `name` (String) Member display name.
- `effective_amount` (String) Effective spend limit in minor units (cents).
  Null means unlimited; `"0"` means included usage only.
- `currency` (String) Billing currency code, e.g. `USD`.
- `period` (String) Limit period, currently `monthly`.
- `source_type` (String) Where the limit resolved from: `user`,
  `seat_tier`, `rbac_group`, or `organization` (open set — the API may add
  new source types).
- `spend_limit_id` (String) ID of the spend limit row the effective limit
  resolved from.
- `period_to_date_spend` (String) Spend accrued this period, informational
  only (may temporarily read `"0"`).
