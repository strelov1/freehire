package employer

import (
	"context"
	"testing"

	"github.com/strelov1/freehire/internal/job/job"
	"github.com/strelov1/freehire/internal/job/jobderive"
)

func TestUpdateVacancy_KeepsTheAccountsCompanySlugWhenTheNameSlugsDifferently(t *testing.T) {
	jobs := newFakeJobRepo()
	repo := newFakeRepo()
	// The catalogue lists the company as "Acme Technologies" under the slug "acme".
	repo.companies["acme"] = struct{ name, website string }{name: "Acme Technologies", website: "https://acme.test"}
	s := New(repo, newFakeCodeIssuer(), &fakeClaimMailer{}, jobs, &fakeMinter{})
	if _, err := s.ClaimBySlug(context.Background(), 1, "acme", "hr@acme.test"); err != nil {
		t.Fatalf("ClaimBySlug: %v", err)
	}
	if _, err := s.ConfirmClaim(context.Background(), 1, "654321"); err != nil {
		t.Fatalf("ConfirmClaim: %v", err)
	}
	j, err := job.New(job.Draft{
		Input:                jobderive.Input{Source: jobSource, ExternalID: "https://acme.test/jobs/1", Title: "Go Engineer", Company: "Acme Technologies"},
		CanonicalCompanySlug: "acme",
	})
	if err != nil {
		t.Fatalf("seed job.New: %v", err)
	}
	slug := j.Fields().PublicSlug
	jobs.seed(slug, 1, j.Fields())

	newTitle := "Senior Go Engineer"
	if _, _, err := s.UpdateVacancy(context.Background(), 1, slug, VacancyPatch{Title: &newTitle}); err != nil {
		t.Fatalf("UpdateVacancy: %v", err)
	}
	if got := jobs.bySlug[slug].CompanySlug; got != "acme" {
		t.Errorf("CompanySlug = %q after an edit, want the account's acme", got)
	}
}
