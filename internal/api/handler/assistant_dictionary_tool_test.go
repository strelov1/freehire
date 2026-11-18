package handler

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestDictionaryMCPUsesWebsiteValidation(t *testing.T) {
	calls := 0
	h := &assistantHandlers{dictionary: &dictionaryHandlers{write: func(
		ctx context.Context, userID int64, kind, raw, normalized string,
	) (int32, string, error) {
		calls++
		if userID != 42 || kind != "skill" || raw != "Unlisted-Quasar-Tech" || normalized != "unlisted-quasar-tech" {
			t.Fatalf("write = %d %q %q %q", userID, kind, raw, normalized)
		}
		return 3, "pending", nil
	}}}
	tool := h.dictionaryMCPTools()[0]
	for _, args := range []string{
		`{"kind":"skill","term":"  "}`,
		`{"kind":"role","term":"unlisted quasar tech"}`,
		`{"kind":"skill","term":"k8s"}`,
		`{"kind":"specialization","term":"Backend"}`,
		`{"kind":"industry","term":"finance"}`,
		`{"kind":"skill","term":"` + strings.Repeat("x", 81) + `"}`,
		`{"kind":"skill","term":false}`,
		`{"kind":"skill","term":"x","user_id":99}`,
	} {
		if _, err := tool.Run(t.Context(), 42, json.RawMessage(args)); err == nil {
			t.Errorf("accepted %s", args)
		}
	}
	if calls != 0 {
		t.Fatal("invalid or known terms reached the queue")
	}
	got, err := tool.Run(t.Context(), 42, json.RawMessage(`{"kind":"skill","term":"  Unlisted-Quasar-Tech  "}`))
	if err != nil || got != (dictionaryRecordBody{RequestCount: 3, Status: "pending"}) || calls != 1 {
		t.Fatalf("result=%#v err=%v calls=%d", got, err, calls)
	}
	failure := errors.New("store unavailable")
	h.dictionary.write = func(context.Context, int64, string, string, string) (int32, string, error) {
		return 0, "", failure
	}
	if _, err := tool.Run(t.Context(), 42, json.RawMessage(`{"kind":"skill","term":"Unlisted-Quasar-Tech"}`)); !errors.Is(err, failure) {
		t.Fatalf("store failure = %v", err)
	}
}

func TestDictionaryMCPIsOAuthOnlyAndFailsClosed(t *testing.T) {
	h := &assistantHandlers{}
	for _, tool := range mcpKeyTools(h, testMCPOrigin) {
		if tool.Name == suggestDictionaryTermToolName {
			t.Fatal("cookie-only dictionary write exposed to an API key")
		}
	}
	tool := h.dictionaryMCPTools()[0]
	for _, userID := range []int64{0, 42} {
		if _, err := tool.Run(t.Context(), userID, json.RawMessage(`{"kind":"skill","term":"Unlisted-Quasar-Tech"}`)); err == nil {
			t.Fatalf("missing dependency/identity accepted for user%d", userID)
		}
	}
}

func TestDictionaryMCPProtocol(t *testing.T) {
	h := &assistantHandlers{dictionary: &dictionaryHandlers{write: func(
		ctx context.Context, userID int64, kind, raw, normalized string,
	) (int32, string, error) {
		if userID != 42 {
			t.Fatalf("tool used user%d", userID)
		}
		return 1, "pending", nil
	}}}
	session := connectMCPTools(t, h.dictionaryMCPTools(), "42")
	listed, err := session.ListTools(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	var tool *mcp.Tool
	for _, listedTool := range listed.Tools {
		if listedTool.Name == suggestDictionaryTermToolName {
			tool = listedTool
		}
	}
	if tool == nil {
		t.Fatalf("%s is not served: %v", suggestDictionaryTermToolName, listed.Tools)
	}
	annotations := tool.Annotations
	if annotations == nil || annotations.ReadOnlyHint || annotations.DestructiveHint == nil || *annotations.DestructiveHint || annotations.IdempotentHint {
		t.Fatalf("wrong annotations: %+v", annotations)
	}
	res, err := session.CallTool(t.Context(), &mcp.CallToolParams{Name: suggestDictionaryTermToolName, Arguments: map[string]any{"kind": "industry", "term": "Unlisted-Quasar-Tech"}})
	if err != nil || res.IsError {
		t.Fatalf("call=%+v err=%v", res, err)
	}
	body := []byte(res.Content[0].(*mcp.TextContent).Text)
	var result dictionaryRecordBody
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatal(err)
	}
	if result.Status != "pending" || result.RequestCount != 1 {
		t.Fatalf("result=%+v", result)
	}
}
