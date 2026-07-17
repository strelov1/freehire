package cv

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/strelov1/freehire/internal/resumeextract"
)

// fakeSeeder stands in for the résumé provider (resume.Store.Structured) in unit tests.
type fakeSeeder struct {
	st resumeextract.Structured
	ok bool
}

func (f fakeSeeder) Structured(context.Context, int64) (resumeextract.Structured, bool, error) {
	return f.st, f.ok, nil
}

func TestStorePatchAppliesAndSanitizes(t *testing.T) {
	s := NewStore(newFakeRepo())
	ctx := context.Background()
	base, err := s.Create(ctx, 7, "General", DefaultTemplateID, Document{
		Experience: []ExperienceItem{{Role: "Eng", Bullets: []string{"A"}}},
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := s.Patch(ctx, base.ID, 7, Patch{Op: PatchAddBullet, Experience: 0, Value: "B"}); err != nil {
		t.Fatalf("patch: %v", err)
	}
	rec, err := s.Get(ctx, base.ID, 7)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got := rec.Document.Experience[0].Bullets; len(got) != 2 || got[1] != "B" {
		t.Errorf("bullets = %v, want [A B]", got)
	}
}

func TestStorePatchInvalidAddressingIsErrInvalidPatch(t *testing.T) {
	s := NewStore(newFakeRepo())
	ctx := context.Background()
	base, _ := s.Create(ctx, 7, "General", DefaultTemplateID, Document{Experience: []ExperienceItem{{Role: "Eng"}}})
	if _, err := s.Patch(ctx, base.ID, 7, Patch{Op: PatchReplaceBullet, Experience: 0, Bullet: 9, Value: "x"}); !errors.Is(err, ErrInvalidPatch) {
		t.Errorf("err = %v, want ErrInvalidPatch", err)
	}
}

func TestStorePatchForeignOwnerIsNotFound(t *testing.T) {
	s := NewStore(newFakeRepo())
	ctx := context.Background()
	base, _ := s.Create(ctx, 1, "Mine", DefaultTemplateID, Document{})
	if _, err := s.Patch(ctx, base.ID, 2, Patch{Op: PatchSetSummary, Value: "x"}); !errors.Is(err, ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

func TestStoreTailorSeedsBaseFromResumeWhenAbsent(t *testing.T) {
	s := NewStore(newFakeRepo())
	ctx := context.Background()
	seeder := fakeSeeder{ok: true, st: resumeextract.Structured{FullName: "Ada", Summary: "Eng", Skills: []string{"Go"}}}

	base, tailored, err := s.Tailor(ctx, 7, 100, "Tailored: X", seeder)
	if err != nil {
		t.Fatalf("tailor: %v", err)
	}
	if tailored.ID == base.ID {
		t.Fatalf("tailored and base are the same row")
	}
	brec, err := s.Get(ctx, base.ID, 7)
	if err != nil {
		t.Fatalf("get base: %v", err)
	}
	if brec.Document.Header.FullName != "Ada" {
		t.Errorf("base not seeded from résumé: %+v", brec.Document.Header)
	}
	trec, err := s.Get(ctx, tailored.ID, 7)
	if err != nil {
		t.Fatalf("get tailored: %v", err)
	}
	if !reflect.DeepEqual(trec.Document, brec.Document) {
		t.Errorf("tailored doc != base doc")
	}
}

func TestStoreTailorRefusesWithoutResume(t *testing.T) {
	repo := newFakeRepo()
	s := NewStore(repo)
	ctx := context.Background()
	if _, _, err := s.Tailor(ctx, 7, 100, "T", fakeSeeder{ok: false}); !errors.Is(err, ErrNoResume) {
		t.Errorf("err = %v, want ErrNoResume", err)
	}
	if len(repo.rows) != 0 {
		t.Errorf("no CV rows should be created on refusal, got %d", len(repo.rows))
	}
}

func TestStoreTailorUsesExistingBaseUntouched(t *testing.T) {
	s := NewStore(newFakeRepo())
	ctx := context.Background()
	base, _ := s.Create(ctx, 7, "General", DefaultTemplateID, Document{
		Summary:    "Base summary",
		Experience: []ExperienceItem{{Role: "Eng", Bullets: []string{"A"}}},
	})
	before, _ := s.Get(ctx, base.ID, 7)

	rbase, tailored, err := s.Tailor(ctx, 7, 100, "Tailored", fakeSeeder{ok: false})
	if err != nil {
		t.Fatalf("tailor: %v", err)
	}
	if rbase.ID != base.ID {
		t.Errorf("returned base %d, want existing %d", rbase.ID, base.ID)
	}
	after, _ := s.Get(ctx, base.ID, 7)
	if !reflect.DeepEqual(after.Document, before.Document) {
		t.Errorf("existing base was mutated by Tailor")
	}
	trec, _ := s.Get(ctx, tailored.ID, 7)
	if !reflect.DeepEqual(trec.Document, before.Document) {
		t.Errorf("tailored doc != base doc")
	}
}
