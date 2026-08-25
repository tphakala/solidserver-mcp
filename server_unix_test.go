//go:build !windows

package main

import (
	"context"
	"encoding/json"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"
)

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
// socket cleanly when the context is cancelled (the SIGINT/SIGTERM path). It is
// Unix-only: the 0600 model and the /tmp socket path do not apply on Windows.
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
