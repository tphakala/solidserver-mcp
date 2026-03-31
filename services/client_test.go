package services

import (
	"testing"

	"github.com/efficientip-labs/solidserver-go-client/sdsclient"
)

func TestNewSolidServerClient_MissingCredentials(t *testing.T) {
	_, err := NewSolidServerClient("", "admin", "secret", false)
	if err == nil {
		t.Error("expected error when host is missing")
	}

	_, err = NewSolidServerClient("host", "", "secret", false)
	if err == nil {
		t.Error("expected error when username is missing")
	}

	_, err = NewSolidServerClient("host", "admin", "", false)
	if err == nil {
		t.Error("expected error when password is missing")
	}
}

func TestNewSolidServerClient_Success(t *testing.T) {
	client, err := NewSolidServerClient("sds.local", "admin", "secret", false)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if client == nil {
		t.Fatal("expected client to be non-nil")
	}
}

func TestAuthContext(t *testing.T) {
	client, err := NewSolidServerClient("sds.local", "testuser", "testpass", false)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	ctx := client.AuthContext(t.Context())
	val := ctx.Value(sdsclient.ContextBasicAuth)
	
	auth, ok := val.(sdsclient.BasicAuth)
	if !ok {
		t.Fatalf("expected context to contain BasicAuth, got %T", val)
	}

	if auth.UserName != "testuser" {
		t.Errorf("expected UserName testuser, got %q", auth.UserName)
	}
	if auth.Password != "testpass" {
		t.Errorf("expected Password testpass, got %q", auth.Password)
	}
}
