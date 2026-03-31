package tools

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestTextResult(t *testing.T) {
	res, _ := textResult("hello %s", "world")
	if res.IsError {
		t.Error("expected IsError to be false")
	}
	if len(res.Content) != 1 {
		t.Fatalf("expected 1 content item, got %d", len(res.Content))
	}

	contentStr := fmt.Sprintf("%v", res.Content[0])
	if !strings.Contains(contentStr, "hello world") {
		t.Errorf("expected content to contain 'hello world', got %q", contentStr)
	}
}

func TestJsonResult(t *testing.T) {
	data := map[string]string{"key": "value"}
	res, _ := jsonResult(data)
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
	res, _ := errorResult("something failed: %v", errors.New("boom"))
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

func TestCommonListHandler(t *testing.T) {
	// Test success case
	mockExecuteSuccess := func(ctx context.Context, where string, limit, offset int32) (any, error) {
		if limit != 10 {
			t.Errorf("expected limit 10, got %d", limit)
		}
		if offset != 5 {
			t.Errorf("expected offset 5, got %d", offset)
		}
		if where != "name='test'" {
			t.Errorf("expected where name='test', got %q", where)
		}
		return map[string]string{"result": "ok"}, nil
	}

	res, _, err := commonListHandler(t.Context(), ListOptions{Where: "name='test'", Limit: 10, Offset: 5}, mockExecuteSuccess)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if res.IsError {
		t.Errorf("expected IsError to be false")
	}

	// Test default limit
	mockExecuteDefaultLimit := func(ctx context.Context, where string, limit, offset int32) (any, error) {
		if limit != 50 {
			t.Errorf("expected default limit 50, got %d", limit)
		}
		return map[string]string{}, nil
	}

	_, _, _ = commonListHandler(t.Context(), ListOptions{Limit: 0}, mockExecuteDefaultLimit)

	// Test error case
	mockExecuteError := func(ctx context.Context, where string, limit, offset int32) (any, error) {
		return nil, errors.New("API failure")
	}

	resErr, _, err := commonListHandler(t.Context(), ListOptions{}, mockExecuteError)
	if err != nil {
		t.Fatalf("expected error to be handled and returned in CallToolResult, but got actual error: %v", err)
	}
	if !resErr.IsError {
		t.Errorf("expected IsError to be true on API failure")
	}
}
