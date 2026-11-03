package cv

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
)

func twoBases(t *testing.T, s *Store) (first, second Meta) {
	t.Helper()
	ctx := context.Background()
	first, err := s.Create(ctx, 7, "First", DefaultTemplateID, Document{Summary: "one"})
	if err != nil {
		t.Fatal(err)
	}
	second, err = s.Create(ctx, 7, "Second", DefaultTemplateID, Document{Summary: "two"})
	if err != nil {
		t.Fatal(err)
	}
	return first, second
}

func TestTailorFromChosenBaseRecordsItAndMakesItTheDefault(t *testing.T) {
	s := NewStore(newFakeRepo())
	ctx := context.Background()
	first, second := twoBases(t, s)

	if def, _, _ := s.BaseCV(ctx, 7); def.ID != first.ID {
		t.Fatalf("default before tailoring = %s, want oldest %s", def.ID, first.ID)
	}
	base, tailored, _, err := s.Tailor(ctx, 7, 100, second.ID, "T", fakeSeeder{}, nil)
	if err != nil {
		t.Fatalf("tailor: %v", err)
	}
	if base.ID != second.ID {
		t.Errorf("base = %s, want the chosen %s", base.ID, second.ID)
	}
	trec, _ := s.Get(ctx, tailored.ID, 7)
	if trec.BaseCVID != second.ID {
		t.Errorf("tailored.BaseCVID = %s, want %s", trec.BaseCVID, second.ID)
	}
	// The copy carries the chosen base's document, not the default's.
	if trec.Document.Summary != "two" {
		t.Errorf("copied summary = %q, want the chosen base's", trec.Document.Summary)
	}
	if def, _, _ := s.BaseCV(ctx, 7); def.ID != second.ID {
		t.Errorf("default after tailoring = %s, want the base last used %s", def.ID, second.ID)
	}
}

func TestTailorRejectsAForeignOrTailoredBase(t *testing.T) {
	s := NewStore(newFakeRepo())
	ctx := context.Background()
	first, _ := twoBases(t, s)
	foreign, err := s.Create(ctx, 8, "Theirs", DefaultTemplateID, Document{})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := s.Tailor(ctx, 7, 100, foreign.ID, "T", fakeSeeder{}, nil); !errors.Is(err, ErrNotFound) {
		t.Errorf("foreign base err = %v, want ErrNotFound", err)
	}
	_, tailored, _, err := s.Tailor(ctx, 7, 100, first.ID, "T", fakeSeeder{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := s.Tailor(ctx, 7, 200, tailored.ID, "T", fakeSeeder{}, nil); !errors.Is(err, ErrNotBase) {
		t.Errorf("tailored-as-base err = %v, want ErrNotBase", err)
	}
}

func TestReopeningAVacancyWithAnotherBaseReportsTheCopysOwnBase(t *testing.T) {
	s := NewStore(newFakeRepo())
	ctx := context.Background()
	first, second := twoBases(t, s)
	if _, _, _, err := s.Tailor(ctx, 7, 100, first.ID, "T", fakeSeeder{}, nil); err != nil {
		t.Fatal(err)
	}
	base, _, created, err := s.Tailor(ctx, 7, 100, second.ID, "T", fakeSeeder{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if created {
		t.Error("a repeat tailor of the same vacancy must reuse the copy")
	}
	if base.ID != first.ID {
		t.Errorf("base = %s, want the copy's own base %s", base.ID, first.ID)
	}
}

func TestBaseForFallsBackToTheDefaultWhenItsBaseIsGone(t *testing.T) {
	s := NewStore(newFakeRepo())
	ctx := context.Background()
	first, second := twoBases(t, s)
	_, tailored, _, err := s.Tailor(ctx, 7, 100, second.ID, "T", fakeSeeder{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	trec, _ := s.Get(ctx, tailored.ID, 7)
	if got, ok, _ := s.BaseFor(ctx, 7, trec); !ok || got.ID != second.ID {
		t.Fatalf("BaseFor = %s ok=%v, want own base %s", got.ID, ok, second.ID)
	}
	if err := s.Delete(ctx, second.ID, 7); err != nil {
		t.Fatal(err)
	}
	if got, ok, _ := s.BaseFor(ctx, 7, trec); !ok || got.ID != first.ID {
		t.Fatalf("after delete BaseFor = %s ok=%v, want default %s", got.ID, ok, first.ID)
	}
	if got, ok, _ := s.BaseFor(ctx, 7, Record{}); !ok || got.ID != first.ID {
		t.Fatalf("copy with no recorded base: BaseFor = %s ok=%v, want default", got.ID, ok)
	}
}

func TestBasesFlagsExactlyOneDefaultAndIsEmptyForNone(t *testing.T) {
	s := NewStore(newFakeRepo())
	ctx := context.Background()
	if items, err := s.Bases(ctx, 7); err != nil || len(items) != 0 {
		t.Fatalf("no bases: items=%v err=%v", items, err)
	}
	first, second := twoBases(t, s)
	items, err := s.Bases(ctx, 7)
	if err != nil || len(items) != 2 {
		t.Fatalf("items=%v err=%v", items, err)
	}
	if items[0].ID != first.ID || !items[0].IsDefault || items[1].ID != second.ID || items[1].IsDefault {
		t.Errorf("bases = %+v, want [first(default), second]", items)
	}
}

func TestDuplicateCopiesABaseAndRefusesTailoredAndForeign(t *testing.T) {
	s := NewStore(newFakeRepo())
	ctx := context.Background()
	first, _ := twoBases(t, s)

	dup, err := s.Duplicate(ctx, first.ID, 7, "First (copy)")
	if err != nil {
		t.Fatalf("duplicate: %v", err)
	}
	rec, _ := s.Get(ctx, dup.ID, 7)
	if rec.Title != "First (copy)" || rec.Document.Summary != "one" || rec.IsTailored {
		t.Errorf("duplicate = %+v, want an untailored copy of First", rec)
	}
	if def, _, _ := s.BaseCV(ctx, 7); def.ID != first.ID {
		t.Errorf("duplicating moved the default to %s", def.ID)
	}
	if _, err := s.Duplicate(ctx, first.ID, 8, "x"); !errors.Is(err, ErrNotFound) {
		t.Errorf("foreign duplicate err = %v, want ErrNotFound", err)
	}
	if _, err := s.Duplicate(ctx, uuid.New(), 7, "x"); !errors.Is(err, ErrNotFound) {
		t.Errorf("missing duplicate err = %v, want ErrNotFound", err)
	}
	_, tailored, _, err := s.Tailor(ctx, 7, 100, first.ID, "T", fakeSeeder{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Duplicate(ctx, tailored.ID, 7, "x"); !errors.Is(err, ErrNotBase) {
		t.Errorf("tailored duplicate err = %v, want ErrNotBase", err)
	}
}
