package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
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
	t.Setenv(envReadOnly, "true")
	t.Setenv(envProtectedSpaces, "prod-space, corp-space")
	t.Setenv(envProtectedZones, "corp.internal, prod.example.com")
	t.Setenv(envProtectedSubnets, "10.0.0.0/8, 192.168.1.0/24")
	t.Setenv(envHTTPTimeout, "45s")
	t.Setenv(envMaxRetries, "5")
	t.Setenv(envRateLimit, "25.5")
	t.Setenv(envLogRedactPII, "true")
	t.Setenv(envTokenIDFile, "")
	t.Setenv(envTokenSecretFile, "")

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	assertBasicConfig(t, &cfg)
	assertProtectedConfig(t, &cfg)
	assertResilienceConfig(t, &cfg)
}

func assertBasicConfig(t *testing.T, cfg *Config) {
	t.Helper()
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
	if cfg.HTTPHost != "127.0.0.1" || cfg.SSLVerify || cfg.HTTPPort != 9090 {
		t.Errorf("unexpected HTTP config: %+v", cfg)
	}
}

func assertProtectedConfig(t *testing.T, cfg *Config) {
	t.Helper()
	if !cfg.ReadOnly {
		t.Errorf("expected ReadOnly true, got %v", cfg.ReadOnly)
	}
	if len(cfg.ProtectedSpaces) != 2 || cfg.ProtectedSpaces[0] != "prod-space" || cfg.ProtectedSpaces[1] != "corp-space" {
		t.Errorf("expected ProtectedSpaces [prod-space corp-space], got %v", cfg.ProtectedSpaces)
	}
	if len(cfg.ProtectedZones) != 2 || cfg.ProtectedZones[0] != "corp.internal" || cfg.ProtectedZones[1] != "prod.example.com" {
		t.Errorf("expected ProtectedZones [corp.internal prod.example.com], got %v", cfg.ProtectedZones)
	}
	if len(cfg.ProtectedSubnets) != 2 || cfg.ProtectedSubnets[0] != "10.0.0.0/8" || cfg.ProtectedSubnets[1] != "192.168.1.0/24" {
		t.Errorf("expected ProtectedSubnets [10.0.0.0/8 192.168.1.0/24], got %v", cfg.ProtectedSubnets)
	}
}

