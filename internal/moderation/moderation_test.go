package moderation_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/job"
	"github.com/strelov1/freehire/internal/moderation"
	"github.com/strelov1/freehire/internal/normalize"
)

// fakeRepo captures the domain inputs it is handed and returns canned aggregates, so the
// tests can assert what the service derived without a database.
type fakeRepo struct {
	created     job.Fields
	createActor int64
	updated     job.Fields
	updateSlug  string
	updateActor int64

	createCalled bool
	updateCalled bool

	bySlugJob job.Job
	bySlugErr error

	ret job.Job
}

func (f *fakeRepo) Create(_ context.Context, fields job.Fields, actorID int64) (job.Job, job.Extras, error) {
	f.created, f.createActor, f.createCalled = fields, actorID, true
	return f.ret, job.Extras{}, nil
}

func (f *fakeRepo) BySlug(_ context.Context, _ string) (job.Job, job.Extras, error) {
	return f.bySlugJob, job.Extras{}, f.bySlugErr
}

func (f *fakeRepo) Update(_ context.Context, slug string, fields job.Fields, actorID int64) (job.Job, job.Extras, error) {
	f.updated, f.updateSlug, f.updateActor, f.updateCalled = fields, slug, actorID, true
	return f.ret, job.Extras{}, nil
}

// mustJob hydrates a db row into the aggregate for the BySlug fixtures (the load path used
// by Update), failing the test on a malformed row.
func mustJob(t *testing.T, r db.Job) job.Job {
	t.Helper()
	j, _, err := job.FromRow(r)
	if err != nil {
		t.Fatalf("FromRow: %v", err)
	}
	return j
}

