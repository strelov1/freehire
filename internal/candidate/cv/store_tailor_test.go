package cv

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/strelov1/freehire/internal/candidate/resumeextract"
	"github.com/strelov1/freehire/internal/platform/db"
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

	base, tailored, created, err := s.Tailor(ctx, 7, 100, "Tailored: X", seeder, nil)
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

// The résumé-seed branch is the only place Store.Tailor mints a base CV from scratch, so it
// is where saved appearance defaults must apply — see the add-cv-appearance-defaults change.
func TestStoreTailorSeedsBaseFromSavedAppearanceDefaults(t *testing.T) {
	s := NewStore(newFakeRepo())
	ctx := context.Background()

	saved := AppearanceDefaults{
		TemplateID: "timeline",
		Style:      Style{FontFamily: "carlito", FontSize: 10, LineHeight: 0.65},
		Margins:    Margins{Top: 1, Right: 1, Bottom: 1, Left: 1},
	}
	if _, err := s.SetAppearanceDefaults(ctx, 7, saved); err != nil {
		t.Fatalf("set appearance defaults: %v", err)
	}

	seeder := fakeSeeder{ok: true, st: resumeextract.Structured{FullName: "Ada Lovelace"}}
	base, _, _, err := s.Tailor(ctx, 7, 100, "Tailored: X", seeder, nil)
	if err != nil {
		t.Fatalf("tailor: %v", err)
	}
	if base.TemplateID != saved.TemplateID {
		t.Errorf("base template = %q, want saved default %q", base.TemplateID, saved.TemplateID)
	}
	brec, err := s.Get(ctx, base.ID, 7)
	if err != nil {
		t.Fatalf("get base: %v", err)
	}
	if brec.Document.Style != saved.Style {
		t.Errorf("base style = %+v, want saved default %+v", brec.Document.Style, saved.Style)
	}
	if brec.Document.Margins != saved.Margins {
		t.Errorf("base margins = %+v, want saved default %+v", brec.Document.Margins, saved.Margins)
	}
}

// Without saved appearance defaults, the résumé-seed branch must behave exactly as it did
// before this change: the system's hardcoded defaults.
func TestStoreTailorSeedsBaseFromSystemDefaultsWhenNoneSaved(t *testing.T) {
	s := NewStore(newFakeRepo())
	ctx := context.Background()

	seeder := fakeSeeder{ok: true, st: resumeextract.Structured{FullName: "Ada Lovelace"}}
	base, _, _, err := s.Tailor(ctx, 7, 100, "Tailored: X", seeder, nil)
	if err != nil {
		t.Fatalf("tailor: %v", err)
	}
	if base.TemplateID != DefaultTemplateID {
		t.Errorf("base template = %q, want system default %q", base.TemplateID, DefaultTemplateID)
	}
	brec, err := s.Get(ctx, base.ID, 7)
	if err != nil {
		t.Fatalf("get base: %v", err)
	}
	if brec.Document.Margins != DefaultMargins() {
		t.Errorf("base margins = %+v, want system default %+v", brec.Document.Margins, DefaultMargins())
	}
}

