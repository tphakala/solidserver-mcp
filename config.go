package main

import (
	"os"
	"strconv"
)

// Config holds the configuration for the SolidServer MCP server.
type Config struct {
	Host       string
	Username   string
	Password   string
	SSLVerify  bool
	Transport  string // "stdio" or "http"
	HTTPPort   int
	HTTPHost   string
	LogLevel   string // "debug", "info", "warn", "error"
}

// LoadConfig reads configuration from environment variables.
func LoadConfig() Config {
	cfg := Config{
		Host:      os.Getenv("SOLIDSERVER_HOST"),
		Username:  os.Getenv("SOLIDSERVER_USERNAME"),
		Password:  os.Getenv("SOLIDSERVER_PASSWORD"),
		Transport: os.Getenv("MCP_TRANSPORT"),
		LogLevel:  os.Getenv("LOG_LEVEL"),
		HTTPHost:  os.Getenv("MCP_HTTP_HOST"),
	}

	if cfg.Transport == "" {
		cfg.Transport = "stdio"
	}

	if cfg.LogLevel == "" {
		cfg.LogLevel = "info"
	}

	if cfg.HTTPHost == "" {
		cfg.HTTPHost = "localhost"
	}

	sslVerifyStr := os.Getenv("SOLIDSERVER_SSL_VERIFY")
	if sslVerifyStr == "" {
		cfg.SSLVerify = true
	} else {
		verify, err := strconv.ParseBool(sslVerifyStr)
		if err != nil {
			cfg.SSLVerify = true
		} else {
			cfg.SSLVerify = verify
		}
	}

	portStr := os.Getenv("MCP_HTTP_PORT")
	if portStr != "" {
		port, err := strconv.Atoi(portStr)
		if err == nil {
			cfg.HTTPPort = port
		}
	}
	if cfg.HTTPPort == 0 {
		cfg.HTTPPort = 8080
	}

	return cfg
}
