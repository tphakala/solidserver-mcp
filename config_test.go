package main

import (
	"testing"
)

func TestLoadConfig_Valid(t *testing.T) {
	t.Setenv("SOLIDSERVER_HOST", "sds.example.com")
	t.Setenv("SOLIDSERVER_TOKEN_ID", "token-id")
	t.Setenv("SOLIDSERVER_TOKEN_SECRET", "token-secret")
	t.Setenv("MCP_TRANSPORT", "http")
	t.Setenv("LOG_LEVEL", "debug")
	t.Setenv("MCP_HTTP_HOST", "127.0.0.1")
	t.Setenv("SOLIDSERVER_SSL_VERIFY", "false")
	t.Setenv("MCP_HTTP_PORT", "9090")

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if cfg.Host != "sds.example.com" {
		t.Errorf("expected Host sds.example.com, got %q", cfg.Host)
	}
	if cfg.TokenID != "token-id" {
		t.Errorf("expected TokenID token-id, got %q", cfg.TokenID)
	}
	if cfg.TokenSecret != "token-secret" {
		t.Errorf("expected TokenSecret token-secret, got %q", cfg.TokenSecret)
	}
	if cfg.Transport != "http" {
		t.Errorf("expected Transport http, got %q", cfg.Transport)
	}
	if cfg.LogLevel != "debug" {
		t.Errorf("expected LogLevel debug, got %q", cfg.LogLevel)
	}
	if cfg.HTTPHost != "127.0.0.1" {
		t.Errorf("expected HTTPHost 127.0.0.1, got %q", cfg.HTTPHost)
	}
	if cfg.SSLVerify != false {
		t.Errorf("expected SSLVerify false, got %v", cfg.SSLVerify)
	}
	if cfg.HTTPPort != 9090 {
		t.Errorf("expected HTTPPort 9090, got %d", cfg.HTTPPort)
	}
}

func TestLoadConfig_Defaults(t *testing.T) {
	t.Setenv("SOLIDSERVER_HOST", "sds.example.com")
	t.Setenv("SOLIDSERVER_TOKEN_ID", "token-id")
	t.Setenv("SOLIDSERVER_TOKEN_SECRET", "token-secret")
	// Unset optional ones to trigger defaults

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	const defaultTransport = "stdio"
	if cfg.Transport != defaultTransport {
		t.Errorf("expected default Transport %s, got %q", defaultTransport, cfg.Transport)
	}
	if cfg.LogLevel != "info" {
		t.Errorf("expected default LogLevel info, got %q", cfg.LogLevel)
	}
	if cfg.HTTPHost != "localhost" {
		t.Errorf("expected default HTTPHost localhost, got %q", cfg.HTTPHost)
	}
	if cfg.SSLVerify != true {
		t.Errorf("expected default SSLVerify true, got %v", cfg.SSLVerify)
	}
	if cfg.HTTPPort != 8080 {
		t.Errorf("expected default HTTPPort 8080, got %d", cfg.HTTPPort)
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
		{"missing token id", "sds.example.com", "", "secret", "SOLIDSERVER_TOKEN_ID environment variable is required"},
		{"missing token secret", "sds.example.com", "id", "", "SOLIDSERVER_TOKEN_SECRET environment variable is required"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("SOLIDSERVER_HOST", tt.host)
			t.Setenv("SOLIDSERVER_TOKEN_ID", tt.tokenID)
			t.Setenv("SOLIDSERVER_TOKEN_SECRET", tt.tokenSecret)

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