func TestStoreTailorRefusesWithoutResume(t *testing.T) {
	repo := newFakeRepo()
	s := NewStore(repo)
	ctx := context.Background()
	if _, _, _, err := s.Tailor(ctx, 7, 100, "T", fakeSeeder{ok: false}, nil); !errors.Is(err, ErrNoResume) {
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

	rbase, tailored, _, err := s.Tailor(ctx, 7, 100, "Tailored", fakeSeeder{ok: false}, nil)
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

	_, first, _, err := s.Tailor(ctx, 7, 100, "Tailored", fakeSeeder{ok: false}, nil)
	if err != nil {
		t.Fatalf("first tailor: %v", err)
	}
	_, second, _, err := s.Tailor(ctx, 7, 100, "Tailored", fakeSeeder{ok: false}, nil)
	if err != nil {
		t.Fatalf("second tailor: %v", err)
	}
	if first.ID != second.ID {
		t.Errorf("second bootstrap made a new CV (%s vs %s); the candidate's conversation is bound to the first", second.ID, first.ID)
	}

	// A DIFFERENT vacancy still gets its own copy.
	_, other, _, err := s.Tailor(ctx, 7, 200, "Tailored", fakeSeeder{ok: false}, nil)
	if err != nil {
		t.Fatalf("other tailor: %v", err)
	}
	if other.ID == first.ID {
		t.Error("a second vacancy reused the first vacancy's tailored CV")
	}
}

// racingGetTailoredRepo wraps fakeRepo to force the exact race Store.Tailor's check-then-insert
// leaves open: the first two GetTailoredForJob calls (Tailor's pre-insert existence check, one
// per concurrent caller) rendezvous before either returns, so both callers observe "no existing
// copy" and both proceed to CreateTailored — instead of the race depending on goroutine
// scheduling ever lining the two calls up. Any later call (the re-fetch a caller makes after
// losing the unique-violation race) passes straight through.
type racingGetTailoredRepo struct {
	*fakeRepo
	gate  sync.WaitGroup
	calls int32
}

func (r *racingGetTailoredRepo) GetTailoredForJob(ctx context.Context, userID, jobID int64) (db.GetTailoredCVForJobRow, error) {
	row, err := r.fakeRepo.GetTailoredForJob(ctx, userID, jobID)
	if atomic.AddInt32(&r.calls, 1) <= 2 {
		r.gate.Done()
		r.gate.Wait()
	}
	return row, err
}

// Two concurrent Tailor() calls for the same vacancy must land on exactly one tailored CV —
// the race that shipped the production incident TestStoreTailorReturnsTheExistingCopyForTheSameVacancy
// documents. The fake's CreateTailored enforces the same uniqueness cvs_user_id_job_id_tailored_uniq_idx
// (migrations/0091) does in Postgres, and Store.Tailor must resolve the resulting collision by
// re-fetching rather than surfacing it as an error.
func TestStoreTailorRacesToOneTailoredCopy(t *testing.T) {
	repo := &racingGetTailoredRepo{fakeRepo: newFakeRepo()}
	s := NewStore(repo)
	ctx := context.Background()

	if _, err := s.Create(ctx, 7, "My CV", DefaultTemplateID, Document{Summary: "base"}); err != nil {
		t.Fatalf("seed base: %v", err)
	}

	repo.gate.Add(2)
	var wg sync.WaitGroup
	results := make([]Meta, 2)
	errs := make([]error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, tailored, _, err := s.Tailor(ctx, 7, 100, "Tailored", fakeSeeder{ok: false}, nil)
			results[i], errs[i] = tailored, err
		}(i)
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Fatalf("tailor call %d: %v", i, err)
		}
	}
	if results[0].ID != results[1].ID {
		t.Errorf("concurrent Tailor() calls landed on different rows: %s vs %s", results[0].ID, results[1].ID)
	}

	repo.mu.Lock()
	tailoredCount := 0
	for _, r := range repo.rows {
		if r.jobID == 100 {
			tailoredCount++
		}
	}
	repo.mu.Unlock()
	if tailoredCount != 1 {
		t.Errorf("stored tailored CVs for job 100 = %d, want 1", tailoredCount)
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

	_, _, created, err := s.Tailor(ctx, 7, 100, "Tailored", fakeSeeder{ok: false}, nil)
	if err != nil {
		t.Fatalf("first tailor: %v", err)
	}
	if !created {
		t.Error("first bootstrap: created = false, want true — it just inserted the row")
	}

	_, _, created, err = s.Tailor(ctx, 7, 100, "Tailored", fakeSeeder{ok: false}, nil)
	if err != nil {
		t.Fatalf("second tailor: %v", err)
	}
	if created {
		t.Error("second bootstrap: created = true, want false — it reused the existing copy")
	}
}
