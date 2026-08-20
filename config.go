package main

import (
	"fmt"
	"math"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	TransportStdio = "stdio"
	TransportHTTP  = "http"

	LogLevelDebug = "debug"
	LogLevelInfo  = "info"
	LogLevelWarn  = "warn"
	LogLevelError = "error"

	defaultHTTPHost    = "127.0.0.1"
	defaultHTTPPort    = 8080
	defaultHTTPTimeout = 30 * time.Second
	defaultMaxRetries  = 3
	maxPort            = 65535
)

const (
	envHost             = "SOLIDSERVER_HOST"
	envTokenID          = "SOLIDSERVER_TOKEN_ID"
	envTokenIDFile      = "SOLIDSERVER_TOKEN_ID_FILE"
	envTokenSecret      = "SOLIDSERVER_TOKEN_SECRET"
	envTokenSecretFile  = "SOLIDSERVER_TOKEN_SECRET_FILE"
	envSSLVerify        = "SOLIDSERVER_SSL_VERIFY"
	envReadOnly         = "SOLIDSERVER_READ_ONLY"
	envProtectedSpaces  = "SOLIDSERVER_PROTECTED_SPACES"
	envProtectedZones   = "SOLIDSERVER_PROTECTED_ZONES"
	envProtectedSubnets = "SOLIDSERVER_PROTECTED_SUBNETS"
	envHTTPTimeout      = "SOLIDSERVER_HTTP_TIMEOUT"
	envMaxRetries       = "SOLIDSERVER_MAX_RETRIES"
	envRateLimit        = "SOLIDSERVER_RATE_LIMIT"
	envLogRedactPII     = "SOLIDSERVER_LOG_REDACT_PII"
	envTransport        = "SOLIDSERVER_TRANSPORT"
	envLogLevel         = "SOLIDSERVER_LOG_LEVEL"
	envHTTPHost         = "SOLIDSERVER_HTTP_HOST"
	envHTTPPort         = "SOLIDSERVER_HTTP_PORT"
)

// Config holds all server configuration loaded from environment variables and secret files.
type Config struct {
	Host             string
	TokenID          string
	TokenSecret      string
	SSLVerify        bool
	ReadOnly         bool
	ProtectedSpaces  []string
	ProtectedZones   []string
	ProtectedSubnets []string
	HTTPTimeout      time.Duration
	MaxRetries       int
	RateLimit        float64
	LogRedactPII     bool
	Transport        string
	LogLevel         string
	HTTPHost         string
	HTTPPort         int
}

func readSecret(envVar, fileVar string) (string, error) {
	if filePath := os.Getenv(fileVar); filePath != "" {
		data, err := os.ReadFile(filePath)
		if err != nil {
			return "", fmt.Errorf("reading %s from %q: %w", fileVar, filePath, err)
		}
		secret := strings.TrimSpace(string(data))
		if secret == "" {
			return "", fmt.Errorf("secret file %s %q is empty", fileVar, filePath)
		}
		return secret, nil
	}
	return os.Getenv(envVar), nil
}

func parseCommaList(val string) []string {
	if val == "" {
		return []string{}
	}
	parts := strings.Split(val, ",")
	res := make([]string, 0, len(parts))
	for _, p := range parts {
		trimmed := strings.TrimSpace(p)
		if trimmed != "" {
			res = append(res, trimmed)
		}
	}
	return res
}

func parseTransport(t string) (string, error) {
	switch t {
	case "":
		return TransportStdio, nil
	case TransportStdio, TransportHTTP:
		return t, nil
	default:
		return "", fmt.Errorf("invalid %s %q: expected %q or %q", envTransport, t, TransportStdio, TransportHTTP)
	}
}

func parseLogLevel(l string) (string, error) {
	switch l {
	case "":
		return LogLevelInfo, nil
	case LogLevelDebug, LogLevelInfo, LogLevelWarn, LogLevelError:
		return l, nil
	default:
		return "", fmt.Errorf("invalid %s %q: expected %q, %q, %q or %q",
			envLogLevel, l, LogLevelDebug, LogLevelInfo, LogLevelWarn, LogLevelError)
	}
}

