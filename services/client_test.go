package services

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/efficientip-labs/solidserver-go-client/sdsclient"
	"golang.org/x/time/rate"
)

func TestNewSolidServerClient_MissingCredentials(t *testing.T) {
	_, err := NewSolidServerClient("", "id", "secret", false)
	if err == nil {
		t.Error("expected error when host is missing")
	}

	_, err = NewSolidServerClient("host", "", "secret", false)
	if err == nil {
		t.Error("expected error when token id is missing")
	}

	_, err = NewSolidServerClient("host", "id", "", false)
	if err == nil {
		t.Error("expected error when token secret is missing")
	}
}

func TestNewSolidServerClient_Success(t *testing.T) {
	client, err := NewSolidServerClient("sds.local", "id", "secret", false)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if client == nil {
		t.Fatal("expected client to be non-nil")
	}
	if client.Host() != "sds.local" {
		t.Errorf("expected Host sds.local, got %s", client.Host())
	}
	if client.SSLVerify() {
		t.Errorf("expected SSLVerify false, got %v", client.SSLVerify())
	}
}

func TestNewSolidServerClientWithOptions(t *testing.T) {
	client, err := NewSolidServerClientWithOptions(ClientOptions{
		Host:        "sds.corp",
		TokenID:     "tok-id",
		TokenSecret: "tok-secret",
		SSLVerify:   true,
		HTTPTimeout: 10 * time.Second,
		MaxRetries:  2,
		RateLimit:   10,
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if client == nil {
		t.Fatal("expected client to be non-nil")
	}
	if client.Host() != "sds.corp" {
		t.Errorf("expected Host sds.corp, got %s", client.Host())
	}
	if !client.SSLVerify() {
		t.Errorf("expected SSLVerify true, got %v", client.SSLVerify())
	}
}

func TestAuthContext(t *testing.T) {
	client, err := NewSolidServerClient("sds.local", "testid", "testsecret", false)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	ctx := client.AuthContext(t.Context())
	val := ctx.Value(sdsclient.ContextEipApiTokenAuth)

	auth, ok := val.(sdsclient.EipApiTokenAuth)
	if !ok {
		t.Fatalf("expected context to contain EipApiTokenAuth, got %T", val)
	}

	if auth.Token != "testid" {
		t.Errorf("expected Token testid, got %q", auth.Token)
	}
	if auth.Secret != "testsecret" {
		t.Errorf("expected Secret testsecret, got %q", auth.Secret)
	}
}

func TestRetryTransport_Success(t *testing.T) {
	var attempts int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&attempts, 1)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	defer server.Close()

	rt := &RetryTransport{
		Base:           http.DefaultTransport,
		MaxRetries:     3,
		InitialBackoff: 10 * time.Millisecond,
	}
	client := &http.Client{Transport: rt}

	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, server.URL, http.NoBody)
	if err != nil {
		t.Fatalf("NewRequestWithContext: %v", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	_ = resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
	if atomic.LoadInt32(&attempts) != 1 {
		t.Errorf("expected 1 attempt, got %d", attempts)
	}
}

func TestRetryTransport_RetriesOn503ThenSucceeds(t *testing.T) {
	var attempts int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		att := atomic.AddInt32(&attempts, 1)
		if att < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("recovered"))
	}))
	defer server.Close()

	rt := &RetryTransport{
		Base:           http.DefaultTransport,
		MaxRetries:     3,
		InitialBackoff: 10 * time.Millisecond,
	}
	client := &http.Client{Transport: rt}

	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, server.URL, http.NoBody)
	if err != nil {
		t.Fatalf("NewRequestWithContext: %v", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	_ = resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
	if atomic.LoadInt32(&attempts) != 3 {
		t.Errorf("expected 3 attempts, got %d", attempts)
	}
}

func TestRetryTransport_NonRetryable400(t *testing.T) {
	var attempts int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&attempts, 1)
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer server.Close()

	rt := &RetryTransport{
		Base:           http.DefaultTransport,
		MaxRetries:     3,
		InitialBackoff: 10 * time.Millisecond,
	}
	client := &http.Client{Transport: rt}

	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, server.URL, http.NoBody)
	if err != nil {
		t.Fatalf("NewRequestWithContext: %v", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	_ = resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", resp.StatusCode)
	}
	if atomic.LoadInt32(&attempts) != 1 {
		t.Errorf("expected 1 attempt, got %d", attempts)
	}
}

