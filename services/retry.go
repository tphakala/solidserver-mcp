package services

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"net"
	"net/http"
	"strconv"
	"strings"
	"syscall"
	"time"

	"golang.org/x/time/rate"
)

const (
	defaultInitialBackoff = 200 * time.Millisecond
	defaultMaxBackoff     = 10 * time.Second
	maxRequestBodyBytes   = 10 * 1024 * 1024 // 10 MB
	maxBackoffShift       = 10
	jitterDivisor         = 4
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
	if errors.Is(err, io.EOF) ||
		errors.Is(err, io.ErrUnexpectedEOF) ||
		errors.Is(err, syscall.ECONNRESET) ||
		errors.Is(err, syscall.ECONNREFUSED) ||
		errors.Is(err, syscall.EPIPE) {
		return true
	}
	errStr := strings.ToLower(err.Error())
	return strings.Contains(errStr, "connection reset") ||
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
	if resp != nil && (resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode == http.StatusServiceUnavailable) {
		backoff = parseRetryAfter(resp.Header.Get("Retry-After"))
	}
	if backoff <= 0 {
		shift := max(0, min(attempt-1, maxBackoffShift))
		multiplier := 1 << shift
		backoff = initialBackoff * time.Duration(multiplier)
		backoff = min(backoff, maxBackoff)
		jitterBound := max(1, int64(backoff/jitterDivisor))
		jitter := time.Duration(rand.Int64N(jitterBound))
		backoff += jitter
	}
	return min(backoff, maxBackoff)
}

func (t *RetryTransport) readBody(req *http.Request) ([]byte, error) {
	if req.Body == nil || req.Body == http.NoBody {
		return nil, nil
	}
	var buf bytes.Buffer
	n, err := io.Copy(&buf, io.LimitReader(req.Body, maxRequestBodyBytes+1))
	_ = req.Body.Close()
	if err != nil {
		return nil, err
	}
	if n > maxRequestBodyBytes {
		return nil, fmt.Errorf("request body exceeds maximum allowed size of %d bytes", maxRequestBodyBytes)
	}
	return buf.Bytes(), nil
}

func cloneRequest(req *http.Request, bodyBytes []byte) *http.Request {
	reqAttempt := req.Clone(req.Context())
	if bodyBytes != nil {
		reqAttempt.Body = io.NopCloser(bytes.NewReader(bodyBytes))
		reqAttempt.ContentLength = int64(len(bodyBytes))
		reqAttempt.GetBody = func() (io.ReadCloser, error) {
			return io.NopCloser(bytes.NewReader(bodyBytes)), nil
		}
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

type retryOptions struct {
	base           http.RoundTripper
	initialBackoff time.Duration
	maxBackoff     time.Duration
	maxAttempts    int
}

func (t *RetryTransport) resolveOptions() retryOptions {
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
	return retryOptions{
		base:           base,
		initialBackoff: initialBackoff,
		maxBackoff:     maxBackoff,
		maxAttempts:    max(1, t.MaxRetries+1),
	}
}

// RoundTrip executes a single HTTP transaction with rate limiting and retry handling.
func (t *RetryTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	opts := t.resolveOptions()
	bodyBytes, err := t.readBody(req)
	if err != nil {
		return nil, err
	}

	var lastErr error
	var resp *http.Response

	for attempt := range opts.maxAttempts {
		if err := waitRetry(req.Context(), attempt, resp, opts.initialBackoff, opts.maxBackoff); err != nil {
			return nil, err
		}

		if t.Limiter != nil {
			if err := t.Limiter.Wait(req.Context()); err != nil {
				return nil, err
			}
		}

		resp, lastErr = opts.base.RoundTrip(cloneRequest(req, bodyBytes))
		if lastErr != nil {
			if !isRetryableNetworkError(lastErr) || attempt == opts.maxAttempts-1 {
				return nil, lastErr
			}
			continue
		}

		if !isRetryableStatusCode(resp.StatusCode) || attempt == opts.maxAttempts-1 {
			return resp, nil
		}
	}

	if lastErr != nil {
		return nil, lastErr
	}
	return resp, nil
}
