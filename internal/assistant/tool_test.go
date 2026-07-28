package assistant

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func echoTool() Tool {
	return Tool{
		Name:        "echo",
		Description: "echo the argument back",
		Schema: map[string]any{
			"type":       "object",
			"properties": map[string]any{"text": map[string]any{"type": "string"}},
			"required":   []string{"text"},
		},
		Run: func(_ context.Context, _ int64, raw json.RawMessage) (any, error) {
			var in struct {
				Text string `json:"text"`
			}
			if err := DecodeArgs(raw, &in); err != nil {
				return nil, err
			}
			return map[string]string{"echoed": in.Text}, nil
		},
	}
}

func TestRegistryRendersToolDefinitionsInOrder(t *testing.T) {
	r := NewRegistry(echoTool(), Tool{Name: "second", Description: "d", Schema: map[string]any{"type": "object"}})

	defs := r.Definitions()
	if len(defs) != 2 {
		t.Fatalf("rendered %d definitions, want 2", len(defs))
	}
	if defs[0].Type != "function" || defs[0].Function.Name != "echo" {
		t.Errorf("first definition = %+v, want the echo function", defs[0])
	}
	if defs[0].Function.Description != "echo the argument back" {
		t.Errorf("description = %q, want the tool's own", defs[0].Function.Description)
	}
	if defs[1].Function.Name != "second" {
		t.Errorf("definitions are not in registration order: %+v", defs)
	}
}

func TestCallDispatchesAndReturnsJSON(t *testing.T) {
	r := NewRegistry(echoTool())

	got := r.Call(context.Background(), 3, "echo", json.RawMessage(`{"text":"hi"}`))
	if got.Failed {
		t.Fatalf("call failed: %s", got.Content)
	}
	if got.Content != `{"echoed":"hi"}` {
		t.Errorf("content = %s, want the tool's JSON result", got.Content)
	}
}

func TestCallOfAnUnknownToolIsAResultNotACrash(t *testing.T) {
	r := NewRegistry(echoTool())

	got := r.Call(context.Background(), 3, "teleport", json.RawMessage(`{}`))
	if !got.Failed {
		t.Fatal("an unknown tool must come back as a failed result the model can read")
	}
	if !strings.Contains(got.Content, "teleport") {
		t.Errorf("content = %s, want it to name the unknown tool", got.Content)
	}
	if !json.Valid([]byte(got.Content)) {
		t.Errorf("content = %s, want valid JSON", got.Content)
	}
}

func TestCallReportsAToolErrorToTheModel(t *testing.T) {
	r := NewRegistry(Tool{
		Name:   "boom",
		Schema: map[string]any{"type": "object"},
		Run: func(context.Context, int64, json.RawMessage) (any, error) {
			return nil, errors.New("search backend unavailable")
		},
	})

	got := r.Call(context.Background(), 3, "boom", json.RawMessage(`{}`))
	if !got.Failed {
		t.Fatal("a failing tool must come back as a failed result, not abort the turn")
	}
	if !strings.Contains(got.Content, "search backend unavailable") {
		t.Errorf("content = %s, want the tool's error message", got.Content)
	}
}

func TestCallPassesTheCallerThrough(t *testing.T) {
	var seen int64
	r := NewRegistry(Tool{
		Name:   "whoami",
		Schema: map[string]any{"type": "object"},
		Run: func(_ context.Context, userID int64, _ json.RawMessage) (any, error) {
			seen = userID
			return map[string]int64{"user_id": userID}, nil
		},
	})

	r.Call(context.Background(), 42, "whoami", json.RawMessage(`{}`))
	if seen != 42 {
		t.Errorf("tool saw user %d, want the authenticated caller 42", seen)
	}
}

func TestDecodeArgsRejectsUnknownFields(t *testing.T) {
	var in struct {
		Text string `json:"text"`
	}
	err := DecodeArgs(json.RawMessage(`{"text":"hi","colour":"blue"}`), &in)
	if err == nil {
		t.Fatal("DecodeArgs accepted an unknown field; a mis-addressed call must be reported")
	}
	if !strings.Contains(err.Error(), "colour") {
		t.Errorf("error = %v, want it to name the offending field so the model can correct itself", err)
	}
}

func TestDecodeArgsRejectsATypeMismatch(t *testing.T) {
	var in struct {
		Limit int `json:"limit"`
	}
	if err := DecodeArgs(json.RawMessage(`{"limit":"twenty"}`), &in); err == nil {
		t.Fatal("DecodeArgs accepted a string for an int field")
	}
}

func TestDecodeArgsAcceptsAnEmptyArgumentObject(t *testing.T) {
	// Models routinely send "" or "{}" for a no-argument tool; neither is an error.
	var in struct{}
	if err := DecodeArgs(json.RawMessage(``), &in); err != nil {
		t.Errorf("empty arguments: %v", err)
	}
	if err := DecodeArgs(json.RawMessage(`{}`), &in); err != nil {
		t.Errorf("empty object: %v", err)
	}
}

func TestDecodeArgsRejectsTrailingContent(t *testing.T) {
	var in struct {
		Text string `json:"text"`
	}
	if err := DecodeArgs(json.RawMessage(`{"text":"a"}{"text":"b"}`), &in); err == nil {
		t.Fatal("DecodeArgs accepted two concatenated objects; only one argument object is valid")
	}
}