func assertResilienceConfig(t *testing.T, cfg *Config) {
	t.Helper()
	if cfg.HTTPTimeout != 45*time.Second {
		t.Errorf("expected HTTPTimeout 45s, got %v", cfg.HTTPTimeout)
	}
	if cfg.MaxRetries != 5 {
		t.Errorf("expected MaxRetries 5, got %d", cfg.MaxRetries)
	}
	if cfg.RateLimit != 25.5 {
		t.Errorf("expected RateLimit 25.5, got %f", cfg.RateLimit)
	}
	if !cfg.LogRedactPII {
		t.Errorf("expected LogRedactPII true, got %v", cfg.LogRedactPII)
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
	t.Setenv(envReadOnly, "")
	t.Setenv(envProtectedSpaces, "")
	t.Setenv(envProtectedZones, "")
	t.Setenv(envProtectedSubnets, "")
	t.Setenv(envHTTPTimeout, "")
	t.Setenv(envMaxRetries, "")
	t.Setenv(envRateLimit, "")
	t.Setenv(envLogRedactPII, "")
	t.Setenv(envTokenIDFile, "")
	t.Setenv(envTokenSecretFile, "")

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
	if cfg.ReadOnly {
		t.Errorf("expected default ReadOnly false, got %v", cfg.ReadOnly)
	}
	if len(cfg.ProtectedSpaces) != 0 {
		t.Errorf("expected empty ProtectedSpaces, got %v", cfg.ProtectedSpaces)
	}
	if len(cfg.ProtectedZones) != 0 {
		t.Errorf("expected empty ProtectedZones, got %v", cfg.ProtectedZones)
	}
	if len(cfg.ProtectedSubnets) != 0 {
		t.Errorf("expected empty ProtectedSubnets, got %v", cfg.ProtectedSubnets)
	}
	if cfg.HTTPTimeout != defaultHTTPTimeout {
		t.Errorf("expected default HTTPTimeout %v, got %v", defaultHTTPTimeout, cfg.HTTPTimeout)
	}
	if cfg.MaxRetries != defaultMaxRetries {
		t.Errorf("expected default MaxRetries %d, got %d", defaultMaxRetries, cfg.MaxRetries)
	}
	if cfg.RateLimit != 0 {
		t.Errorf("expected default RateLimit 0, got %f", cfg.RateLimit)
	}
	if cfg.LogRedactPII {
		t.Errorf("expected default LogRedactPII false, got %v", cfg.LogRedactPII)
	}
}

func TestLoadConfig_FileSecrets(t *testing.T) {
	tempDir := t.TempDir()
	idFile := filepath.Join(tempDir, "token_id.txt")
	secretFile := filepath.Join(tempDir, "token_secret.txt")

	if err := os.WriteFile(idFile, []byte("file-token-id\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(secretFile, []byte("  file-token-secret  \n"), 0o600); err != nil {
		t.Fatal(err)
	}

	t.Setenv(envHost, testHost)
	t.Setenv(envTokenID, "ignored-id")
	t.Setenv(envTokenSecret, "ignored-secret")
	t.Setenv(envTokenIDFile, idFile)
	t.Setenv(envTokenSecretFile, secretFile)

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if cfg.TokenID != "file-token-id" {
		t.Errorf("expected TokenID file-token-id, got %q", cfg.TokenID)
	}
	if cfg.TokenSecret != "file-token-secret" {
		t.Errorf("expected TokenSecret file-token-secret, got %q", cfg.TokenSecret)
	}
}

func TestLoadConfig_FileSecretErrors(t *testing.T) {
	tempDir := t.TempDir()
	emptyFile := filepath.Join(tempDir, "empty.txt")
	if err := os.WriteFile(emptyFile, []byte("   \n"), 0o600); err != nil {
		t.Fatal(err)
	}

	t.Run("missing file", func(t *testing.T) {
		t.Setenv(envHost, testHost)
		t.Setenv(envTokenIDFile, "/nonexistent/path/id.txt")
		t.Setenv(envTokenSecret, "secret")

		_, err := LoadConfig()
		if err == nil {
			t.Fatal("expected error for nonexistent file, got nil")
		}
		if !strings.Contains(err.Error(), "reading SOLIDSERVER_TOKEN_ID_FILE") {
			t.Errorf("unexpected error message: %v", err)
		}
	})

	t.Run("empty file", func(t *testing.T) {
		t.Setenv(envHost, testHost)
		t.Setenv(envTokenIDFile, emptyFile)
		t.Setenv(envTokenSecret, "secret")

		_, err := LoadConfig()
		if err == nil {
			t.Fatal("expected error for empty file, got nil")
		}
		if !strings.Contains(err.Error(), "is empty") {
			t.Errorf("unexpected error message: %v", err)
		}
	})
}

func TestLoadConfig_InvalidValues(t *testing.T) {
	tests := []struct {
		name    string
		env     string
		value   string
		wantErr string
	}{
		{"unknown transport", envTransport, "grpc", `invalid SOLIDSERVER_TRANSPORT "grpc": expected "stdio" or "http"`},
		{"unknown log level", envLogLevel, "trace", `invalid SOLIDSERVER_LOG_LEVEL "trace": expected "debug", "info", "warn" or "error"`},
		{"non-boolean ssl verify", envSSLVerify, "fasle", `invalid SOLIDSERVER_SSL_VERIFY "fasle": expected a boolean`},
		{"non-boolean read only", envReadOnly, "notabool", `invalid SOLIDSERVER_READ_ONLY "notabool": expected a boolean`},
		{"non-boolean log redact", envLogRedactPII, "notabool", `invalid SOLIDSERVER_LOG_REDACT_PII "notabool": expected a boolean`},
		{"invalid http timeout", envHTTPTimeout, "invalid", `invalid SOLIDSERVER_HTTP_TIMEOUT "invalid": expected a positive duration (e.g. 30s, 1m)`},
		{"zero http timeout", envHTTPTimeout, "0s", `invalid SOLIDSERVER_HTTP_TIMEOUT "0s": expected a positive duration (e.g. 30s, 1m)`},
		{"negative retries", envMaxRetries, "-1", `invalid SOLIDSERVER_MAX_RETRIES "-1": expected a non-negative integer`},
		{"non-numeric retries", envMaxRetries, "five", `invalid SOLIDSERVER_MAX_RETRIES "five": expected a non-negative integer`},
		{"negative rate limit", envRateLimit, "-5.0", `invalid SOLIDSERVER_RATE_LIMIT "-5.0": expected a non-negative number`},
		{"non-numeric rate limit", envRateLimit, "abc", `invalid SOLIDSERVER_RATE_LIMIT "abc": expected a non-negative number`},
		{"non-numeric port", envHTTPPort, "http", `invalid SOLIDSERVER_HTTP_PORT "http": expected an integer`},
		{"port above range", envHTTPPort, "99999", "invalid SOLIDSERVER_HTTP_PORT 99999: expected 1-65535"},
		{"port below range", envHTTPPort, "0", "invalid SOLIDSERVER_HTTP_PORT 0: expected 1-65535"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(envHost, testHost)
			t.Setenv(envTokenID, "token-id")
			t.Setenv(envTokenSecret, "token-secret")
			for _, optional := range []string{
				envTransport, envLogLevel, envHTTPHost, envSSLVerify, envHTTPPort,
				envReadOnly, envProtectedSpaces, envProtectedZones, envProtectedSubnets,
				envHTTPTimeout, envMaxRetries, envRateLimit, envLogRedactPII,
				envTokenIDFile, envTokenSecretFile,
			} {
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
		{"missing token id", testHost, "", "secret", "SOLIDSERVER_TOKEN_ID or SOLIDSERVER_TOKEN_ID_FILE environment variable is required"},
		{"missing token secret", testHost, "id", "", "SOLIDSERVER_TOKEN_SECRET or SOLIDSERVER_TOKEN_SECRET_FILE environment variable is required"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(envHost, tt.host)
			t.Setenv(envTokenID, tt.tokenID)
			t.Setenv(envTokenSecret, tt.tokenSecret)
			t.Setenv(envTokenIDFile, "")
			t.Setenv(envTokenSecretFile, "")

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
