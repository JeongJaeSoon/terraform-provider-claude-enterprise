package client

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
)

// Scope identifies what a spend limit applies to. Only type "user" is
// writable through this API; treat other types as an open set.
type Scope struct {
	Type   string `json:"type"`
	UserID string `json:"user_id,omitempty"`
}

// Source describes which hierarchy level an effective limit resolved from:
// user, seat_tier, rbac_group, or organization (open set).
type Source struct {
	Type     string `json:"type"`
	SeatTier string `json:"seat_tier,omitempty"`
}

// Actor is the member a spend limit row applies to.
type Actor struct {
	Type         string `json:"type"`
	UserID       string `json:"user_id"`
	Name         string `json:"name"`
	EmailAddress string `json:"email_address"`
	Deleted      bool   `json:"deleted"`
}

// SpendLimit is a configured spend-limit row (per-user override).
type SpendLimit struct {
	Type      string  `json:"type"`
	ID        string  `json:"id"`
	CreatedAt string  `json:"created_at"`
	UpdatedAt string  `json:"updated_at"`
	Scope     Scope   `json:"scope"`
	Amount    *string `json:"amount"`
	Currency  string  `json:"currency"`
	Period    string  `json:"period"`
}

// EffectiveSpendLimit is one member's resolved limit as returned by the
// /effective listing. A nil Amount means unlimited.
type EffectiveSpendLimit struct {
	Scope             Scope   `json:"scope"`
	Actor             Actor   `json:"actor"`
	Amount            *string `json:"amount"`
	Currency          string  `json:"currency"`
	Period            string  `json:"period"`
	Source            Source  `json:"source"`
	SpendLimitID      string  `json:"spend_limit_id"`
	PeriodToDateSpend string  `json:"period_to_date_spend"`
}

const (
	spendLimitsBasePath      = "/v1/organizations/spend_limits"
	spendLimitsEffectivePath = spendLimitsBasePath + "/effective"
	effectivePageLimit       = "100"
)

type effectivePage struct {
	Data     []EffectiveSpendLimit `json:"data"`
	NextPage *string               `json:"next_page"`
}

// ListEffectiveSpendLimits walks every page of the /effective listing.
// Pass userIDs to filter server-side; nil fetches the whole organization.
// Cursors are bound to their query parameters, so nothing but the cursor
// may change between pages.
func (c *Client) ListEffectiveSpendLimits(ctx context.Context, userIDs []string) ([]EffectiveSpendLimit, error) {
	q := url.Values{}
	q.Set("limit", effectivePageLimit)
	for _, id := range userIDs {
		q.Add("user_ids[]", id)
	}

	var all []EffectiveSpendLimit
	for {
		var page effectivePage
		if err := c.do(ctx, http.MethodGet, spendLimitsEffectivePath, q, nil, &page); err != nil {
			return nil, err
		}
		all = append(all, page.Data...)
		if page.NextPage == nil || *page.NextPage == "" {
			return all, nil
		}
		q.Set("page", *page.NextPage)
	}
}

// GetSpendLimit fetches one configured spend limit by ID.
func (c *Client) GetSpendLimit(ctx context.Context, id string) (*SpendLimit, error) {
	var sl SpendLimit
	if err := c.do(ctx, http.MethodGet, spendLimitsBasePath+"/"+url.PathEscape(id), nil, nil, &sl); err != nil {
		return nil, err
	}
	return &sl, nil
}

// UpsertSpendLimit creates or overwrites the per-user override keyed on
// (scope, period).
func (c *Client) UpsertSpendLimit(ctx context.Context, userID, amount, period string) (*SpendLimit, error) {
	body := map[string]any{
		"scope":  Scope{Type: "user", UserID: userID},
		"amount": amount,
		"period": period,
	}
	var sl SpendLimit
	if err := c.do(ctx, http.MethodPost, spendLimitsBasePath, nil, body, &sl); err != nil {
		return nil, err
	}
	if sl.ID == "" {
		return nil, fmt.Errorf("upsert response missing spend limit id")
	}
	return &sl, nil
}

// DeleteSpendLimit removes a per-user override; the member falls back to
// the inherited limit.
func (c *Client) DeleteSpendLimit(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodDelete, spendLimitsBasePath+"/"+url.PathEscape(id), nil, nil, nil)
}
