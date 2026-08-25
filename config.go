package main

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	TransportStdio = "stdio"
	TransportHTTP  = "http"
	TransportUnix  = "unix"

	LogLevelDebug = "debug"
	LogLevelInfo  = "info"
	LogLevelWarn  = "warn"
	LogLevelError = "error"

	defaultHTTPHost    = "127.0.0.1"
	defaultHTTPPort    = 8080
	defaultHTTPTimeout = 30 * time.Second
	defaultMaxRetries  = 3
	maxPort            = 65535

	defaultSocketName = "solidserver-mcp.sock"
	// maxSocketPathLen is the tightest platform limit for a Unix socket path
	// (sun_path is 104 bytes on darwin, 108 on Linux); cap at the smaller so a
	// path that works locally also works on macOS.
	maxSocketPathLen = 104
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
	envSocket           = "SOLIDSERVER_SOCKET"
	envXDGRuntimeDir    = "XDG_RUNTIME_DIR"
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
	SocketPath       string
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
	case TransportStdio, TransportHTTP, TransportUnix:
		return t, nil
	default:
		return "", fmt.Errorf("invalid %s %q: expected %q, %q or %q", envTransport, t, TransportStdio, TransportHTTP, TransportUnix)
	}
}

// socketRuntimeDir returns the directory the default socket lives in: the user
// runtime dir when set to an absolute path, otherwise /tmp. It deliberately
// avoids os.TempDir(), whose per-user path on macOS is long enough to blow the
// sun_path limit.
func socketRuntimeDir() string {
	if dir := os.Getenv(envXDGRuntimeDir); dir != "" && filepath.IsAbs(dir) {
		return dir
	}
	return "/tmp"
}

// parseSocket resolves and validates the Unix socket path, used only when the
// transport is unix. It defaults to a socket named under the runtime dir and
// enforces an absolute path within the platform length limit.
func parseSocket(cfg *Config) error {
	if cfg.Transport != TransportUnix {
		return nil
	}
	path := os.Getenv(envSocket)
	if path == "" {
		path = filepath.Join(socketRuntimeDir(), defaultSocketName)
	}
	if !filepath.IsAbs(path) {
		return fmt.Errorf("invalid %s %q: socket path must be absolute", envSocket, path)
	}
	if len(path) >= maxSocketPathLen {
		return fmt.Errorf("invalid %s %q: path length %d meets or exceeds the %d-byte platform limit", envSocket, path, len(path), maxSocketPathLen)
	}
	cfg.SocketPath = path
	return nil
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
	if err := parseSocket(&cfg); err != nil {
		return Config{}, err
	}

	return cfg, nil
}
