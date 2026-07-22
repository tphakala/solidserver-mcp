package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
)

// version is reported over MCP in the server implementation, on /health, and in
// the startup log. Release builds overwrite it via -ldflags with the tag being
// built, so there is no constant here to drift out of step with the tag. A
// local or source build reports "dev".
var version = "dev"

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

func runMain() error {
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
	logger := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: logLevel}))

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	// Fatal errors are reported once, by main, so they are not printed twice.
	return run(ctx, &cfg, logger)
}
