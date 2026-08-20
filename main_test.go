package main

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRedactPIIAttr(t *testing.T) {
	var buf bytes.Buffer
	handler := slog.NewJSONHandler(&buf, &slog.HandlerOptions{
		ReplaceAttr: redactPIIAttr,
	})
	logger := slog.New(handler)

	logger.Info("allocated ip", "ip", "192.168.1.10", "mac", "00:11:22:33:44:55", "subnet", "10.0.0.0/24")

	out := buf.String()
	if strings.Contains(out, "192.168.1.10") {
		t.Errorf("expected IP to be redacted, got %s", out)
	}
	if strings.Contains(out, "00:11:22:33:44:55") {
		t.Errorf("expected MAC to be redacted, got %s", out)
	}
	if !strings.Contains(out, "[REDACTED_IP]") {
		t.Errorf("expected [REDACTED_IP] in output, got %s", out)
	}
	if !strings.Contains(out, "[REDACTED_MAC]") {
		t.Errorf("expected [REDACTED_MAC] in output, got %s", out)
	}
}

func TestRunDoctorCLI(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"data":[{"space_name":"dev"}]}`))
	}))
	defer server.Close()

	host := strings.TrimPrefix(server.URL, "https://")
	cfg := &Config{
		Host:        host,
		TokenID:     "id",
		TokenSecret: "secret",
		SSLVerify:   false,
	}

	err := runDoctorCLI(t.Context(), cfg)
	if err != nil {
		t.Errorf("expected runDoctorCLI to pass with mock server, got %v", err)
	}
}

func TestRunDoctorCLI_Failure(t *testing.T) {
	cfg := &Config{
		Host:        "127.0.0.1:1",
		TokenID:     "id",
		TokenSecret: "secret",
		SSLVerify:   false,
	}

	err := runDoctorCLI(t.Context(), cfg)
	if err == nil {
		t.Fatal("expected runDoctorCLI to return error on failed preflight, got nil")
	}
	if err.Error() != "preflight doctor checks failed" {
		t.Errorf("expected %q, got %q", "preflight doctor checks failed", err.Error())
	}
}
