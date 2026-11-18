package handler

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/strelov1/freehire/internal/ai/assistant"
	"github.com/strelov1/freehire/internal/platform/observability"
)

func TestEmptySchemasKeepTheirShape(t *testing.T) {
	for _, tc := range []struct {
		schema map[string]any
		want   string
	}{
		{assistant.EmptySchema, `{"properties":{},"type":"object"}`},
		{assistant.ClosedEmptySchema, `{"additionalProperties":false,"properties":{},"type":"object"}`},
	} {
		got, _ := json.Marshal(tc.schema)
		if string(got) != tc.want {
			t.Errorf("schema = %s, want %s", got, tc.want)
		}
	}
}

func TestJobNotFoundWording(t *testing.T) {
	err := jobNotFound("x")
	if want := `no vacancy with slug "x" — use the public_slug from a search result`; err.Error() != want {
		t.Errorf("text = %q, want %q", err, want)
	}
	if code := observability.ErrorCode(err); code != "not_found" {
		t.Errorf("code = %q", code)
	}
	if got := slugToolError("x", pgx.ErrNoRows); got.Error() != err.Error() {
		t.Errorf("slugToolError = %q", got)
	}
}

func TestToolPageClamps(t *testing.T) {
	for _, tc := range [][4]int{{0, -3, assistantDefaultSearchLimit, 0}, {500, 7, assistantMaxSearchLimit, 7}, {5, 2, 5, 2}} {
		if l, o := toolPage(tc[0], tc[1]); l != tc[2] || o != tc[3] {
			t.Errorf("toolPage(%d,%d) = %d,%d", tc[0], tc[1], l, o)
		}
	}
}

func TestSlugToolArgumentName(t *testing.T) {
	tool := slugTool("job_slug", "t", "T", "d", assistant.EffectReads, false, func(_ context.Context, _ int64, slug string) (any, error) { return slug, nil })
	if got, err := tool.Run(context.Background(), 1, json.RawMessage(`{"job_slug":"a"}`)); err != nil || got != "a" {
		t.Errorf("got %v, %v", got, err)
	}
	for _, bad := range []string{`{}`, `{"job_slug":"a","x":"b"}`} {
		if _, err := tool.Run(context.Background(), 1, json.RawMessage(bad)); err == nil {
			t.Errorf("%s: want an error", bad)
		}
	}
	if _, err := tool.Run(context.Background(), 1, json.RawMessage(`{"slug":"a"}`)); !errors.Is(err, assistant.ErrInvalidArguments) {
		t.Errorf("wrong argument name: %v", err)
	}
}
