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

	return newHTTPMux(client, slog.New(slog.DiscardHandler))
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
}
