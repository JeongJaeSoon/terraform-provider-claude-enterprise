package provider

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/JeongJaeSoon/terraform-provider-claude-enterprise/internal/client"
)

// Resolver maps member emails to user IDs. The /effective listing is
// walked once per provider instance and cached, so a plan with many
// email-keyed resources costs one listing, not one per resource.
type Resolver struct {
	client *client.Client

	mu      sync.Mutex
	loaded  bool
	byEmail map[string]string
}

// NewResolver builds a Resolver on top of the API client.
func NewResolver(c *client.Client) *Resolver {
	return &Resolver{client: c}
}

// UserIDByEmail resolves an email (case-insensitive) to a user ID.
// Deleted members are excluded. The mutex intentionally serializes
// concurrent first-time fills so the listing runs at most once.
func (r *Resolver) UserIDByEmail(ctx context.Context, email string) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if !r.loaded {
		rows, err := r.client.ListEffectiveSpendLimits(ctx, nil)
		if err != nil {
			return "", fmt.Errorf("listing organization members: %w", err)
		}
		r.byEmail = make(map[string]string, len(rows))
		for _, row := range rows {
			if row.Actor.Deleted || row.Actor.EmailAddress == "" {
				continue
			}
			r.byEmail[strings.ToLower(row.Actor.EmailAddress)] = row.Actor.UserID
		}
		r.loaded = true
	}

	id, ok := r.byEmail[strings.ToLower(email)]
	if !ok {
		return "", fmt.Errorf("no organization member found with email %q", email)
	}
	return id, nil
}
