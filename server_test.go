package main

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

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

// The Unix socket transport end-to-end test lives in server_unix_test.go, which
// is built only on non-Windows platforms.
