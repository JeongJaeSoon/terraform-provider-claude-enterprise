package client

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

func TestDoSendsAuthHeaders(t *testing.T) {
	var gotAPIKey, gotVersion, gotContentType string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAPIKey = r.Header.Get("x-api-key")
		gotVersion = r.Header.Get("anthropic-version")
		gotContentType = r.Header.Get("content-type")
		w.Header().Set("content-type", "application/json")
		_, _ = w.Write([]byte(`{"ok": true}`))
	}))
	defer srv.Close()

	c, err := New(srv.URL, "sk-ant-admin-test")
	if err != nil {
		t.Fatal(err)
	}

	var out struct {
		OK bool `json:"ok"`
	}
	if err := c.do(context.Background(), http.MethodPost, "/v1/test", nil, map[string]string{"a": "b"}, &out); err != nil {
		t.Fatal(err)
	}
	if gotAPIKey != "sk-ant-admin-test" {
		t.Fatalf("x-api-key = %q", gotAPIKey)
	}
	if gotVersion != "2023-06-01" {
		t.Fatalf("anthropic-version = %q", gotVersion)
	}
	if gotContentType != "application/json" {
		t.Fatalf("content-type = %q", gotContentType)
	}
	if !out.OK {
		t.Fatal("response not decoded")
	}
}

func TestDoQueryParams(t *testing.T) {
	var gotQuery url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.Query()
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	c, _ := New(srv.URL, "k")
	q := url.Values{}
	q.Set("limit", "100")
	q.Add("user_ids[]", "user_01A")
	q.Add("user_ids[]", "user_01B")
	if err := c.do(context.Background(), http.MethodGet, "/v1/test", q, nil, nil); err != nil {
		t.Fatal(err)
	}
	if gotQuery.Get("limit") != "100" {
		t.Fatalf("limit = %q", gotQuery.Get("limit"))
	}
	if ids := gotQuery["user_ids[]"]; len(ids) != 2 || ids[0] != "user_01A" || ids[1] != "user_01B" {
		t.Fatalf("user_ids[] = %v", ids)
	}
}

func TestDoAPIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"type":"error","error":{"type":"not_found_error","message":"spend limit not found"},"request_id":"req_123"}`))
	}))
	defer srv.Close()

	c, _ := New(srv.URL, "k")
	err := c.do(context.Background(), http.MethodGet, "/v1/missing", nil, nil, nil)
	if err == nil {
		t.Fatal("expected error")
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected *APIError, got %T: %v", err, err)
	}
	if apiErr.StatusCode != 404 || apiErr.Type != "not_found_error" || apiErr.RequestID != "req_123" {
		t.Fatalf("unexpected APIError: %+v", apiErr)
	}
	if !IsNotFound(err) {
		t.Fatal("IsNotFound should be true")
	}
	if IsNotFound(errors.New("plain")) {
		t.Fatal("IsNotFound(plain) should be false")
	}
}

func TestDoNonJSONError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte(`upstream exploded`))
	}))
	defer srv.Close()

	c, _ := New(srv.URL, "k", WithMaxRetries(0))
	err := c.do(context.Background(), http.MethodGet, "/v1/x", nil, nil, nil)
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.StatusCode != 502 {
		t.Fatalf("expected 502 APIError, got %v", err)
	}
}

func TestNewValidation(t *testing.T) {
	if _, err := New("://bad url", "k"); err == nil {
		t.Fatal("expected error for invalid base URL")
	}
	if _, err := New(DefaultBaseURL, ""); err == nil {
		t.Fatal("expected error for empty api key")
	}
}