func TestCreate_DerivesAndPersists(t *testing.T) {
	repo := &fakeRepo{}
	svc := moderation.New(repo)

	const url = "https://acme.example/jobs/1"
	_, _, err := svc.Create(context.Background(), 7, moderation.CreateInput{
		URL:         url,
		Title:       "Senior Go Developer",
		Company:     "Acme",
		Location:    "Germany",
		Description: "We use Golang.",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if !repo.createCalled {
		t.Fatal("repo.Create was not called")
	}
	got := repo.created
	if got.ExternalID != url || got.URL != url {
		t.Errorf("external_id/url = %q/%q, want both %q", got.ExternalID, got.URL, url)
	}
	if want := normalize.JobSlug("Senior Go Developer", "Acme", "manual", url); got.PublicSlug != want {
		t.Errorf("PublicSlug = %q, want %q", got.PublicSlug, want)
	}
	if got.CompanySlug != normalize.Slug("Acme") {
		t.Errorf("CompanySlug = %q", got.CompanySlug)
	}
	if len(got.Countries) == 0 || got.Countries[0] != "de" {
		t.Errorf("Countries = %v, want [de]", got.Countries)
	}
	if len(got.Skills) != 1 || got.Skills[0] != "go" {
		t.Errorf("Skills = %v, want [go]", got.Skills)
	}
	// The acting moderator is stamped onto both created_by and updated_by by the adapter.
	if repo.createActor != 7 {
		t.Errorf("actor = %d, want 7", repo.createActor)
	}
	if got.Source != "manual" {
		t.Errorf("Source = %q, want manual (default when none given)", got.Source)
	}
}

func TestCreate_SourceIsRecordedAndSlugsFromIt(t *testing.T) {
	repo := &fakeRepo{}
	const url = "https://www.workatastartup.com/jobs/96572"
	_, _, err := moderation.New(repo).Create(context.Background(), 7, moderation.CreateInput{
		URL:     url,
		Source:  "workatastartup",
		Title:   "Senior Frontend Engineer",
		Company: "Dalus",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if repo.created.Source != "workatastartup" {
		t.Errorf("Source = %q, want workatastartup", repo.created.Source)
	}
	// The public slug is minted from the real source, not the literal "manual".
	if want := normalize.JobSlug("Senior Frontend Engineer", "Dalus", "workatastartup", url); repo.created.PublicSlug != want {
		t.Errorf("PublicSlug = %q, want %q", repo.created.PublicSlug, want)
	}
}

func TestCreate_SanitizesDescription(t *testing.T) {
	// Moderator descriptions are bulk-imported from scraped pages and rendered with
	// {@html}, so the service must strip active markup before persisting (stored XSS).
	repo := &fakeRepo{}
	_, _, err := moderation.New(repo).Create(context.Background(), 7, moderation.CreateInput{
		URL:         "https://acme.example/jobs/1",
		Title:       "Dev",
		Company:     "Acme",
		Description: `<p>Build it.</p><script>alert(document.cookie)</script><img src=x onerror=alert(1)>`,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if got := repo.created.Description; strings.Contains(got, "<script>") || strings.Contains(got, "onerror") {
		t.Errorf("description not sanitized: %q", got)
	}
	if !strings.Contains(repo.created.Description, "Build it.") {
		t.Errorf("sanitizer dropped legitimate content: %q", repo.created.Description)
	}
}

func TestUpdate_SanitizesDescription(t *testing.T) {
	repo := &fakeRepo{bySlugJob: mustJob(t, db.Job{Source: "manual", ExternalID: "https://acme.example/jobs/1", Title: "Dev", PublicSlug: "s", Description: "old"})}
	evil := `<script>alert(1)</script><b>new</b>`
	_, _, err := moderation.New(repo).Update(context.Background(), 9, "s", moderation.UpdatePatch{Description: &evil})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if got := repo.updated.Description; strings.Contains(got, "<script>") {
		t.Errorf("description not sanitized on update: %q", got)
	}
}

func TestCreate_ValidationRejects(t *testing.T) {
	cases := []struct {
		name string
		in   moderation.CreateInput
	}{
		{"missing url", moderation.CreateInput{Title: "T", Company: "C"}},
		{"missing title", moderation.CreateInput{URL: "https://x/1", Company: "C"}},
		{"missing company", moderation.CreateInput{URL: "https://x/1", Title: "T"}},
		{"non-http url", moderation.CreateInput{URL: "ftp://x/1", Title: "T", Company: "C"}},
		{"relative url", moderation.CreateInput{URL: "/jobs/1", Title: "T", Company: "C"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := &fakeRepo{}
			_, _, err := moderation.New(repo).Create(context.Background(), 7, tc.in)
			if !errors.Is(err, moderation.ErrInvalid) {
				t.Errorf("err = %v, want ErrInvalid", err)
			}
			if repo.createCalled {
				t.Error("repo.Create should not be called on invalid input")
			}
		})
	}
}

func TestUpdate_MergesAndRederives(t *testing.T) {
	repo := &fakeRepo{
		bySlugJob: mustJob(t, db.Job{
			Source:      "manual",
			ExternalID:  "https://acme.example/jobs/1",
			Title:       "Old Title",
			Company:     "Acme",
			Location:    "Remote",
			Description: "old",
			PublicSlug:  "old-title-acme-abcd1234",
		}),
	}
	svc := moderation.New(repo)

	newLoc := "Germany"
	_, _, err := svc.Update(context.Background(), 9, "old-title-acme-abcd1234", moderation.UpdatePatch{
		Location: &newLoc,
	})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if !repo.updateCalled {
		t.Fatal("repo.Update was not called")
	}
	got := repo.updated
	// Unsupplied fields keep their current values.
	if got.Title != "Old Title" || got.Company != "Acme" {
		t.Errorf("unchanged fields drifted: title=%q company=%q", got.Title, got.Company)
	}
	// The edited location re-derives geography.
	if got.Location != "Germany" || len(got.Countries) == 0 || got.Countries[0] != "de" {
		t.Errorf("location/geo not re-derived: loc=%q countries=%v", got.Location, got.Countries)
	}
	// Identity is preserved: the adapter is asked to write under the original slug.
	if repo.updateSlug != "old-title-acme-abcd1234" {
		t.Errorf("update slug = %q, want unchanged identity", repo.updateSlug)
	}
	if repo.updateActor != 9 {
		t.Errorf("update actor = %d, want 9", repo.updateActor)
	}
}

func TestCreate_RemoteDerivesWorkMode(t *testing.T) {
	repo := &fakeRepo{}
	_, _, err := moderation.New(repo).Create(context.Background(), 7, moderation.CreateInput{
		URL:      "https://acme.example/jobs/r",
		Title:    "Dev",
		Company:  "Acme",
		Location: "Berlin", // no remote marker in the text
		Remote:   true,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if repo.created.WorkMode != "remote" {
		t.Errorf("WorkMode = %q, want remote (derived from the remote flag)", repo.created.WorkMode)
	}
}

func TestUpdate_RejectsBlankRequiredField(t *testing.T) {
	repo := &fakeRepo{}
	blank := ""
	_, _, err := moderation.New(repo).Update(context.Background(), 9, "slug", moderation.UpdatePatch{Company: &blank})
	if !errors.Is(err, moderation.ErrInvalid) {
		t.Errorf("err = %v, want ErrInvalid for a blanked company", err)
	}
	if repo.updateCalled {
		t.Error("repo.Update should not be called when validation fails")
	}
}

func TestUpdate_NotFoundPropagates(t *testing.T) {
	repo := &fakeRepo{bySlugErr: moderation.ErrJobNotFound}
	_, _, err := moderation.New(repo).Update(context.Background(), 9, "missing", moderation.UpdatePatch{})
	if !errors.Is(err, moderation.ErrJobNotFound) {
		t.Errorf("err = %v, want ErrJobNotFound", err)
	}
	if repo.updateCalled {
		t.Error("repo.Update should not be called when the job is not found")
	}
}
