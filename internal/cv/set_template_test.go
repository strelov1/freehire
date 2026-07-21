package cv

import (
	"context"
	"errors"
	"testing"
)

func TestStoreSetTemplateRoundTripLeavesContentUntouched(t *testing.T) {
	s := NewStore(newFakeRepo())
	ctx := context.Background()
	doc := Document{Header: Header{FullName: "Ada Lovelace"}, Summary: "Backend engineer."}
	cvMeta, err := s.Create(ctx, 7, "My CV", DefaultTemplateID, doc)
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	if err := s.SetTemplate(ctx, cvMeta.ID, 7, "modern-sans"); err != nil {
		t.Fatalf("set template: %v", err)
	}

	rec, err := s.Get(ctx, cvMeta.ID, 7)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if rec.TemplateID != "modern-sans" {
		t.Errorf("template = %q, want modern-sans", rec.TemplateID)
	}
	if rec.Title != "My CV" {
		t.Errorf("title changed to %q, want My CV", rec.Title)
	}
	if rec.Document.Header.FullName != "Ada Lovelace" || rec.Document.Summary != "Backend engineer." {
		t.Errorf("document changed: %+v", rec.Document)
	}
}

func TestStoreSetTemplateForeignOwnerIsNotFound(t *testing.T) {
	s := NewStore(newFakeRepo())
	ctx := context.Background()
	cvMeta, _ := s.Create(ctx, 1, "Mine", DefaultTemplateID, Document{})
	if err := s.SetTemplate(ctx, cvMeta.ID, 2, "centered"); !errors.Is(err, ErrNotFound) {
		t.Errorf("foreign set err = %v, want ErrNotFound", err)
	}
}
