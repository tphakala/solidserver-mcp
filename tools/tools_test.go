package tools

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"testing"
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
	res, out, err := validationErrorResult[IPCreateOut](errors.New("bad input"))
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if !res.IsError {
		t.Error("expected IsError to be true")
	}
	if len(out.Data) != 0 {
		t.Errorf("expected empty data in zero value out, got %d", len(out.Data))
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