func TestRetryTransport_RateLimit(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	limiter := rate.NewLimiter(rate.Limit(50), 5)
	rt := &RetryTransport{
		Base:    http.DefaultTransport,
		Limiter: limiter,
	}
	client := &http.Client{Transport: rt}

	for i := range 5 {
		req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, server.URL, http.NoBody)
		if err != nil {
			t.Fatalf("request %d failed: %v", i, err)
		}
		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("request %d failed: %v", i, err)
		}
		_ = resp.Body.Close()
	}
}

func TestParseRetryAfter(t *testing.T) {
	if d := parseRetryAfter(""); d != 0 {
		t.Errorf("expected 0 for empty header, got %v", d)
	}
	if d := parseRetryAfter("5"); d != 5*time.Second {
		t.Errorf("expected 5s, got %v", d)
	}
	futureTime := time.Now().Add(10 * time.Second).UTC().Format(http.TimeFormat)
	if d := parseRetryAfter(futureTime); d <= 0 || d > 11*time.Second {
		t.Errorf("expected ~10s for date, got %v", d)
	}
}

func TestDoctor_FailedHost(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()

	result := RunDoctor(ctx, nil, "nonexistent.invalid.local", true)
	if result.Healthy {
		t.Errorf("expected Healthy false for invalid host")
	}
	if len(result.Checks) == 0 {
		t.Errorf("expected at least 1 check result")
	}
	if result.Checks[0].Status != StatusFail {
		t.Errorf("expected first check to FAIL, got %s", result.Checks[0].Status)
	}
}

func TestRetryTransport_RetriesOn429WithRetryAfter(t *testing.T) {
	var attempts int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		att := atomic.AddInt32(&attempts, 1)
		if att == 1 {
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	defer server.Close()

	rt := &RetryTransport{
		Base:           http.DefaultTransport,
		MaxRetries:     2,
		InitialBackoff: 5 * time.Millisecond,
	}
	client := &http.Client{Transport: rt}

	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, server.URL, http.NoBody)
	if err != nil {
		t.Fatalf("NewRequestWithContext: %v", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	_ = resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}
	if atomic.LoadInt32(&attempts) != 2 {
		t.Errorf("expected 2 attempts, got %d", attempts)
	}
}

func TestRetryTransport_ContextCancelledDuringBackoff(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	rt := &RetryTransport{
		Base:           http.DefaultTransport,
		MaxRetries:     3,
		InitialBackoff: 5 * time.Second, // Long backoff to test cancellation
	}
	client := &http.Client{Transport: rt}

	ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL, http.NoBody)
	if err != nil {
		t.Fatalf("NewRequestWithContext: %v", err)
	}
	resp, err := client.Do(req)
	if err == nil {
		if resp != nil && resp.Body != nil {
			_ = resp.Body.Close()
		}
		t.Fatal("expected error due to cancelled context, got nil")
	}
}

func TestRetryTransport_ExhaustsRetries(t *testing.T) {
	var attempts int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&attempts, 1)
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	rt := &RetryTransport{
		Base:           http.DefaultTransport,
		MaxRetries:     2,
		InitialBackoff: 5 * time.Millisecond,
	}
	client := &http.Client{Transport: rt}

	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, server.URL, http.NoBody)
	if err != nil {
		t.Fatalf("NewRequestWithContext: %v", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("expected response with 503, got error: %v", err)
	}
	_ = resp.Body.Close()

	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("expected status 503, got %d", resp.StatusCode)
	}
	if atomic.LoadInt32(&attempts) != 3 {
		t.Errorf("expected 3 total attempts (1 initial + 2 retries), got %d", attempts)
	}
}
