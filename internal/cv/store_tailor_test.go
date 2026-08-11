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

func TestStoreTailorSeedsBaseFromResumeWhenAbsent(t *testing.T) {
	s := NewStore(newFakeRepo())
	ctx := context.Background()
	// Full contact block + skills: both must survive the first-time tailor seed path.
	// Contacts used to go missing when only FullName was asserted.
	seeder := fakeSeeder{ok: true, st: resumeextract.Structured{
		FullName: "Ada Lovelace",
		Email:    "ada@example.com",
		Phone:    "+44 000",
		Location: "London, UK",
		Links:    []string{"github.com/ada"},
		Summary:  "Eng",
		Skills:   []string{"Go", "PostgreSQL"},
	}}

	base, tailored, created, err := s.Tailor(ctx, 7, 100, "Tailored: X", seeder)
	if err != nil {
		t.Fatalf("tailor: %v", err)
	}
	if !created {
		t.Error("created = false, want true — this call inserted the row")
	}
	if tailored.ID == base.ID {
		t.Fatalf("tailored and base are the same row")
	}
	brec, err := s.Get(ctx, base.ID, 7)
	if err != nil {
		t.Fatalf("get base: %v", err)
	}
	wantHeader := Header{
		FullName: "Ada Lovelace", Email: "ada@example.com", Phone: "+44 000",
		Location: "London, UK", Links: []string{"github.com/ada"},
	}
	if !reflect.DeepEqual(brec.Document.Header, wantHeader) {
		t.Errorf("base header = %+v, want full contact block from résumé", brec.Document.Header)
	}
	if len(brec.Document.Skills) != 1 || !reflect.DeepEqual(brec.Document.Skills[0].Items, []string{"Go", "PostgreSQL"}) {
		t.Errorf("base skills = %+v, want Go/PostgreSQL from résumé", brec.Document.Skills)
	}
	trec, err := s.Get(ctx, tailored.ID, 7)
	if err != nil {
		t.Fatalf("get tailored: %v", err)
	}
	if !reflect.DeepEqual(trec.Document, brec.Document) {
		t.Errorf("tailored doc != base doc")
	}
	if !reflect.DeepEqual(trec.Document.Header, wantHeader) {
		t.Errorf("tailored header = %+v, want same contacts as base", trec.Document.Header)
	}
}

func TestStoreTailorRefusesWithoutResume(t *testing.T) {
	repo := newFakeRepo()
	s := NewStore(repo)
	ctx := context.Background()
	if _, _, _, err := s.Tailor(ctx, 7, 100, "T", fakeSeeder{ok: false}); !errors.Is(err, ErrNoResume) {
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

	rbase, tailored, _, err := s.Tailor(ctx, 7, 100, "Tailored", fakeSeeder{ok: false})
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

// Tailoring the SAME vacancy twice must reach the same tailored CV.
//
// The workspace opens at /tailor/<slug>, and without a `?cv` reference that address is a
// bootstrap. So every reload used to mint another copy: three CVs for one vacancy in half an
// hour on production, each with its own empty conversation, and the candidate's actual chat
// stranded on the first of them. The bootstrap is the same request either way — it has to be
// idempotent per (user, vacancy).
func TestStoreTailorReturnsTheExistingCopyForTheSameVacancy(t *testing.T) {
	repo := newFakeRepo()
	s := NewStore(repo)
	ctx := context.Background()

	if _, err := s.Create(ctx, 7, "My CV", DefaultTemplateID, Document{Summary: "base"}); err != nil {
		t.Fatalf("seed base: %v", err)
	}

	_, first, _, err := s.Tailor(ctx, 7, 100, "Tailored", fakeSeeder{ok: false})
	if err != nil {
		t.Fatalf("first tailor: %v", err)
	}
	_, second, _, err := s.Tailor(ctx, 7, 100, "Tailored", fakeSeeder{ok: false})
	if err != nil {
		t.Fatalf("second tailor: %v", err)
	}
	if first.ID != second.ID {
		t.Errorf("second bootstrap made a new CV (%s vs %s); the candidate's conversation is bound to the first", second.ID, first.ID)
	}

	// A DIFFERENT vacancy still gets its own copy.
	_, other, _, err := s.Tailor(ctx, 7, 200, "Tailored", fakeSeeder{ok: false})
	if err != nil {
		t.Fatalf("other tailor: %v", err)
	}
	if other.ID == first.ID {
		t.Error("a second vacancy reused the first vacancy's tailored CV")
	}
}

// Tailor is the only place that actually knows whether it just inserted the tailored row or
// returned an existing one — the caller used to guess from CreatedAt == UpdatedAt, which stays
// true across every idempotent reload until something else edits the CV, so it can't tell "just
// created" from "reused, never edited since." Tailor must report it directly.
func TestStoreTailorReportsWhetherItCreated(t *testing.T) {
	repo := newFakeRepo()
	s := NewStore(repo)
	ctx := context.Background()

	if _, err := s.Create(ctx, 7, "My CV", DefaultTemplateID, Document{Summary: "base"}); err != nil {
		t.Fatalf("seed base: %v", err)
	}

	_, _, created, err := s.Tailor(ctx, 7, 100, "Tailored", fakeSeeder{ok: false})
	if err != nil {
		t.Fatalf("first tailor: %v", err)
	}
	if !created {
		t.Error("first bootstrap: created = false, want true — it just inserted the row")
	}

	_, _, created, err = s.Tailor(ctx, 7, 100, "Tailored", fakeSeeder{ok: false})
	if err != nil {
		t.Fatalf("second tailor: %v", err)
	}
	if created {
		t.Error("second bootstrap: created = true, want false — it reused the existing copy")
	}
}