func parseBools(cfg *Config) error {
	cfg.SSLVerify = true
	if sslVerifyStr := os.Getenv(envSSLVerify); sslVerifyStr != "" {
		verify, err := strconv.ParseBool(sslVerifyStr)
		if err != nil {
			return fmt.Errorf("invalid %s %q: expected a boolean", envSSLVerify, sslVerifyStr)
		}
		cfg.SSLVerify = verify
	}

	if readOnlyStr := os.Getenv(envReadOnly); readOnlyStr != "" {
		ro, err := strconv.ParseBool(readOnlyStr)
		if err != nil {
			return fmt.Errorf("invalid %s %q: expected a boolean", envReadOnly, readOnlyStr)
		}
		cfg.ReadOnly = ro
	}

	if redactStr := os.Getenv(envLogRedactPII); redactStr != "" {
		redact, err := strconv.ParseBool(redactStr)
		if err != nil {
			return fmt.Errorf("invalid %s %q: expected a boolean", envLogRedactPII, redactStr)
		}
		cfg.LogRedactPII = redact
	}
	return nil
}

func parseResilience(cfg *Config) error {
	cfg.HTTPTimeout = defaultHTTPTimeout
	if timeoutStr := os.Getenv(envHTTPTimeout); timeoutStr != "" {
		dur, err := time.ParseDuration(timeoutStr)
		if err != nil || dur <= 0 {
			return fmt.Errorf("invalid %s %q: expected a positive duration (e.g. 30s, 1m)", envHTTPTimeout, timeoutStr)
		}
		cfg.HTTPTimeout = dur
	}

	cfg.MaxRetries = defaultMaxRetries
	if retriesStr := os.Getenv(envMaxRetries); retriesStr != "" {
		retries, err := strconv.Atoi(retriesStr)
		if err != nil || retries < 0 {
			return fmt.Errorf("invalid %s %q: expected a non-negative integer", envMaxRetries, retriesStr)
		}
		cfg.MaxRetries = retries
	}

	if rateLimitStr := os.Getenv(envRateLimit); rateLimitStr != "" {
		rl, err := strconv.ParseFloat(rateLimitStr, 64)
		if err != nil || rl < 0 || math.IsNaN(rl) || math.IsInf(rl, 0) {
			return fmt.Errorf("invalid %s %q: expected a non-negative number", envRateLimit, rateLimitStr)
		}
		cfg.RateLimit = rl
	}
	return nil
}

func parseHTTPPort(cfg *Config) error {
	cfg.HTTPPort = defaultHTTPPort
	if portStr := os.Getenv(envHTTPPort); portStr != "" {
		port, err := strconv.Atoi(portStr)
		if err != nil {
			return fmt.Errorf("invalid %s %q: expected an integer", envHTTPPort, portStr)
		}
		if port < 1 || port > maxPort {
			return fmt.Errorf("invalid %s %d: expected 1-%d", envHTTPPort, port, maxPort)
		}
		cfg.HTTPPort = port
	}
	return nil
}

// LoadConfig reads configuration from environment variables and secret files.
func LoadConfig() (Config, error) {
	tokenID, err := readSecret(envTokenID, envTokenIDFile)
	if err != nil {
		return Config{}, err
	}

	tokenSecret, err := readSecret(envTokenSecret, envTokenSecretFile)
	if err != nil {
		return Config{}, err
	}

	transport, err := parseTransport(os.Getenv(envTransport))
	if err != nil {
		return Config{}, err
	}

	logLevel, err := parseLogLevel(os.Getenv(envLogLevel))
	if err != nil {
		return Config{}, err
	}

	httpHost := os.Getenv(envHTTPHost)
	if httpHost == "" {
		httpHost = defaultHTTPHost
	}

	cfg := Config{
		Host:             os.Getenv(envHost),
		TokenID:          tokenID,
		TokenSecret:      tokenSecret,
		ProtectedSpaces:  parseCommaList(os.Getenv(envProtectedSpaces)),
		ProtectedZones:   parseCommaList(os.Getenv(envProtectedZones)),
		ProtectedSubnets: parseCommaList(os.Getenv(envProtectedSubnets)),
		Transport:        transport,
		LogLevel:         logLevel,
		HTTPHost:         httpHost,
	}

	if cfg.Host == "" {
		return Config{}, fmt.Errorf("%s environment variable is required", envHost)
	}
	if cfg.TokenID == "" {
		return Config{}, fmt.Errorf("%s or %s environment variable is required", envTokenID, envTokenIDFile)
	}
	if cfg.TokenSecret == "" {
		return Config{}, fmt.Errorf("%s or %s environment variable is required", envTokenSecret, envTokenSecretFile)
	}

	if err := parseBools(&cfg); err != nil {
		return Config{}, err
	}
	if err := parseResilience(&cfg); err != nil {
		return Config{}, err
	}
	if err := parseHTTPPort(&cfg); err != nil {
		return Config{}, err
	}

	return cfg, nil
}
