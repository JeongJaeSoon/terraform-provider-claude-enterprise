// Package client is a minimal HTTP client for the Claude Enterprise Admin
// API (spend limits endpoints).
package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"golang.org/x/time/rate"
)

const (
	// DefaultBaseURL is the production Admin API endpoint.
	DefaultBaseURL = "https://api.anthropic.com"

	anthropicVersion = "2023-06-01"

	defaultMaxRetries = 5
	// The API allows 60 requests/minute per organization; stay under it by
	// default so Terraform's parallelism doesn't trip the limit.
	defaultRequestsPerMinute = 50
)

// Client talks to the Claude Enterprise Admin API.
type Client struct {
	baseURL    *url.URL
	apiKey     string
	httpClient *http.Client
	limiter    *rate.Limiter
	maxRetries int
	// sleep is indirected so retry tests don't wait in real time.
	sleep func(context.Context, time.Duration) error
}

// Option customizes a Client.
type Option func(*Client)

// WithHTTPClient replaces the underlying *http.Client.
func WithHTTPClient(h *http.Client) Option {
	return func(c *Client) { c.httpClient = h }
}

// WithMaxRetries sets how many times a 429/5xx response is retried.
func WithMaxRetries(n int) Option {
	return func(c *Client) { c.maxRetries = n }
}

// WithRequestsPerMinute adjusts the client-side rate limit.
func WithRequestsPerMinute(n int) Option {
	return func(c *Client) {
		c.limiter = rate.NewLimiter(rate.Limit(float64(n)/60.0), 1)
	}
}

// New builds a Client. apiKey is required; baseURL falls back to
// DefaultBaseURL when empty.
func New(baseURL, apiKey string, opts ...Option) (*Client, error) {
	if apiKey == "" {
		return nil, errors.New("admin API key is required")
	}
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	u, err := url.Parse(baseURL)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return nil, fmt.Errorf("invalid base URL %q", baseURL)
	}
	c := &Client{
		baseURL:    u,
		apiKey:     apiKey,
		httpClient: &http.Client{Timeout: 60 * time.Second},
		limiter:    rate.NewLimiter(rate.Limit(defaultRequestsPerMinute/60.0), 1),
		maxRetries: defaultMaxRetries,
		sleep:      sleepCtx,
	}
	for _, opt := range opts {
		opt(c)
	}
	return c, nil
}

// APIError is a non-2xx response from the Admin API.
type APIError struct {
	StatusCode int
	Type       string
	Message    string
	RequestID  string
}

func (e *APIError) Error() string {
	msg := e.Message
	if msg == "" {
		msg = http.StatusText(e.StatusCode)
	}
	s := fmt.Sprintf("claude enterprise admin API error (HTTP %d, %s): %s", e.StatusCode, e.Type, msg)
	if e.RequestID != "" {
		s += fmt.Sprintf(" (request_id: %s)", e.RequestID)
	}
	return s
}

// IsNotFound reports whether err is an APIError with HTTP 404.
func IsNotFound(err error) bool {
	var apiErr *APIError
	return errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusNotFound
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// do executes one API call with auth headers, rate limiting, retries on
// 429/5xx, and JSON (de)serialization.
func (c *Client) do(ctx context.Context, method, path string, query url.Values, body, out any) error {
	var payload []byte
	if body != nil {
		var err error
		payload, err = json.Marshal(body)
		if err != nil {
			return fmt.Errorf("encoding request body: %w", err)
		}
	}

	u := *c.baseURL
	u.Path = strings.TrimSuffix(u.Path, "/") + path
	if query != nil {
		u.RawQuery = query.Encode()
	}

	for attempt := 0; ; attempt++ {
		if err := c.limiter.Wait(ctx); err != nil {
			return err
		}

		var reqBody io.Reader
		if payload != nil {
			reqBody = bytes.NewReader(payload)
		}
		req, err := http.NewRequestWithContext(ctx, method, u.String(), reqBody)
		if err != nil {
			return err
		}
		req.Header.Set("x-api-key", c.apiKey)
		req.Header.Set("anthropic-version", anthropicVersion)
		if payload != nil {
			req.Header.Set("content-type", "application/json")
		}

		resp, err := c.httpClient.Do(req)
		if err != nil {
			return err
		}
		respBody, err := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if err != nil {
			return fmt.Errorf("reading response body: %w", err)
		}

		if shouldRetry(resp.StatusCode) && attempt < c.maxRetries {
			if err := c.sleep(ctx, retryDelay(resp, attempt)); err != nil {
				return err
			}
			continue
		}
		if resp.StatusCode >= 400 {
			return parseAPIError(resp.StatusCode, respBody)
		}
		if out != nil && len(respBody) > 0 {
			if err := json.Unmarshal(respBody, out); err != nil {
				return fmt.Errorf("decoding response: %w", err)
			}
		}
		return nil
	}
}

func parseAPIError(status int, body []byte) error {
	apiErr := &APIError{StatusCode: status}
	var envelope struct {
		Error struct {
			Type    string `json:"type"`
			Message string `json:"message"`
		} `json:"error"`
		RequestID string `json:"request_id"`
	}
	if err := json.Unmarshal(body, &envelope); err == nil {
		apiErr.Type = envelope.Error.Type
		apiErr.Message = envelope.Error.Message
		apiErr.RequestID = envelope.RequestID
	}
	if apiErr.Message == "" {
		apiErr.Message = strings.TrimSpace(string(body))
	}
	return apiErr
}

// Temporary stubs; replaced by retry.go in the next task.
func shouldRetry(status int) bool { return status == http.StatusTooManyRequests || status >= 500 }

func retryDelay(_ *http.Response, _ int) time.Duration { return 0 }
