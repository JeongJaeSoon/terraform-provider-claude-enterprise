package client

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

// newTestClient returns a client whose sleep records delays instead of
// waiting. The rate limiter is effectively disabled (60000 req/min) so tests
// don't block in real time; caller-supplied opts can still override it.
func newTestClient(t *testing.T, baseURL string, opts ...Option) (*Client, *[]time.Duration) {
	t.Helper()
	c, err := New(baseURL, "k", append([]Option{WithRequestsPerMinute(60000)}, opts...)...)
	if err != nil {
		t.Fatal(err)
	}
	var slept []time.Duration
	c.sleep = func(_ context.Context, d time.Duration) error {
		slept = append(slept, d)
		return nil
	}
	return c, &slept
}

func TestRetryOn429ThenSuccess(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) <= 2 {
			w.Header().Set("Retry-After", "3")
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"type":"error","error":{"type":"rate_limit_error","message":"slow down"}}`))
			return
		}
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	c, slept := newTestClient(t, srv.URL)
	if err := c.do(context.Background(), http.MethodGet, "/v1/x", nil, nil, nil); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 3 {
		t.Fatalf("expected 3 calls, got %d", calls.Load())
	}
	if len(*slept) != 2 || (*slept)[0] != 3*time.Second || (*slept)[1] != 3*time.Second {
		t.Fatalf("expected two 3s Retry-After sleeps, got %v", *slept)
	}
}

func TestRetryExponentialBackoffWithoutHeader(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) <= 3 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	c, slept := newTestClient(t, srv.URL)
	if err := c.do(context.Background(), http.MethodGet, "/v1/x", nil, nil, nil); err != nil {
		t.Fatal(err)
	}
	want := []time.Duration{1 * time.Second, 2 * time.Second, 4 * time.Second}
	if len(*slept) != len(want) {
		t.Fatalf("expected %d sleeps, got %v", len(want), *slept)
	}
	for i := range want {
		if (*slept)[i] != want[i] {
			t.Fatalf("sleep[%d] = %v, want %v", i, (*slept)[i], want[i])
		}
	}
}

func TestRetryExhaustionReturnsAPIError(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"type":"error","error":{"type":"rate_limit_error","message":"nope"}}`))
	}))
	defer srv.Close()

	c, _ := newTestClient(t, srv.URL, WithMaxRetries(2))
	err := c.do(context.Background(), http.MethodGet, "/v1/x", nil, nil, nil)
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("expected 429 APIError, got %v", err)
	}
	if calls.Load() != 3 { // initial + 2 retries
		t.Fatalf("expected 3 calls, got %d", calls.Load())
	}
}

func TestNoRetryOn4xx(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"type":"error","error":{"type":"invalid_request_error","message":"bad"}}`))
	}))
	defer srv.Close()

	c, _ := newTestClient(t, srv.URL)
	if err := c.do(context.Background(), http.MethodGet, "/v1/x", nil, nil, nil); err == nil {
		t.Fatal("expected error")
	}
	if calls.Load() != 1 {
		t.Fatalf("400 must not retry; got %d calls", calls.Load())
	}
}

func TestRetryDelayCap(t *testing.T) {
	resp := &http.Response{Header: http.Header{}}
	if d := retryDelay(resp, 10); d != 30*time.Second {
		t.Fatalf("expected 30s cap, got %v", d)
	}
}

func TestRetryDelayRetryAfterParsing(t *testing.T) {
	tests := []struct {
		name       string
		retryAfter string
		want       time.Duration
	}{
		// Malformed or negative values fall back to exponential backoff
		// (1s for attempt 0).
		{"non-numeric", "abc", 1 * time.Second},
		{"negative", "-5", 1 * time.Second},
		{"fractional", "1.5", 1 * time.Second},
		{"valid seconds", "3", 3 * time.Second},
		// A valid but enormous header must not suspend Terraform beyond
		// the backoff cap.
		{"huge value capped", "99999", backoffCap},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := &http.Response{Header: http.Header{}}
			resp.Header.Set("Retry-After", tt.retryAfter)
			if d := retryDelay(resp, 0); d != tt.want {
				t.Fatalf("retryDelay(Retry-After=%q, 0) = %v, want %v", tt.retryAfter, d, tt.want)
			}
		})
	}
}
