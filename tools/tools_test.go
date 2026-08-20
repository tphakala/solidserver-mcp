package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"testing"

	"github.com/efficientip-labs/solidserver-go-client/sdsclient"
)

func TestJsonResult(t *testing.T) {
	data := map[string]string{"key": "value"}
	res := jsonResult(data)
	if res.IsError {
		t.Error("expected IsError to be false")
	}
	if len(res.Content) != 1 {
		t.Fatalf("expected 1 content item, got %d", len(res.Content))
	}

	contentStr := fmt.Sprintf("%v", res.Content[0])
	if !strings.Contains(contentStr, "key") || !strings.Contains(contentStr, "value") {
		t.Errorf("expected content to contain JSON data, got %q", contentStr)
	}
}

func TestErrorResult(t *testing.T) {
	res := errorResult("something failed: %v", errors.New("boom"))
	if !res.IsError {
		t.Error("expected IsError to be true")
	}
	if len(res.Content) != 1 {
		t.Fatalf("expected 1 content item, got %d", len(res.Content))
	}

	contentStr := fmt.Sprintf("%v", res.Content[0])
	if !strings.Contains(contentStr, "something failed: boom") {
		t.Errorf("expected content to contain error message, got %q", contentStr)
	}
}

func TestValidationErrorResult(t *testing.T) {
	emptyOut := IPCreateOut{Data: make([]sdsclient.DataInnerIpamAddressAddSuccess, 0)}
	res, out, err := validationErrorResult(errors.New("bad input"), emptyOut)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if !res.IsError {
		t.Error("expected IsError to be true")
	}
	if out.Data == nil {
		t.Fatal("expected non-nil Data slice in out")
	}
	if len(out.Data) != 0 {
		t.Errorf("expected empty data slice in out, got %d", len(out.Data))
	}
	marshaled, err := json.Marshal(out)
	if err != nil {
		t.Fatalf("failed to marshal out: %v", err)
	}
	if !strings.Contains(string(marshaled), `"data":[]`) {
		t.Errorf("expected JSON to serialize empty array \"data\":[], got %s", string(marshaled))
	}
	contentStr := fmt.Sprintf("%v", res.Content[0])
	if !strings.Contains(contentStr, "invalid parameter: bad input") {
		t.Errorf("expected error message to contain 'invalid parameter: bad input', got %q", contentStr)
	}
}

func TestFormatAPIError(t *testing.T) {
	t.Run("nil error", func(t *testing.T) {
		if msg := formatAPIError(nil, nil); msg != "" {
			t.Errorf("expected empty string for nil error, got %q", msg)
		}
	})

	t.Run("generic error with http response", func(t *testing.T) {
		httpResp := &http.Response{StatusCode: http.StatusUnauthorized}
		msg := formatAPIError(errors.New("unauthorized"), httpResp)
		if !strings.Contains(msg, "status 401") || !strings.Contains(msg, "check API token") {
			t.Errorf("expected status and hint in error message, got %q", msg)
		}
	})

	t.Run("openAPI error with errno and errmsg body", func(t *testing.T) {
		client, _ := newFakeAppliance(t, http.StatusNotFound, `{"errno":"6001","errmsg":"IP subnet not found"}`)
		authCtx := client.AuthContext(t.Context())
		_, httpResp, apiErr := client.IpamAPI.IpamNetworkInfo(authCtx).NetworkId(999).Execute()
		closeBody(httpResp)
		msg := formatAPIError(apiErr, httpResp)
		if !strings.Contains(msg, "errno 6001") {
			t.Errorf("expected error to contain errno 6001, got %q", msg)
		}
		if !strings.Contains(msg, "IP subnet not found") {
			t.Errorf("expected error to contain errmsg, got %q", msg)
		}
		if !strings.Contains(msg, "status 404") {
			t.Errorf("expected error to contain status 404, got %q", msg)
		}
	})
}

func TestCommonListHandler(t *testing.T) {
	type testItem struct {
		Name string `json:"name"`
	}

	// Test success case
	mockExecuteSuccess := func(ctx context.Context, where string, limit, offset int32) ([]testItem, *http.Response, error) {
		if limit != 10 {
			t.Errorf("expected limit 10, got %d", limit)
		}
		if offset != 5 {
			t.Errorf("expected offset 5, got %d", offset)
		}
		if where != "name='test'" {
			t.Errorf("expected where name='test', got %q", where)
		}
		return []testItem{{Name: "ok"}}, &http.Response{StatusCode: http.StatusOK}, nil
	}

	res, out, err := commonListHandler(t.Context(), ListOptions{Where: "name='test'", Limit: 10, Offset: 5}, slog.Default(), "test_tool", mockExecuteSuccess)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if res.IsError {
		t.Errorf("expected IsError to be false")
	}
	if out.Count != 1 || len(out.Data) != 1 || out.Data[0].Name != "ok" {
		t.Errorf("unexpected out: %+v", out)
	}

	// Test default limit
	mockExecuteDefaultLimit := func(ctx context.Context, where string, limit, offset int32) ([]testItem, *http.Response, error) {
		if limit != 50 {
			t.Errorf("expected default limit 50, got %d", limit)
		}
		return nil, &http.Response{StatusCode: http.StatusOK}, nil
	}

	_, outDef, _ := commonListHandler(t.Context(), ListOptions{Limit: 0}, slog.Default(), "test_tool", mockExecuteDefaultLimit)
	if outDef.Limit != 50 {
		t.Errorf("expected default limit 50 in output, got %d", outDef.Limit)
	}
	if outDef.Data == nil {
		t.Errorf("expected non-nil data slice for nil result")
	}

	// Test error case
	mockExecuteError := func(ctx context.Context, where string, limit, offset int32) ([]testItem, *http.Response, error) {
		return nil, &http.Response{StatusCode: http.StatusInternalServerError}, errors.New("API failure")
	}

	resErr, outErr, err := commonListHandler(t.Context(), ListOptions{}, slog.Default(), "test_tool", mockExecuteError)
	if err != nil {
		t.Fatalf("expected error to be handled and returned in CallToolResult, but got actual error: %v", err)
	}
	if !resErr.IsError {
		t.Errorf("expected IsError to be true on API failure")
	}
	if outErr.Data == nil {
		t.Errorf("expected non-nil Data slice on error")
	}
}

func TestCombineWhereClause(t *testing.T) {
	tests := []struct {
		fixed    string
		user     string
		expected string
	}{
		{"", "", ""},
		{"space='corp'", "", "space='corp'"},
		{"", "name='server1'", "name='server1'"},
		{"space='corp'", "name='server1'", "(space='corp') AND (name='server1')"},
	}

	for _, tt := range tests {
		got := CombineWhereClause(tt.fixed, tt.user)
		if got != tt.expected {
			t.Errorf("CombineWhereClause(%q, %q) = %q, want %q", tt.fixed, tt.user, got, tt.expected)
		}
	}
}
