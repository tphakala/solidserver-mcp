package main

import (
	"testing"
)

func TestLoadConfig_Valid(t *testing.T) {
	t.Setenv("SOLIDSERVER_HOST", "sds.example.com")
	t.Setenv("SOLIDSERVER_USERNAME", "admin")
	t.Setenv("SOLIDSERVER_PASSWORD", "secret")
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
	if cfg.Username != "admin" {
		t.Errorf("expected Username admin, got %q", cfg.Username)
	}
	if cfg.Password != "secret" {
		t.Errorf("expected Password secret, got %q", cfg.Password)
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
	t.Setenv("SOLIDSERVER_USERNAME", "admin")
	t.Setenv("SOLIDSERVER_PASSWORD", "secret")
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
		name     string
		host     string
		username string
		password string
		wantErr  string
	}{
		{"missing host", "", "admin", "secret", "SOLIDSERVER_HOST environment variable is required"},
		{"missing username", "sds.example.com", "", "secret", "SOLIDSERVER_USERNAME environment variable is required"},
		{"missing password", "sds.example.com", "admin", "", "SOLIDSERVER_PASSWORD environment variable is required"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("SOLIDSERVER_HOST", tt.host)
			t.Setenv("SOLIDSERVER_USERNAME", tt.username)
			t.Setenv("SOLIDSERVER_PASSWORD", tt.password)

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
