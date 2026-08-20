package services

import (
	"bytes"
	"context"
	"errors"
	"io"
	"math/rand/v2"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"golang.org/x/time/rate"
)

const (
	defaultInitialBackoff = 200 * time.Millisecond
	defaultMaxBackoff     = 10 * time.Second
	maxRequestBodyBytes   = 10 * 1024 * 1024 // 10 MB
)

// RetryTransport wraps an http.RoundTripper with rate limiting, timeouts, and bounded retries with backoff.
type RetryTransport struct {
	Base           http.RoundTripper
	MaxRetries     int
	InitialBackoff time.Duration
	MaxBackoff     time.Duration
	Limiter        *rate.Limiter
}

func isRetryableStatusCode(code int) bool {
	switch code {
	case http.StatusTooManyRequests,
		http.StatusBadGateway,
		http.StatusServiceUnavailable,
		http.StatusGatewayTimeout:
		return true
	default:
		return false
	}
}

func isRetryableNetworkError(err error) bool {
	if err == nil {
		return false
	}
	if netErr, ok := errors.AsType[net.Error](err); ok && netErr.Timeout() {
		return true
	}
	errStr := strings.ToLower(err.Error())
	return errors.Is(err, io.EOF) ||
		errors.Is(err, io.ErrUnexpectedEOF) ||
		strings.Contains(errStr, "connection reset") ||
		strings.Contains(errStr, "connection refused") ||
		strings.Contains(errStr, "broken pipe") ||
		strings.Contains(errStr, "temporary failure")
}

func parseRetryAfter(header string) time.Duration {
	if header == "" {
		return 0
	}
	if secs, err := strconv.Atoi(header); err == nil && secs > 0 {
		return time.Duration(secs) * time.Second
	}
	if t, err := http.ParseTime(header); err == nil {
		d := time.Until(t)
		if d > 0 {
			return d
		}
	}
	return 0
}

func calculateBackoff(attempt int, resp *http.Response, initialBackoff, maxBackoff time.Duration) time.Duration {
	var backoff time.Duration
	if resp != nil && resp.StatusCode == http.StatusTooManyRequests {
		backoff = parseRetryAfter(resp.Header.Get("Retry-After"))
	}
	if backoff <= 0 {
		multiplier := 1 << (attempt - 1)
		backoff = initialBackoff * time.Duration(multiplier)
		jitter := time.Duration(rand.Int64N(int64(backoff/4 + 1)))
		backoff += jitter
	}
	if backoff > maxBackoff {
		backoff = maxBackoff
	}
	return backoff
}

func (t *RetryTransport) readBody(req *http.Request) ([]byte, error) {
	if req.Body == nil {
		return nil, nil
	}
	bodyBytes, err := io.ReadAll(io.LimitReader(req.Body, maxRequestBodyBytes))
	_ = req.Body.Close()
	if err != nil {
		return nil, err
	}
	return bodyBytes, nil
}

func cloneRequest(req *http.Request, bodyBytes []byte) *http.Request {
	reqAttempt := req.Clone(req.Context())
	if bodyBytes != nil {
		reqAttempt.Body = io.NopCloser(bytes.NewReader(bodyBytes))
	}
	return reqAttempt
}

func waitRetry(ctx context.Context, attempt int, resp *http.Response, initialBackoff, maxBackoff time.Duration) error {
	if attempt == 0 {
		return nil
	}
	backoff := calculateBackoff(attempt, resp, initialBackoff, maxBackoff)
	if resp != nil && resp.Body != nil {
		_ = resp.Body.Close()
	}
	timer := time.NewTimer(backoff)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// RoundTrip executes a single HTTP transaction with rate limiting and retry handling.
func (t *RetryTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if t.Limiter != nil {
		if err := t.Limiter.Wait(req.Context()); err != nil {
			return nil, err
		}
	}

	base := t.Base
	if base == nil {
		base = http.DefaultTransport
	}

	initialBackoff := t.InitialBackoff
	if initialBackoff <= 0 {
		initialBackoff = defaultInitialBackoff
	}
	maxBackoff := t.MaxBackoff
	if maxBackoff <= 0 {
		maxBackoff = defaultMaxBackoff
	}

	bodyBytes, err := t.readBody(req)
	if err != nil {
		return nil, err
	}

	maxAttempts := t.MaxRetries + 1
	var lastErr error
	var resp *http.Response

	for attempt := range maxAttempts {
		if err := waitRetry(req.Context(), attempt, resp, initialBackoff, maxBackoff); err != nil {
			return nil, err
		}

		resp, lastErr = base.RoundTrip(cloneRequest(req, bodyBytes))
		if lastErr != nil {
			if !isRetryableNetworkError(lastErr) || attempt == maxAttempts-1 {
				return nil, lastErr
			}
			continue
		}

		if !isRetryableStatusCode(resp.StatusCode) || attempt == maxAttempts-1 {
			return resp, nil
		}
	}

	if lastErr != nil {
		return nil, lastErr
	}
	return resp, nil
}
