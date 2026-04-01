package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/tphakala/solidserver-mcp/services"
	"github.com/tphakala/solidserver-mcp/tools"
)

const (
	serverInstructions = "EfficientIP SolidServer IPAM/DNS MCP Server. Provides tools for managing IP addresses, subnets, and DNS records. Use solidserver_ip_* tools for IP management, solidserver_subnet_* for subnets, and solidserver_dns_* for DNS records."
	readTimeout        = 30 * time.Second
	writeTimeout       = 60 * time.Second
	idleTimeout        = 120 * time.Second
	shutdownTimeout    = 15 * time.Second
)

// buildServer creates and configures an MCP server with all tool handlers registered.
func buildServer(client *services.APIClientWrapper, logger *slog.Logger) *mcp.Server {
	s := mcp.NewServer(
		&mcp.Implementation{Name: "solidserver-mcp", Version: version},
		&mcp.ServerOptions{Instructions: serverInstructions},
	)

	// Tool registration
	tools.RegisterAll(s, client, logger)

	return s
}

// run is the main entry point for the server logic.
func run(ctx context.Context, cfg *Config, logger *slog.Logger) error {
	logger.Info("solidserver-mcp starting", "version", version, "transport", cfg.Transport)

	switch cfg.Transport {
	case "stdio":
		return runStdio(ctx, cfg, logger)
	case "http":
		return runHTTP(ctx, cfg, logger)
	default:
		return fmt.Errorf("unknown transport %q: expected \"stdio\" or \"http\"", cfg.Transport)
	}
}

// runStdio starts the MCP server on stdin/stdout.
func runStdio(ctx context.Context, cfg *Config, logger *slog.Logger) error {
	client, err := services.NewSolidServerClient(cfg.Host, cfg.TokenID, cfg.TokenSecret, cfg.SSLVerify)
	if err != nil {
		return fmt.Errorf("creating solidserver client: %w", err)
	}

	s := buildServer(client, logger)
	logger.Info("solidserver-mcp ready", "transport", "stdio")
	return s.Run(ctx, &mcp.StdioTransport{})
}

// runHTTP starts the MCP server over HTTP with streamable transport.
func runHTTP(ctx context.Context, cfg *Config, logger *slog.Logger) error {
	client, err := services.NewSolidServerClient(cfg.Host, cfg.TokenID, cfg.TokenSecret, cfg.SSLVerify)
	if err != nil {
		return fmt.Errorf("creating solidserver client: %w", err)
	}

	// Factory function returns an *mcp.Server for each request.
	getServer := func(r *http.Request) *mcp.Server {
		return buildServer(client, logger)
	}

	mcpHandler := mcp.NewStreamableHTTPHandler(getServer, &mcp.StreamableHTTPOptions{
		Logger: logger,
	})

	mux := http.NewServeMux()
	mux.Handle("/mcp", mcpHandler)
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{
			"status":    "ok",
			"transport": "http",
			"version":   version,
		})
	})

	addr := net.JoinHostPort(cfg.HTTPHost, strconv.Itoa(cfg.HTTPPort))
	httpServer := &http.Server{
		Addr:         addr,
		Handler:      mux,
		ReadTimeout:  readTimeout,
		WriteTimeout: writeTimeout,
		IdleTimeout:  idleTimeout,
	}

	// Graceful shutdown: drain active connections before closing.
	go func() {
		<-ctx.Done()
		// Use a context that inherits values but not cancellation to allow shutdown.
		shutdownCtx, cancelShutdown := context.WithTimeout(context.WithoutCancel(ctx), shutdownTimeout)
		defer cancelShutdown()
		_ = httpServer.Shutdown(shutdownCtx)
	}()

	logger.Info("solidserver-mcp HTTP server listening", "addr", addr)
	if err := httpServer.ListenAndServe(); errors.Is(err, http.ErrServerClosed) {
		return nil
	} else {
		return err
	}
}
