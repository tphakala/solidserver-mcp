package services

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"time"
)

const (
	defaultCheckCapacity = 4
	dialTimeout          = 5 * time.Second
)

// CheckStatus represents the status of a single diagnostic check.
type CheckStatus string

const (
	StatusOK   CheckStatus = "OK"
	StatusFail CheckStatus = "FAIL"
)

// DiagnosticCheck represents a single preflight diagnostic check.
type DiagnosticCheck struct {
	Name    string      `json:"name"`
	Status  CheckStatus `json:"status"`
	Message string      `json:"message,omitempty"`
}

// DoctorResult represents the aggregate outcome of all preflight checks.
type DoctorResult struct {
	Healthy bool              `json:"healthy"`
	Host    string            `json:"host"`
	Checks  []DiagnosticCheck `json:"checks"`
}

// RunDoctor performs connectivity, TLS, and credential preflight checks against SOLIDserver.
// It never includes secrets or credentials in the output.
func RunDoctor(ctx context.Context, client *APIClientWrapper, host string, sslVerify bool) DoctorResult {
	res := DoctorResult{
		Healthy: true,
		Host:    host,
		Checks:  make([]DiagnosticCheck, 0, defaultCheckCapacity),
	}

	addCheck := func(name string, ok bool, msg string) {
		status := StatusOK
		if !ok {
			status = StatusFail
			res.Healthy = false
		}
		res.Checks = append(res.Checks, DiagnosticCheck{
			Name:    name,
			Status:  status,
			Message: msg,
		})
	}

	hostname := host
	port := "443"
	if h, p, err := net.SplitHostPort(host); err == nil {
		hostname = h
		port = p
	}

	// Check 1: DNS Resolution
	ips, err := net.DefaultResolver.LookupIP(ctx, "ip", hostname)
	if err != nil || len(ips) == 0 {
		addCheck("DNS Resolution", false, fmt.Sprintf("failed to resolve host %q: %v", hostname, err))
		return res
	}
	addCheck("DNS Resolution", true, fmt.Sprintf("resolved %d IP address(es)", len(ips)))

	// Check 2: Network Reachability & TCP connection
	dialer := &net.Dialer{Timeout: dialTimeout}
	conn, err := dialer.DialContext(ctx, "tcp", net.JoinHostPort(hostname, port))
	if err != nil {
		addCheck("Network Reachability", false, fmt.Sprintf("TCP dial to %s:%s failed: %v", hostname, port, err))
		return res
	}
	_ = conn.Close()
	addCheck("Network Reachability", true, fmt.Sprintf("TCP connect to %s:%s succeeded", hostname, port))

	// Check 3: TLS Handshake
	tlsConfig := &tls.Config{
		ServerName:         hostname,
		InsecureSkipVerify: !sslVerify,
		MinVersion:         tls.VersionTLS12,
	}
	tlsDialer := &tls.Dialer{
		NetDialer: dialer,
		Config:    tlsConfig,
	}
	tlsConn, err := tlsDialer.DialContext(ctx, "tcp", net.JoinHostPort(hostname, port))
	if err != nil {
		addCheck("TLS Handshake", false, fmt.Sprintf("TLS handshake to %s:%s failed: %v", hostname, port, err))
		return res
	}
	_ = tlsConn.Close()
	addCheck("TLS Handshake", true, fmt.Sprintf("TLS handshake established (SSLVerify=%v)", sslVerify))

	// Check 4: Authentication & API Probe
	if client != nil {
		authCtx := client.AuthContext(ctx)
		req := client.IpamAPI.IpamSpaceList(authCtx).Limit(1)
		_, httpResp, apiErr := req.Execute()
		if httpResp != nil && httpResp.Body != nil {
			_ = httpResp.Body.Close()
		}
		if apiErr != nil {
			statusMsg := ""
			if httpResp != nil {
				statusMsg = fmt.Sprintf(" (HTTP %d)", httpResp.StatusCode)
			}
			addCheck("API Authentication", false, fmt.Sprintf("token authentication failed%s: %v", statusMsg, apiErr))
			return res
		}
		addCheck("API Authentication", true, "API token verified and space query succeeded")
	}

	return res
}

// ProbeUpstream performs a lightweight API check to verify upstream readiness.
func (c *APIClientWrapper) ProbeUpstream(ctx context.Context) error {
	authCtx := c.AuthContext(ctx)
	req := c.IpamAPI.IpamSpaceList(authCtx).Limit(1)
	_, httpResp, err := req.Execute()
	if httpResp != nil && httpResp.Body != nil {
		_ = httpResp.Body.Close()
	}
	return err
}
