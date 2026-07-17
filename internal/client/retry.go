package client

import (
	"math"
	"net/http"
	"strconv"
	"time"
)

const (
	backoffBase = 1 * time.Second
	backoffCap  = 30 * time.Second
)

// shouldRetry reports whether the HTTP status is worth retrying: rate
// limits (429) and transient server errors (5xx).
func shouldRetry(status int) bool {
	return status == http.StatusTooManyRequests || status >= 500
}

// retryDelay honors a Retry-After header (in seconds) when present and
// otherwise falls back to capped exponential backoff.
func retryDelay(resp *http.Response, attempt int) time.Duration {
	if s := resp.Header.Get("Retry-After"); s != "" {
		if secs, err := strconv.Atoi(s); err == nil && secs >= 0 {
			d := time.Duration(secs) * time.Second
			if d > backoffCap {
				return backoffCap
			}
			return d
		}
	}
	d := time.Duration(float64(backoffBase) * math.Pow(2, float64(attempt)))
	if d > backoffCap || d <= 0 {
		return backoffCap
	}
	return d
}
