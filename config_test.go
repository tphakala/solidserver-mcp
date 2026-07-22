package main

import (
	"testing"
)

const testHost = "sds.example.com"

func TestLoadConfig_Valid(t *testing.T) {
	t.Setenv(envHost, testHost)
	t.Setenv(envTokenID, "token-id")
	t.Setenv(envTokenSecret, "token-secret")
	t.Setenv(envTransport, TransportHTTP)
	t.Setenv(envLogLevel, LogLevelDebug)
	t.Setenv(envHTTPHost, "127.0.0.1")
	t.Setenv(envSSLVerify, "false")
	t.Setenv(envHTTPPort, "9090")

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if cfg.Host != testHost {
		t.Errorf("expected Host %s, got %q", testHost, cfg.Host)
	}
	if cfg.TokenID != "token-id" {
		t.Errorf("expected TokenID token-id, got %q", cfg.TokenID)
	}
	if cfg.TokenSecret != "token-secret" {
		t.Errorf("expected TokenSecret token-secret, got %q", cfg.TokenSecret)
	}
	if cfg.Transport != TransportHTTP {
		t.Errorf("expected Transport %s, got %q", TransportHTTP, cfg.Transport)
	}
	if cfg.LogLevel != LogLevelDebug {
		t.Errorf("expected LogLevel %s, got %q", LogLevelDebug, cfg.LogLevel)
	}
	if cfg.HTTPHost != "127.0.0.1" {
		t.Errorf("expected HTTPHost 127.0.0.1, got %q", cfg.HTTPHost)
	}
	if cfg.SSLVerify {
		t.Errorf("expected SSLVerify false, got %v", cfg.SSLVerify)
	}
	if cfg.HTTPPort != 9090 {
		t.Errorf("expected HTTPPort 9090, got %d", cfg.HTTPPort)
	}
}

func TestLoadConfig_Defaults(t *testing.T) {
	t.Setenv(envHost, testHost)
	t.Setenv(envTokenID, "token-id")
	t.Setenv(envTokenSecret, "token-secret")
	// Unset optional ones to trigger defaults
	t.Setenv(envTransport, "")
	t.Setenv(envLogLevel, "")
	t.Setenv(envHTTPHost, "")
	t.Setenv(envSSLVerify, "")
	t.Setenv(envHTTPPort, "")

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if cfg.Transport != TransportStdio {
		t.Errorf("expected default Transport %s, got %q", TransportStdio, cfg.Transport)
	}
	if cfg.LogLevel != LogLevelInfo {
		t.Errorf("expected default LogLevel %s, got %q", LogLevelInfo, cfg.LogLevel)
	}
	if cfg.HTTPHost != defaultHTTPHost {
		t.Errorf("expected default HTTPHost %s, got %q", defaultHTTPHost, cfg.HTTPHost)
	}
	if !cfg.SSLVerify {
		t.Errorf("expected default SSLVerify true, got %v", cfg.SSLVerify)
	}
	if cfg.HTTPPort != defaultHTTPPort {
		t.Errorf("expected default HTTPPort %d, got %d", defaultHTTPPort, cfg.HTTPPort)
	}
}

// TestLoadConfig_InvalidValues covers malformed optional variables. These used
// to fall back to a default silently, which hid typos: SOLIDSERVER_SSL_VERIFY=fasle
// looked like an opt-out of TLS verification but was ignored, and an
// out-of-range MCP_HTTP_PORT was accepted and only failed later at listen time.
func TestLoadConfig_InvalidValues(t *testing.T) {
	tests := []struct {
		name    string
		env     string
		value   string
		wantErr string
	}{
		{"unknown transport", envTransport, "grpc", `invalid MCP_TRANSPORT "grpc": expected "stdio" or "http"`},
		{"unknown log level", envLogLevel, "trace", `invalid LOG_LEVEL "trace": expected "debug", "info", "warn" or "error"`},
		{"non-boolean ssl verify", envSSLVerify, "fasle", `invalid SOLIDSERVER_SSL_VERIFY "fasle": expected a boolean`},
		{"non-numeric port", envHTTPPort, "http", `invalid MCP_HTTP_PORT "http": expected an integer`},
		{"port above range", envHTTPPort, "99999", "invalid MCP_HTTP_PORT 99999: expected 1-65535"},
		{"port below range", envHTTPPort, "0", "invalid MCP_HTTP_PORT 0: expected 1-65535"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(envHost, testHost)
			t.Setenv(envTokenID, "token-id")
			t.Setenv(envTokenSecret, "token-secret")
			// Clear the other optional variables so a malformed value
			// inherited from the test process cannot trip an earlier check
			// and mask the one under test.
			for _, optional := range []string{envTransport, envLogLevel, envHTTPHost, envSSLVerify, envHTTPPort} {
				t.Setenv(optional, "")
			}
			t.Setenv(tt.env, tt.value)

			_, err := LoadConfig()
			if err == nil {
				t.Fatalf("expected error for %s=%q, got nil", tt.env, tt.value)
			}
			if err.Error() != tt.wantErr {
				t.Errorf("expected error %q, got %q", tt.wantErr, err.Error())
			}
		})
	}
}

func TestLoadConfig_MissingRequired(t *testing.T) {
	tests := []struct {
		name        string
		host        string
		tokenID     string
		tokenSecret string
		wantErr     string
	}{
		{"missing host", "", "id", "secret", "SOLIDSERVER_HOST environment variable is required"},
		{"missing token id", testHost, "", "secret", "SOLIDSERVER_TOKEN_ID environment variable is required"},
		{"missing token secret", testHost, "id", "", "SOLIDSERVER_TOKEN_SECRET environment variable is required"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(envHost, tt.host)
			t.Setenv(envTokenID, tt.tokenID)
			t.Setenv(envTokenSecret, tt.tokenSecret)

			_, err := LoadConfig()
			if err == nil {
				t.Fatalf("expected error, got nil")
			}
			if err.Error() != tt.wantErr {
				t.Errorf("expected error %q, got %q", tt.wantErr, err.Error())
			}
		})
	}
}
