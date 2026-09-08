package provider

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/JeongJaeSoon/terraform-provider-claude-enterprise/internal/client"
)

// Resolver indexes the /effective listing by member email and by spend limit
// ID. The listing is walked once per provider instance and cached, so a plan
// with many resources costs one listing, not one request per resource.
type Resolver struct {
	client *client.Client

	mu        sync.Mutex
	loaded    bool
	byEmail   map[string]string
	byLimitID map[string]client.EffectiveSpendLimit
}

// NewResolver builds a Resolver on top of the API client.
func NewResolver(c *client.Client) *Resolver {
	return &Resolver{client: c}
}

// load fills both indexes from one listing. Callers must hold r.mu, which
// serializes concurrent first-time fills so the listing runs at most once.
func (r *Resolver) load(ctx context.Context) error {
	if r.loaded {
		return nil
	}

	rows, err := r.client.ListEffectiveSpendLimits(ctx, nil)
	if err != nil {
		return fmt.Errorf("listing organization members: %w", err)
	}

	r.byEmail = make(map[string]string, len(rows))
	r.byLimitID = make(map[string]client.EffectiveSpendLimit)
	for _, row := range rows {
		if row.Source.Type == "user" && row.SpendLimitID != "" {
			r.byLimitID[row.SpendLimitID] = row
		}
		if row.Actor.Deleted || row.Actor.EmailAddress == "" {
			continue
		}
		r.byEmail[strings.ToLower(row.Actor.EmailAddress)] = row.Actor.UserID
	}
	r.loaded = true
	return nil
}

// UserIDByEmail resolves an email (case-insensitive) to a user ID.
// Deleted members are excluded.
func (r *Resolver) UserIDByEmail(ctx context.Context, email string) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if err := r.load(ctx); err != nil {
		return "", err
	}

	id, ok := r.byEmail[strings.ToLower(email)]
	if !ok {
		return "", fmt.Errorf("no organization member found with email %q", email)
	}
	return id, nil
}

// UserOverrideByLimitID returns the user-scope override carrying this spend
// limit ID, from the listing shared with UserIDByEmail.
func (r *Resolver) UserOverrideByLimitID(ctx context.Context, id string) (client.EffectiveSpendLimit, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if err := r.load(ctx); err != nil {
		return client.EffectiveSpendLimit{}, false, err
	}

	row, ok := r.byLimitID[id]
	return row, ok, nil
}
