package moderation_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/moderation"
	"github.com/strelov1/freehire/internal/normalize"
)

// fakeRepo captures the params it is handed and returns canned rows, so the tests can
// assert what the service derived without a database.
type fakeRepo struct {
	created      db.UpsertManualJobParams
	updated      db.UpdateManualJobParams
	createCalled bool
	updateCalled bool

	bySlugJob db.Job
	bySlugErr error

	ret db.Job
}

func (f *fakeRepo) Create(_ context.Context, p db.UpsertManualJobParams) (db.Job, error) {
	f.created, f.createCalled = p, true
	return f.ret, nil
}

func (f *fakeRepo) BySlug(_ context.Context, _ string) (db.Job, error) {
	return f.bySlugJob, f.bySlugErr
}

func (f *fakeRepo) Update(_ context.Context, p db.UpdateManualJobParams) (db.Job, error) {
	f.updated, f.updateCalled = p, true
	return f.ret, nil
}

func TestCreate_DerivesAndPersists(t *testing.T) {
	repo := &fakeRepo{}
	svc := moderation.New(repo)

	const url = "https://acme.example/jobs/1"
	_, err := svc.Create(context.Background(), 7, moderation.CreateInput{
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
	if got.CreatedBy != 7 || got.UpdatedBy != 7 {
		t.Errorf("audit = created %d / updated %d, want both 7", got.CreatedBy, got.UpdatedBy)
	}
	if got.Source != "manual" {
		t.Errorf("Source = %q, want manual (default when none given)", got.Source)
	}
}

func TestCreate_SourceIsRecordedAndSlugsFromIt(t *testing.T) {
	repo := &fakeRepo{}
	const url = "https://www.workatastartup.com/jobs/96572"
	_, err := moderation.New(repo).Create(context.Background(), 7, moderation.CreateInput{
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
	_, err := moderation.New(repo).Create(context.Background(), 7, moderation.CreateInput{
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
	repo := &fakeRepo{bySlugJob: db.Job{Source: "manual", ExternalID: "https://acme.example/jobs/1", Title: "Dev", PublicSlug: "s", Description: "old"}}
	evil := `<script>alert(1)</script><b>new</b>`
	_, err := moderation.New(repo).Update(context.Background(), 9, "s", moderation.UpdatePatch{Description: &evil})
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
			_, err := moderation.New(repo).Create(context.Background(), 7, tc.in)
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
		bySlugJob: db.Job{
			Source:      "manual",
			ExternalID:  "https://acme.example/jobs/1",
			Title:       "Old Title",
			Company:     "Acme",
			Location:    "Remote",
			Description: "old",
			PublicSlug:  "old-title-acme-abcd1234",
		},
	}
	svc := moderation.New(repo)

	newLoc := "Germany"
	_, err := svc.Update(context.Background(), 9, "old-title-acme-abcd1234", moderation.UpdatePatch{
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
	// Identity is preserved.
	if got.PublicSlug != "old-title-acme-abcd1234" {
		t.Errorf("PublicSlug = %q, want unchanged identity", got.PublicSlug)
	}
	if got.UpdatedBy != 9 {
		t.Errorf("UpdatedBy = %d, want 9", got.UpdatedBy)
	}
}

func TestCreate_RemoteDerivesWorkMode(t *testing.T) {
	repo := &fakeRepo{}
	_, err := moderation.New(repo).Create(context.Background(), 7, moderation.CreateInput{
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
	repo := &fakeRepo{bySlugJob: db.Job{Source: "manual"}}
	blank := ""
	_, err := moderation.New(repo).Update(context.Background(), 9, "slug", moderation.UpdatePatch{Company: &blank})
	if !errors.Is(err, moderation.ErrInvalid) {
		t.Errorf("err = %v, want ErrInvalid for a blanked company", err)
	}
	if repo.updateCalled {
		t.Error("repo.Update should not be called when validation fails")
	}
}

func TestUpdate_NotFoundPropagates(t *testing.T) {
	repo := &fakeRepo{bySlugErr: moderation.ErrJobNotFound}
	_, err := moderation.New(repo).Update(context.Background(), 9, "missing", moderation.UpdatePatch{})
	if !errors.Is(err, moderation.ErrJobNotFound) {
		t.Errorf("err = %v, want ErrJobNotFound", err)
	}
	if repo.updateCalled {
		t.Error("repo.Update should not be called when the job is not found")
	}
}
