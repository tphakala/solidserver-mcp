package services

import (
	"testing"

	"github.com/efficientip-labs/solidserver-go-client/sdsclient"
)

func TestNewSolidServerClient_MissingCredentials(t *testing.T) {
	_, err := NewSolidServerClient("", "id", "secret", false)
	if err == nil {
		t.Error("expected error when host is missing")
	}

	_, err = NewSolidServerClient("host", "", "secret", false)
	if err == nil {
		t.Error("expected error when token id is missing")
	}

	_, err = NewSolidServerClient("host", "id", "", false)
	if err == nil {
		t.Error("expected error when token secret is missing")
	}
}

func TestNewSolidServerClient_Success(t *testing.T) {
	client, err := NewSolidServerClient("sds.local", "id", "secret", false)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if client == nil {
		t.Fatal("expected client to be non-nil")
	}
}

func TestAuthContext(t *testing.T) {
	client, err := NewSolidServerClient("sds.local", "testid", "testsecret", false)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	ctx := client.AuthContext(t.Context())
	val := ctx.Value(sdsclient.ContextBasicAuth)
	
	auth, ok := val.(sdsclient.BasicAuth)
	if !ok {
		t.Fatalf("expected context to contain BasicAuth, got %T", val)
	}

	if auth.UserName != "testid" {
		t.Errorf("expected UserName testid, got %q", auth.UserName)
	}
	if auth.Password != "testsecret" {
		t.Errorf("expected Password testsecret, got %q", auth.Password)
	}
}
