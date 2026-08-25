//go:build !windows

package main

import (
	"strings"
	"testing"
)

func setRequiredEnv(t *testing.T) {
	t.Helper()
	t.Setenv(envHost, testHost)
	t.Setenv(envTokenID, "token-id")
	t.Setenv(envTokenSecret, "token-secret")
	t.Setenv(envTokenIDFile, "")
	t.Setenv(envTokenSecretFile, "")
}

// TestLoadConfig_UnixTransport is Unix-only: it asserts Unix-style absolute
// socket paths and the /-separated default under $XDG_RUNTIME_DIR, neither of
// which maps onto Windows path semantics.
func TestLoadConfig_UnixTransport(t *testing.T) {
	t.Run("explicit socket path", func(t *testing.T) {
		setRequiredEnv(t)
		t.Setenv(envTransport, TransportUnix)
		t.Setenv(envSocket, "/tmp/solidserver-test.sock")
		cfg, err := LoadConfig()
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if cfg.Transport != TransportUnix {
			t.Errorf("expected transport %q, got %q", TransportUnix, cfg.Transport)
		}
		if cfg.SocketPath != "/tmp/solidserver-test.sock" {
			t.Errorf("expected explicit socket path, got %q", cfg.SocketPath)
		}
	})

	t.Run("defaults under runtime dir", func(t *testing.T) {
		setRequiredEnv(t)
		t.Setenv(envTransport, TransportUnix)
		t.Setenv(envSocket, "")
		t.Setenv(envXDGRuntimeDir, "/run/user/1000")
		cfg, err := LoadConfig()
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if want := "/run/user/1000/" + defaultSocketName; cfg.SocketPath != want {
			t.Errorf("expected default socket %q, got %q", want, cfg.SocketPath)
		}
	})

	t.Run("rejects a relative socket path", func(t *testing.T) {
		setRequiredEnv(t)
		t.Setenv(envTransport, TransportUnix)
		t.Setenv(envSocket, "relative.sock")
		_, err := LoadConfig()
		if err == nil || !strings.Contains(err.Error(), "must be absolute") {
			t.Fatalf("expected an absolute-path error, got %v", err)
		}
	})

	t.Run("rejects an over-long socket path", func(t *testing.T) {
		setRequiredEnv(t)
		t.Setenv(envTransport, TransportUnix)
		t.Setenv(envSocket, "/tmp/"+strings.Repeat("a", 120)+".sock")
		_, err := LoadConfig()
		if err == nil || !strings.Contains(err.Error(), "platform limit") {
			t.Fatalf("expected a length-limit error, got %v", err)
		}
	})
}
