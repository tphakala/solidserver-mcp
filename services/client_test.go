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
	val := ctx.Value(sdsclient.ContextEipApiTokenAuth)

	auth, ok := val.(sdsclient.EipApiTokenAuth)
	if !ok {
		t.Fatalf("expected context to contain EipApiTokenAuth, got %T", val)
	}

	if auth.Token != "testid" {
		t.Errorf("expected Token testid, got %q", auth.Token)
	}
	if auth.Secret != "testsecret" {
		t.Errorf("expected Secret testsecret, got %q", auth.Secret)
	}
}
