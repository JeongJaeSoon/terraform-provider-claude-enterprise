package testutil

import (
	"context"
	"net/http"
	"testing"

	"github.com/JeongJaeSoon/terraform-provider-claude-enterprise/internal/client"
)

func newMock(t *testing.T) (*MockAdminAPI, *client.Client) {
	t.Helper()
	m := NewMockAdminAPI("test-key", []Member{
		{UserID: "user_01A", Email: "alice@example.com", Name: "Alice"},
		{UserID: "user_01B", Email: "bob@example.com", Name: "Bob"},
		{UserID: "user_01C", Email: "carol@example.com", Name: "Carol", Deleted: true},
	})
	t.Cleanup(m.Close)
	c, err := client.New(m.URL(), "test-key", client.WithRequestsPerMinute(60000))
	if err != nil {
		t.Fatal(err)
	}
	return m, c
}

func TestMockEffectivePaginationAndInheritance(t *testing.T) {
	m, c := newMock(t)

	rows, err := c.ListEffectiveSpendLimits(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 3 {
		t.Fatalf("expected 3 rows, got %d", len(rows))
	}
	// Page size 2 with 3 members forces two requests.
	if m.EffectiveRequestCount() != 2 {
		t.Fatalf("expected 2 paginated requests, got %d", m.EffectiveRequestCount())
	}
	if rows[0].Amount != nil || rows[0].Source.Type != "seat_tier" {
		t.Fatalf("expected inherited row, got %+v", rows[0])
	}
	if !rows[2].Actor.Deleted {
		t.Fatal("carol should be flagged deleted")
	}
}

func TestMockUpsertLifecycle(t *testing.T) {
	_, c := newMock(t)
	ctx := context.Background()

	created, err := c.UpsertSpendLimit(ctx, "user_01A", "50000", "monthly")
	if err != nil {
		t.Fatal(err)
	}
	if created.Scope.UserID != "user_01A" || *created.Amount != "50000" || created.Currency != "USD" {
		t.Fatalf("created wrong: %+v", created)
	}

	// Upsert again: same id, new amount.
	updated, err := c.UpsertSpendLimit(ctx, "user_01A", "75000", "monthly")
	if err != nil {
		t.Fatal(err)
	}
	if updated.ID != created.ID || *updated.Amount != "75000" {
		t.Fatalf("upsert must keep id and update amount: %+v vs %+v", created, updated)
	}

	got, err := c.GetSpendLimit(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if *got.Amount != "75000" {
		t.Fatalf("get after upsert: %+v", got)
	}

	// Effective row now reflects the override.
	rows, err := c.ListEffectiveSpendLimits(ctx, []string{"user_01A"})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].Source.Type != "user" || *rows[0].Amount != "75000" || rows[0].SpendLimitID != created.ID {
		t.Fatalf("effective row wrong: %+v", rows)
	}

	if err := c.DeleteSpendLimit(ctx, created.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := c.GetSpendLimit(ctx, created.ID); !client.IsNotFound(err) {
		t.Fatalf("expected 404 after delete, got %v", err)
	}
	if err := c.DeleteSpendLimit(ctx, created.ID); !client.IsNotFound(err) {
		t.Fatalf("second delete should 404, got %v", err)
	}
}

func TestMockRejectsUnknownUserAndBadScope(t *testing.T) {
	_, c := newMock(t)
	if _, err := c.UpsertSpendLimit(context.Background(), "user_ghost", "1", "monthly"); !client.IsNotFound(err) {
		t.Fatalf("expected 404 for unknown user, got %v", err)
	}
}

func TestMockAuth(t *testing.T) {
	m := NewMockAdminAPI("right-key", []Member{{UserID: "user_01A", Email: "a@example.com", Name: "A"}})
	defer m.Close()
	c, _ := client.New(m.URL(), "wrong-key", client.WithRequestsPerMinute(60000))
	_, err := c.ListEffectiveSpendLimits(context.Background(), nil)
	if err == nil {
		t.Fatal("expected 401 error")
	}
}

func TestMockSeedOverride(t *testing.T) {
	m, c := newMock(t)
	id := m.SeedOverride("user_01B", "12300")
	got, err := c.GetSpendLimit(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	if got.Scope.UserID != "user_01B" || *got.Amount != "12300" {
		t.Fatalf("seeded wrong: %+v", got)
	}
	if m.OverrideCount() != 1 {
		t.Fatalf("OverrideCount = %d", m.OverrideCount())
	}
}

func TestMockRejectsOutOfRangeCursor(t *testing.T) {
	m, _ := newMock(t)
	for _, cursor := range []string{"page_-1", "page_99"} {
		req, err := http.NewRequest(http.MethodGet, m.URL()+"/v1/organizations/spend_limits/effective?page="+cursor, nil)
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("x-api-key", "test-key")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("cursor %q: expected 400, got %d", cursor, resp.StatusCode)
		}
	}
}
