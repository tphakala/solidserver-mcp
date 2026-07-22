package main

import (
	"fmt"
	"os"
	"strconv"
)

// Environment variables read by LoadConfig.
const (
	envHost        = "SOLIDSERVER_HOST"
	envTokenID     = "SOLIDSERVER_TOKEN_ID"
	envTokenSecret = "SOLIDSERVER_TOKEN_SECRET"
	envSSLVerify   = "SOLIDSERVER_SSL_VERIFY"
	envTransport   = "MCP_TRANSPORT"
	envHTTPHost    = "MCP_HTTP_HOST"
	envHTTPPort    = "MCP_HTTP_PORT"
	envLogLevel    = "LOG_LEVEL"
)

// Supported values for Config.Transport.
const (
	TransportStdio = "stdio"
	TransportHTTP  = "http"
)

// Supported values for Config.LogLevel.
const (
	LogLevelDebug = "debug"
	LogLevelInfo  = "info"
	LogLevelWarn  = "warn"
	LogLevelError = "error"
)

// Defaults applied when the corresponding environment variable is unset.
const (
	defaultHTTPHost = "localhost"
	defaultHTTPPort = 8080
)

// maxPort is the highest valid TCP port number.
const maxPort = 65535

// Config holds the configuration for the SolidServer MCP server.
type Config struct {
	Host        string
	TokenID     string
	TokenSecret string
	SSLVerify   bool
	Transport   string // TransportStdio or TransportHTTP
	HTTPPort    int
	HTTPHost    string
	LogLevel    string // LogLevelDebug, LogLevelInfo, LogLevelWarn or LogLevelError
}

// LoadConfig reads configuration from environment variables.
func LoadConfig() (Config, error) {
	cfg := Config{
		Host:        os.Getenv(envHost),
		TokenID:     os.Getenv(envTokenID),
		TokenSecret: os.Getenv(envTokenSecret),
		Transport:   os.Getenv(envTransport),
		LogLevel:    os.Getenv(envLogLevel),
		HTTPHost:    os.Getenv(envHTTPHost),
	}

	if cfg.Host == "" {
		return Config{}, fmt.Errorf("%s environment variable is required", envHost)
	}
	if cfg.TokenID == "" {
		return Config{}, fmt.Errorf("%s environment variable is required", envTokenID)
	}
	if cfg.TokenSecret == "" {
		return Config{}, fmt.Errorf("%s environment variable is required", envTokenSecret)
	}

	switch cfg.Transport {
	case "":
		cfg.Transport = TransportStdio
	case TransportStdio, TransportHTTP:
		// valid
	default:
		return Config{}, fmt.Errorf("invalid %s %q: expected %q or %q", envTransport, cfg.Transport, TransportStdio, TransportHTTP)
	}

	switch cfg.LogLevel {
	case "":
		cfg.LogLevel = LogLevelInfo
	case LogLevelDebug, LogLevelInfo, LogLevelWarn, LogLevelError:
		// valid
	default:
		return Config{}, fmt.Errorf("invalid %s %q: expected %q, %q, %q or %q",
			envLogLevel, cfg.LogLevel, LogLevelDebug, LogLevelInfo, LogLevelWarn, LogLevelError)
	}

	if cfg.HTTPHost == "" {
		cfg.HTTPHost = defaultHTTPHost
	}

	// Default to verifying TLS certificates. A malformed value is rejected
	// rather than silently falling back, so a typo cannot be mistaken for a
	// deliberate opt-out of verification.
	cfg.SSLVerify = true
	if sslVerifyStr := os.Getenv(envSSLVerify); sslVerifyStr != "" {
		verify, err := strconv.ParseBool(sslVerifyStr)
		if err != nil {
			return Config{}, fmt.Errorf("invalid %s %q: expected a boolean", envSSLVerify, sslVerifyStr)
		}
		cfg.SSLVerify = verify
	}

	cfg.HTTPPort = defaultHTTPPort
	if portStr := os.Getenv(envHTTPPort); portStr != "" {
		port, err := strconv.Atoi(portStr)
		if err != nil {
			return Config{}, fmt.Errorf("invalid %s %q: expected an integer", envHTTPPort, portStr)
		}
		if port < 1 || port > maxPort {
			return Config{}, fmt.Errorf("invalid %s %d: expected 1-%d", envHTTPPort, port, maxPort)
		}
		cfg.HTTPPort = port
	}

	return cfg, nil
}
