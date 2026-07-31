package cv

import (
	"context"
	"errors"
	"testing"
)

func TestStoreSetSessionRoundTrip(t *testing.T) {
	s := NewStore(newFakeRepo())
	ctx := context.Background()
	cvMeta, err := s.CreateTailored(ctx, 7, 100, "Tailored", DefaultTemplateID, Document{})
	if err != nil {
		t.Fatalf("create tailored: %v", err)
	}
	if err := s.SetSession(ctx, cvMeta.ID, 7, "sess-abc"); err != nil {
		t.Fatalf("set session: %v", err)
	}
	rec, err := s.Get(ctx, cvMeta.ID, 7)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if rec.AgentSessionID != "sess-abc" {
		t.Errorf("session = %q, want sess-abc", rec.AgentSessionID)
	}
}

func TestStoreSetSessionForeignOwnerIsNotFound(t *testing.T) {
	s := NewStore(newFakeRepo())
	ctx := context.Background()
	cvMeta, _ := s.Create(ctx, 1, "Mine", DefaultTemplateID, Document{})
	if err := s.SetSession(ctx, cvMeta.ID, 2, "x"); !errors.Is(err, ErrNotFound) {
		t.Errorf("foreign set err = %v, want ErrNotFound", err)
	}
}

// The toggle is a consent decision, so the same owner scoping that protects the document
// protects it: a stranger addressing someone else's CV must be told it does not exist, not that
// it exists and is theirs to configure.
func TestStoreSetTracerLinksIsOwnerScoped(t *testing.T) {
	s := NewStore(newFakeRepo())
	ctx := context.Background()
	meta, err := s.Create(ctx, 7, "CV", "classic-ats", Document{})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	if err := s.SetTracerLinks(ctx, meta.ID, 7, true); err != nil {
		t.Fatalf("owner enabling tracing: %v", err)
	}
	if err := s.SetTracerLinks(ctx, meta.ID, 8, true); !errors.Is(err, ErrNotFound) {
		t.Errorf("a stranger enabled tracing on someone else's CV: err = %v, want ErrNotFound", err)
	}
}
