package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"regexp"
	"syscall"
	"time"

	"github.com/tphakala/solidserver-mcp/services"
)

// version is reported over MCP in the server implementation, on /health, and in
// the startup log. Release builds overwrite it via -ldflags with the tag being
// built, so there is no constant here to drift out of step with the tag. A
// local or source build reports "dev".
var version = "dev"

var (
	ipRegex  = regexp.MustCompile(`\b\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}\b`)
	macRegex = regexp.MustCompile(`\b[0-9a-fA-F]{2}(:[0-9a-fA-F]{2}){5,6}\b`)
)

func redactPIIAttr(groups []string, a slog.Attr) slog.Attr {
	if a.Value.Kind() == slog.KindString {
		str := a.Value.String()
		str = ipRegex.ReplaceAllString(str, "[REDACTED_IP]")
		str = macRegex.ReplaceAllString(str, "[REDACTED_MAC]")
		return slog.Attr{Key: a.Key, Value: slog.StringValue(str)}
	}
	return a
}

func main() {
	if err := runMain(); err != nil {
		// LoadConfig failures happen before the configured logger exists, so
		// report through a bootstrap logger. Without this a missing environment
		// variable exits 1 with no output at all. Going through slog keeps
		// stderr uniformly JSON so log ingestion is not broken by a stray
		// plain-text line.
		slog.New(slog.NewJSONHandler(os.Stderr, nil)).Error("fatal error", "error", err)
		os.Exit(1)
	}
}

const doctorCLITimeout = 30 * time.Second

func runDoctorCLI(ctx context.Context, cfg *Config) error {
	client, err := newClientFromConfig(cfg)
	if err != nil {
		return fmt.Errorf("initializing client for doctor: %w", err)
	}

	doctorCtx, cancel := context.WithTimeout(ctx, doctorCLITimeout)
	defer cancel()

	result := services.RunDoctor(doctorCtx, client, cfg.Host, cfg.SSLVerify)
	_, _ = fmt.Fprintf(os.Stdout, "SOLIDserver Preflight Doctor Report (%s):\n", cfg.Host)
	for _, check := range result.Checks {
		_, _ = fmt.Fprintf(os.Stdout, "  [%s] %s: %s\n", check.Status, check.Name, check.Message)
	}
	if !result.Healthy {
		_, _ = fmt.Fprintln(os.Stdout, "\nResult: UNHEALTHY (one or more checks failed)")
		return errors.New("preflight doctor checks failed")
	}
	_, _ = fmt.Fprintln(os.Stdout, "\nResult: HEALTHY (all checks passed)")
	return nil
}

func runMain() error {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "doctor", "-doctor", "--doctor":
			cfg, err := LoadConfig()
			if err != nil {
				return err
			}
			ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
			defer cancel()
			return runDoctorCLI(ctx, &cfg)
		case "version", "-v", "--version":
			_, _ = fmt.Fprintf(os.Stdout, "solidserver-mcp version %s\n", version)
			return nil
		}
	}

	cfg, err := LoadConfig()
	if err != nil {
		return err
	}

	// Set up structured logging to stderr (protects stdio JSON-RPC channel)
	logLevel := slog.LevelInfo
	switch cfg.LogLevel {
	case "debug":
		logLevel = slog.LevelDebug
	case "warn":
		logLevel = slog.LevelWarn
	case "error":
		logLevel = slog.LevelError
	}

	handlerOpts := &slog.HandlerOptions{
		Level: logLevel,
	}
	if cfg.LogRedactPII {
		handlerOpts.ReplaceAttr = redactPIIAttr
	}

	logger := slog.New(slog.NewJSONHandler(os.Stderr, handlerOpts))

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	// Fatal errors are reported once, by main, so they are not printed twice.
	return run(ctx, &cfg, logger)
}
