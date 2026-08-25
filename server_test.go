package main

import (
	"context"
	"encoding/json"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tphakala/solidserver-mcp/services"
)

// newTestMux builds the HTTP mux with a throwaway client and a silent logger.
func newTestMux(t *testing.T) *http.ServeMux {
	t.Helper()

	client, err := services.NewSolidServerClient("solidserver.invalid", "token-id", "token-secret", true)
	if err != nil {
		t.Fatalf("NewSolidServerClient: %v", err)
	}

	return newHTTPMux(client, slog.New(slog.DiscardHandler), nil, TransportHTTP)
}

// TestMCPEndpointRejectsCrossOriginPost guards the CSRF protection on /mcp.
// The MCP SDK enabled a zero-value CrossOriginProtection by default up to
// v1.5.0 and removed that default in v1.6.0, so the protection now lives in
// newHTTPMux. Without this test an SDK bump silently drops the check.
func TestMCPEndpointRejectsCrossOriginPost(t *testing.T) {
	mux := newTestMux(t)

	tests := []struct {
		name    string
		headers map[string]string
	}{
		{
			name:    "cross-site Sec-Fetch-Site",
			headers: map[string]string{"Sec-Fetch-Site": "cross-site"},
		},
		{
			name:    "mismatched Origin",
			headers: map[string]string{"Origin": "https://attacker.example"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"ping"}`))
			req.Host = "mcp.example"
			req.Header.Set("Content-Type", "application/json")
			for k, v := range tt.headers {
				req.Header.Set(k, v)
			}

			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, req)

			if rec.Code != http.StatusForbidden {
				t.Errorf("cross-origin POST to /mcp: got status %d, want %d", rec.Code, http.StatusForbidden)
			}
		})
	}
}

// TestMCPEndpointAllowsSameOriginPost ensures the CSRF middleware does not
// reject legitimate same-origin and non-browser requests.
func TestMCPEndpointAllowsSameOriginPost(t *testing.T) {
	mux := newTestMux(t)

	tests := []struct {
		name    string
		headers map[string]string
	}{
		{
			name:    "same-origin Sec-Fetch-Site",
			headers: map[string]string{"Sec-Fetch-Site": "same-origin"},
		},
		{
			name:    "matching Origin",
			headers: map[string]string{"Origin": "https://mcp.example"},
		},
		{
			name:    "no browser headers",
			headers: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"ping"}`))
			req.Host = "mcp.example"
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Accept", "application/json, text/event-stream")
			for k, v := range tt.headers {
				req.Header.Set(k, v)
			}

			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, req)

			if rec.Code != http.StatusOK {
				t.Fatalf("same-origin POST to /mcp: got status %d, want %d (body %q)", rec.Code, http.StatusOK, rec.Body.String())
			}
			// Assert on the JSON-RPC reply rather than just the status, so an
			// unregistered or renamed /mcp route fails here instead of passing
			// on a 404 that merely is not a 403.
			if body := rec.Body.String(); !strings.Contains(body, `"result"`) {
				t.Errorf("same-origin POST to /mcp: got body %q, want a JSON-RPC result", body)
			}
		})
	}
}

// TestHealthEndpoint checks the health route stays reachable.
func TestHealthEndpoint(t *testing.T) {
	mux := newTestMux(t)

	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/health", http.NoBody)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /health: got status %d, want %d", rec.Code, http.StatusOK)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("GET /health: got Content-Type %q, want %q", ct, "application/json")
	}
	if body := rec.Body.String(); !strings.Contains(body, `"status":"ok"`) {
		t.Errorf("GET /health: expected status ok in body %q", body)
	}
}

// TestReadyEndpoint verifies the /ready probe behavior.
func TestReadyEndpoint(t *testing.T) {
	t.Run("ready endpoint with failing upstream", func(t *testing.T) {
		mux := newTestMux(t)
		req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/ready", http.NoBody)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		// With invalid host "solidserver.invalid", upstream probe will fail with 503
		if rec.Code != http.StatusServiceUnavailable {
			t.Fatalf("GET /ready: got status %d, want %d", rec.Code, http.StatusServiceUnavailable)
		}
		if body := rec.Body.String(); !strings.Contains(body, `"status":"unavailable"`) {
			t.Errorf("GET /ready: expected unavailable status in body %q", body)
		}
	})

	t.Run("ready endpoint with healthy mock upstream", func(t *testing.T) {
		server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"data":[]}`))
		}))
		defer server.Close()

		host := strings.TrimPrefix(server.URL, "https://")
		client, err := services.NewSolidServerClient(host, "id", "secret", false)
		if err != nil {
			t.Fatalf("NewSolidServerClient: %v", err)
		}

		mux := newHTTPMux(client, slog.New(slog.DiscardHandler), nil, TransportHTTP)
		req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/ready", http.NoBody)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("GET /ready: got status %d, want %d (body: %s)", rec.Code, http.StatusOK, rec.Body.String())
		}
		if body := rec.Body.String(); !strings.Contains(body, `"status":"ready"`) {
			t.Errorf("GET /ready: expected ready status in body %q", body)
		}
	})
}

// waitForSocket polls until path exists as a socket or the deadline passes.
func waitForSocket(t *testing.T, path string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if info, err := os.Stat(path); err == nil && info.Mode()&os.ModeSocket != 0 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("socket %q did not appear before the deadline", path)
}

// TestRunUnixTransport exercises the Unix socket transport end to end: it binds
// a socket with 0600 permissions, serves /health over it, and unlinks the
// socket cleanly when the context is cancelled (the SIGINT/SIGTERM path).
func TestRunUnixTransport(t *testing.T) {
	// A short base dir keeps the socket path under the sun_path length limit,
	// which t.TempDir() can blow on macOS (its per-test path is long).
	//nolint:usetesting,gocritic // t.TempDir() on macOS exceeds the 104-byte sun_path limit; a short /tmp dir is required
	dir, err := os.MkdirTemp("/tmp", "sds")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	sock := filepath.Join(dir, "s.sock")

	cfg := &Config{
		Transport:   TransportUnix,
		Host:        "solidserver.invalid",
		TokenID:     "token-id",
		TokenSecret: "token-secret",
		SocketPath:  sock,
	}

	errCh := make(chan error, 1)
	ctx, cancel := context.WithCancel(t.Context())
	go func() { errCh <- runUnix(ctx, cfg, slog.New(slog.DiscardHandler)) }()

	waitForSocket(t, sock)

	info, err := os.Stat(sock)
	if err != nil {
		t.Fatalf("stat socket: %v", err)
	}
	if info.Mode()&os.ModeSocket == 0 {
		t.Fatalf("expected a socket, got mode %v", info.Mode())
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("socket permissions = %o, want 600", perm)
	}

	httpClient := &http.Client{
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				return (&net.Dialer{}).DialContext(ctx, "unix", sock)
			},
		},
	}
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "http://unix/health", http.NoBody)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		t.Fatalf("GET /health over socket: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /health over socket: status = %d, want 200", resp.StatusCode)
	}
	var body map[string]string
	if decErr := json.NewDecoder(resp.Body).Decode(&body); decErr != nil {
		t.Fatalf("decode /health body: %v", decErr)
	}
	if body[jsonKeyTransport] != TransportUnix {
		t.Errorf("health transport = %q, want %q", body[jsonKeyTransport], TransportUnix)
	}

	cancel()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("runUnix returned error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("runUnix did not return after context cancel")
	}

	if _, err := os.Stat(sock); !os.IsNotExist(err) {
		t.Errorf("socket was not unlinked on shutdown: stat err = %v", err)
	}
}
