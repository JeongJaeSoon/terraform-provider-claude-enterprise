package provider

import (
	"context"
	"strings"
	"testing"

	"github.com/JeongJaeSoon/terraform-provider-claude-enterprise/internal/client"
	"github.com/JeongJaeSoon/terraform-provider-claude-enterprise/internal/provider/testutil"
)

func newResolverFixture(t *testing.T) (*testutil.MockAdminAPI, *Resolver) {
	t.Helper()
	mock := testutil.NewMockAdminAPI("k", []testutil.Member{
		{UserID: "user_01A", Email: "Alice@Example.com", Name: "Alice"},
		{UserID: "user_01B", Email: "bob@example.com", Name: "Bob"},
		{UserID: "user_01C", Email: "carol@example.com", Name: "Carol", Deleted: true},
	})
	t.Cleanup(mock.Close)
	c, err := client.New(mock.URL(), "k", client.WithRequestsPerMinute(60000))
	if err != nil {
		t.Fatal(err)
	}
	return mock, NewResolver(c)
}

func TestResolverCaseInsensitiveAndCached(t *testing.T) {
	mock, r := newResolverFixture(t)
	ctx := context.Background()

	id, err := r.UserIDByEmail(ctx, "alice@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if id != "user_01A" {
		t.Fatalf("got %q", id)
	}

	// Page size 2, 3 members -> the initial fill takes exactly 2 requests.
	fills := mock.EffectiveRequestCount()
	if fills != 2 {
		t.Fatalf("expected 2 requests for cache fill, got %d", fills)
	}

	// Subsequent lookups (any case) must not hit the API again.
	if _, err := r.UserIDByEmail(ctx, "BOB@EXAMPLE.COM"); err != nil {
		t.Fatal(err)
	}
	if mock.EffectiveRequestCount() != fills {
		t.Fatalf("cache miss: request count grew to %d", mock.EffectiveRequestCount())
	}
}

func TestResolverUnknownEmail(t *testing.T) {
	_, r := newResolverFixture(t)
	_, err := r.UserIDByEmail(context.Background(), "ghost@example.com")
	if err == nil || !strings.Contains(err.Error(), "ghost@example.com") {
		t.Fatalf("expected error naming the email, got %v", err)
	}
}

func TestResolverExcludesDeletedMembers(t *testing.T) {
	_, r := newResolverFixture(t)
	if _, err := r.UserIDByEmail(context.Background(), "carol@example.com"); err == nil {
		t.Fatal("deleted member must not resolve")
	}
}
