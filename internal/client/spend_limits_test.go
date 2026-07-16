package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestListEffectiveSpendLimitsPaginates(t *testing.T) {
	// Three members served two per page; the cursor from next_page must come
	// back as the page parameter with other parameters unchanged.
	pages := map[string]string{
		"": `{"data":[
			{"scope":{"type":"user","user_id":"user_01A"},
			 "actor":{"type":"user_actor","user_id":"user_01A","name":"A","email_address":"a@example.com","deleted":false},
			 "amount":"50000","currency":"USD","period":"monthly",
			 "source":{"type":"user"},"spend_limit_id":"spl_01A","period_to_date_spend":"100"},
			{"scope":{"type":"user","user_id":"user_01B"},
			 "actor":{"type":"user_actor","user_id":"user_01B","name":"B","email_address":"b@example.com","deleted":false},
			 "amount":null,"currency":"USD","period":"monthly",
			 "source":{"type":"seat_tier","seat_tier":"enterprise_standard"},"spend_limit_id":"spl_01T","period_to_date_spend":"0"}
		],"next_page":"page_2"}`,
		"page_2": `{"data":[
			{"scope":{"type":"user","user_id":"user_01C"},
			 "actor":{"type":"user_actor","user_id":"user_01C","name":"C","email_address":"c@example.com","deleted":true},
			 "amount":"0","currency":"USD","period":"monthly",
			 "source":{"type":"organization"},"spend_limit_id":"spl_01O","period_to_date_spend":"41280.125"}
		],"next_page":null}`,
	}
	var cursors []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/organizations/spend_limits/effective" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		if r.URL.Query().Get("limit") != "100" {
			t.Errorf("limit must stay constant across pages, got %q", r.URL.Query().Get("limit"))
		}
		cursor := r.URL.Query().Get("page")
		cursors = append(cursors, cursor)
		body, ok := pages[cursor]
		if !ok {
			t.Errorf("unexpected cursor %q", cursor)
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	c, _ := New(srv.URL, "k", WithRequestsPerMinute(60000))
	rows, err := c.ListEffectiveSpendLimits(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 3 {
		t.Fatalf("expected 3 rows, got %d", len(rows))
	}
	if len(cursors) != 2 || cursors[0] != "" || cursors[1] != "page_2" {
		t.Fatalf("unexpected cursor sequence %v", cursors)
	}
	if rows[0].Actor.EmailAddress != "a@example.com" || *rows[0].Amount != "50000" || rows[0].Source.Type != "user" {
		t.Fatalf("row 0 decoded wrong: %+v", rows[0])
	}
	if rows[1].Amount != nil {
		t.Fatalf("row 1 amount should be nil (unlimited), got %v", *rows[1].Amount)
	}
	if !rows[2].Actor.Deleted || rows[2].PeriodToDateSpend != "41280.125" {
		t.Fatalf("row 2 decoded wrong: %+v", rows[2])
	}
}

func TestListEffectiveSpendLimitsUserIDsFilter(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ids := r.URL.Query()["user_ids[]"]
		if len(ids) != 2 || ids[0] != "user_01A" || ids[1] != "user_01B" {
			t.Errorf("user_ids[] = %v", ids)
		}
		_, _ = w.Write([]byte(`{"data":[],"next_page":null}`))
	}))
	defer srv.Close()

	c, _ := New(srv.URL, "k", WithRequestsPerMinute(60000))
	if _, err := c.ListEffectiveSpendLimits(context.Background(), []string{"user_01A", "user_01B"}); err != nil {
		t.Fatal(err)
	}
}

func TestGetSpendLimit(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/v1/organizations/spend_limits/spl_01X" {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"type":"spend_limit","id":"spl_01X","created_at":"2026-05-11T10:02:44Z","updated_at":"2026-05-11T10:02:44Z","scope":{"type":"user","user_id":"user_01A"},"amount":"75000","currency":"USD","period":"monthly"}`))
	}))
	defer srv.Close()

	c, _ := New(srv.URL, "k", WithRequestsPerMinute(60000))
	sl, err := c.GetSpendLimit(context.Background(), "spl_01X")
	if err != nil {
		t.Fatal(err)
	}
	if sl.ID != "spl_01X" || sl.Scope.UserID != "user_01A" || *sl.Amount != "75000" || sl.Currency != "USD" {
		t.Fatalf("decoded wrong: %+v", sl)
	}
}

func TestUpsertSpendLimit(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/organizations/spend_limits" {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
		var body struct {
			Scope  Scope  `json:"scope"`
			Amount string `json:"amount"`
			Period string `json:"period"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode body: %v", err)
		}
		if body.Scope.Type != "user" || body.Scope.UserID != "user_01A" || body.Amount != "75000" || body.Period != "monthly" {
			t.Errorf("unexpected body: %+v", body)
		}
		fmt.Fprint(w, `{"type":"spend_limit","id":"spl_01N","created_at":"2026-05-11T10:02:44Z","updated_at":"2026-05-11T10:02:44Z","scope":{"type":"user","user_id":"user_01A"},"amount":"75000","currency":"USD","period":"monthly"}`)
	}))
	defer srv.Close()

	c, _ := New(srv.URL, "k", WithRequestsPerMinute(60000))
	sl, err := c.UpsertSpendLimit(context.Background(), "user_01A", "75000", "monthly")
	if err != nil {
		t.Fatal(err)
	}
	if sl.ID != "spl_01N" {
		t.Fatalf("decoded wrong: %+v", sl)
	}
}

func TestDeleteSpendLimit(t *testing.T) {
	var called bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		if r.Method != http.MethodDelete || r.URL.Path != "/v1/organizations/spend_limits/spl_01X" {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	c, _ := New(srv.URL, "k", WithRequestsPerMinute(60000))
	if err := c.DeleteSpendLimit(context.Background(), "spl_01X"); err != nil {
		t.Fatal(err)
	}
	if !called {
		t.Fatal("DELETE not sent")
	}
}
