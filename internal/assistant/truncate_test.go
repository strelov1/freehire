package assistant

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

// bulkTool answers with a payload of the requested size.
func bulkTool(size int) Tool {
	return Tool{
		Name:   "bulk",
		Schema: map[string]any{"type": "object"},
		Run: func(context.Context, int64, json.RawMessage) (any, error) {
			return map[string]string{"body": strings.Repeat("x", size)}, nil
		},
	}
}

func TestOversizedToolResultIsTruncatedWithAMarker(t *testing.T) {
	r := NewRegistry(bulkTool(5000))
	r.MaxResultBytes = 500

	got := r.Call(context.Background(), 3, "bulk", nil)
	if len(got.Content) > 700 {
		t.Fatalf("content is %d bytes, want it capped near 500 (plus the envelope)", len(got.Content))
	}
	if !json.Valid([]byte(got.Content)) {
		t.Fatalf("truncated content is not valid JSON: %s", got.Content)
	}
	if !strings.Contains(got.Content, "truncated") {
		t.Errorf("content = %s, want it to say it was truncated so the model knows the set is partial", got.Content)
	}
}

func TestResultWithinTheCapIsUntouched(t *testing.T) {
	r := NewRegistry(bulkTool(10))
	r.MaxResultBytes = 500

	got := r.Call(context.Background(), 3, "bulk", nil)
	if strings.Contains(got.Content, "truncated") {
		t.Errorf("content = %s, want the payload verbatim when it fits", got.Content)
	}
}

func TestZeroCapMeansNoTruncation(t *testing.T) {
	r := NewRegistry(bulkTool(5000))
	// MaxResultBytes left at zero: an unconfigured registry must not silently
	// truncate everything to nothing.
	got := r.Call(context.Background(), 3, "bulk", nil)
	if len(got.Content) < 5000 {
		t.Errorf("content is %d bytes, want the full payload when no cap is set", len(got.Content))
	}
}

func TestAnErrorResultIsNeverTruncated(t *testing.T) {
	r := NewRegistry(bulkTool(10))
	r.MaxResultBytes = 10

	got := r.Call(context.Background(), 3, "nope", nil)
	if !got.Failed {
		t.Fatal("want a failed result for an unknown tool")
	}
	if !strings.Contains(got.Content, "nope") {
		t.Errorf("content = %s, want the error message intact — truncating it would hide the correction", got.Content)
	}
}
