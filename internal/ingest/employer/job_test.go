package employer

import (
	"context"
	"errors"
	"testing"

	"github.com/strelov1/freehire/internal/ingest/moderation"
	"github.com/strelov1/freehire/internal/job/job"
	"github.com/strelov1/freehire/internal/job/jobderive"
)

// fakeJobRepo is an in-memory JobRepository double: one job per public slug, an
// externalID->owner index standing in for the (source, external_id) unique key, and a
// closed set standing in for closed_at.
type fakeJobRepo struct {
	byExternalID map[string]int64 // externalID -> owner userID
	bySlug       map[string]job.Fields
	owner        map[string]int64 // slug -> owner userID
	closed       map[string]bool
}

func newFakeJobRepo() *fakeJobRepo {
	return &fakeJobRepo{
		byExternalID: map[string]int64{},
		bySlug:       map[string]job.Fields{},
		owner:        map[string]int64{},
		closed:       map[string]bool{},
	}
}

func (r *fakeJobRepo) Owner(_ context.Context, externalID string) (int64, bool, error) {
	id, ok := r.byExternalID[externalID]
	return id, ok, nil
}

func (r *fakeJobRepo) BySlug(_ context.Context, actorID int64, slug string) (job.Job, job.Extras, error) {
	f, ok := r.bySlug[slug]
	if !ok || r.owner[slug] != actorID {
		return job.Job{}, job.Extras{}, ErrJobNotFound
	}
	j, err := job.New(job.Draft{Input: jobderive.Input{
		Source: f.Source, ExternalID: f.ExternalID, Title: f.Title, Company: f.Company,
		Location: f.Location, Description: f.Description,
	}})
	return j, job.Extras{}, err
}

func (r *fakeJobRepo) ListMine(_ context.Context, actorID int64) ([]job.Job, []job.Extras, error) {
	var jobs []job.Job
	var extras []job.Extras
	for slug, f := range r.bySlug {
		if r.owner[slug] != actorID {
			continue
		}
		j, err := job.New(job.Draft{Input: jobderive.Input{
			Source: f.Source, ExternalID: f.ExternalID, Title: f.Title, Company: f.Company,
			Location: f.Location, Description: f.Description,
		}})
		if err != nil {
			return nil, nil, err
		}
		jobs = append(jobs, j)
		extras = append(extras, job.Extras{})
	}
	return jobs, extras, nil
}

func (r *fakeJobRepo) Update(_ context.Context, actorID int64, slug string, f job.Fields) (job.Job, job.Extras, error) {
	if r.owner[slug] != actorID {
		return job.Job{}, job.Extras{}, ErrJobNotFound
	}
	r.bySlug[slug] = f
	j, err := job.New(job.Draft{Input: jobderive.Input{
		Source: f.Source, ExternalID: f.ExternalID, Title: f.Title, Company: f.Company,
		Location: f.Location, Description: f.Description,
	}})
	return j, job.Extras{}, err
}

func (r *fakeJobRepo) Close(_ context.Context, actorID int64, slug string) error {
	if r.owner[slug] != actorID {
		return ErrJobNotFound
	}
	r.closed[slug] = true
	return nil
}

// seed installs a job directly, bypassing the Minter — for Update/Close tests that need a
// pre-existing owned vacancy.
func (r *fakeJobRepo) seed(slug string, ownerID int64, f job.Fields) {
	r.bySlug[slug] = f
	r.owner[slug] = ownerID
	r.byExternalID[f.ExternalID] = ownerID
}

// fakeMinter records what CreateVacancy asked it to mint and returns a plausible job.Job,
// standing in for moderation.Service.Create.
type fakeMinter struct {
	calls []moderation.CreateInput
	repo  *fakeJobRepo // so a mint also registers ownership, matching the real Create+Owner relationship
	actor int64
}

func (m *fakeMinter) Create(_ context.Context, actorID int64, in moderation.CreateInput) (job.Job, job.Extras, error) {
	m.calls = append(m.calls, in)
	m.actor = actorID
	j, err := job.New(job.Draft{Input: jobderive.Input{
		Source: jobSource, ExternalID: in.URL, Title: in.Title, Company: in.Company, Location: in.Location, Description: in.Description,
	}})
	if err != nil {
		return job.Job{}, job.Extras{}, err
	}
	if m.repo != nil {
		m.repo.byExternalID[in.URL] = actorID
		m.repo.owner[j.Fields().PublicSlug] = actorID
		m.repo.bySlug[j.Fields().PublicSlug] = j.Fields()
	}
	return j, job.Extras{}, nil
}

// activeService builds a Service with an already-active account for userID, wired to the
// given job repo and minter.
func activeService(t *testing.T, userID int64, jobs JobRepository, minter Minter) *Service {
	t.Helper()
	repo := newFakeRepo()
	s := New(repo, newFakeCodeIssuer(), &fakeClaimMailer{}, jobs, minter)
	repo.companies["acme"] = struct{ name, website string }{name: "Acme", website: "https://acme.test"}
	if _, err := s.Claim(context.Background(), userID, "Acme", "hr@acme.test"); err != nil {
		t.Fatalf("Claim: %v", err)
	}
	if _, err := s.ConfirmClaim(context.Background(), userID, "654321"); err != nil {
		t.Fatalf("ConfirmClaim: %v", err)
	}
	return s
}

func TestListVacancies_ScopedToTheCallersOwn(t *testing.T) {
	jobs := newFakeJobRepo()
	sA := activeService(t, 1, jobs, &fakeMinter{repo: jobs})
	sB := activeService(t, 2, jobs, &fakeMinter{repo: jobs})

	if _, _, err := sA.CreateVacancy(context.Background(), 1, VacancyInput{URL: "https://acme.test/jobs/1", Title: "Go Engineer"}); err != nil {
		t.Fatalf("A CreateVacancy: %v", err)
	}
	if _, _, err := sB.CreateVacancy(context.Background(), 2, VacancyInput{URL: "https://bravo.test/jobs/1", Title: "Rust Engineer"}); err != nil {
		t.Fatalf("B CreateVacancy: %v", err)
	}

	listA, _, err := sA.ListVacancies(context.Background(), 1)
	if err != nil {
		t.Fatalf("A ListVacancies: %v", err)
	}
	if len(listA) != 1 || listA[0].Fields().Title != "Go Engineer" {
		t.Errorf("A's list = %+v, want exactly their own Go Engineer vacancy", listA)
	}
}

func TestListVacancies_RefusesAPendingAccount(t *testing.T) {
	repo := newFakeRepo()
	s := New(repo, newFakeCodeIssuer(), &fakeClaimMailer{}, newFakeJobRepo(), &fakeMinter{})
	if _, err := s.Claim(context.Background(), 1, "Acme", "hr@notacme.test"); err != nil {
		t.Fatalf("Claim: %v", err)
	}
	if _, err := s.ConfirmClaim(context.Background(), 1, "654321"); err != nil {
		t.Fatalf("ConfirmClaim: %v", err)
	}

	if _, _, err := s.ListVacancies(context.Background(), 1); !errors.Is(err, ErrNotActive) {
		t.Fatalf("err = %v, want ErrNotActive", err)
	}
}

func TestCreateVacancy_DelegatesToTheMinterWithTheLockedCompanyName(t *testing.T) {
	jobs := newFakeJobRepo()
	minter := &fakeMinter{repo: jobs}
	s := activeService(t, 1, jobs, minter)

	_, _, err := s.CreateVacancy(context.Background(), 1, VacancyInput{URL: "https://acme.test/jobs/1", Title: "Go Engineer"})
	if err != nil {
		t.Fatalf("CreateVacancy: %v", err)
	}
	if len(minter.calls) != 1 {
		t.Fatalf("minter called %d times, want 1", len(minter.calls))
	}
	in := minter.calls[0]
	if in.Source != jobSource {
		t.Errorf("Source = %q, want %q", in.Source, jobSource)
	}
	if in.Company != "Acme" {
		t.Errorf("Company = %q, want the account's locked company name, not request content", in.Company)
	}
}

func TestCreateVacancy_RefusesAPendingAccount(t *testing.T) {
	repo := newFakeRepo()
	s := New(repo, newFakeCodeIssuer(), &fakeClaimMailer{}, newFakeJobRepo(), &fakeMinter{})
	if _, err := s.Claim(context.Background(), 1, "Acme", "hr@notacme.test"); err != nil {
		t.Fatalf("Claim: %v", err)
	}
	// No matching website configured, so ConfirmClaim leaves this pending.
	if _, err := s.ConfirmClaim(context.Background(), 1, "654321"); err != nil {
		t.Fatalf("ConfirmClaim: %v", err)
	}

	if _, _, err := s.CreateVacancy(context.Background(), 1, VacancyInput{URL: "https://acme.test/jobs/1", Title: "Go Engineer"}); !errors.Is(err, ErrNotActive) {
		t.Fatalf("err = %v, want ErrNotActive", err)
	}
}

func TestCreateVacancy_RefusesAColludingURLFromADifferentEmployer(t *testing.T) {
	jobs := newFakeJobRepo()
	minterA := &fakeMinter{repo: jobs}
	sA := activeService(t, 1, jobs, minterA)
	if _, _, err := sA.CreateVacancy(context.Background(), 1, VacancyInput{URL: "https://shared.example/jobs/1", Title: "Go Engineer"}); err != nil {
		t.Fatalf("employer A CreateVacancy: %v", err)
	}

	minterB := &fakeMinter{repo: jobs}
	sB := activeService(t, 2, jobs, minterB)
	if _, _, err := sB.CreateVacancy(context.Background(), 2, VacancyInput{URL: "https://shared.example/jobs/1", Title: "Different Title"}); !errors.Is(err, ErrURLTaken) {
		t.Fatalf("err = %v, want ErrURLTaken", err)
	}
	if len(minterB.calls) != 0 {
		t.Error("the minter must never be called for a colliding URL owned by someone else")
	}
}

func TestCreateVacancy_SameOwnerReCreateIsNotRefused(t *testing.T) {
	jobs := newFakeJobRepo()
	minter := &fakeMinter{repo: jobs}
	s := activeService(t, 1, jobs, minter)

	if _, _, err := s.CreateVacancy(context.Background(), 1, VacancyInput{URL: "https://acme.test/jobs/1", Title: "Go Engineer"}); err != nil {
		t.Fatalf("first CreateVacancy: %v", err)
	}
	if _, _, err := s.CreateVacancy(context.Background(), 1, VacancyInput{URL: "https://acme.test/jobs/1", Title: "Senior Go Engineer"}); err != nil {
		t.Fatalf("re-Create by the same owner: %v", err)
	}
	if len(minter.calls) != 2 {
		t.Errorf("minter called %d times, want 2 (both delegated, never refused)", len(minter.calls))
	}
}

func TestUpdateVacancy_MergesThePatchOntoTheCurrentContent(t *testing.T) {
	jobs := newFakeJobRepo()
	s := activeService(t, 1, jobs, &fakeMinter{})
	j, err := job.New(job.Draft{Input: jobderive.Input{
		Source: jobSource, ExternalID: "https://acme.test/jobs/1", Title: "Go Engineer", Company: "Acme", Location: "Remote",
	}})
	if err != nil {
		t.Fatalf("seed job.New: %v", err)
	}
	slug := j.Fields().PublicSlug
	jobs.seed(slug, 1, j.Fields())

	newTitle := "Senior Go Engineer"
	_, _, err = s.UpdateVacancy(context.Background(), 1, slug, VacancyPatch{Title: &newTitle})
	if err != nil {
		t.Fatalf("UpdateVacancy: %v", err)
	}
	if jobs.bySlug[slug].Title != newTitle {
		t.Errorf("Title = %q, want %q", jobs.bySlug[slug].Title, newTitle)
	}
	if jobs.bySlug[slug].Location != "Remote" {
		t.Errorf("Location = %q, want the unchanged original (nil in the patch)", jobs.bySlug[slug].Location)
	}
}

func TestUpdateVacancy_RefusesAnotherEmployersVacancy(t *testing.T) {
	jobs := newFakeJobRepo()
	sA := activeService(t, 1, jobs, &fakeMinter{})
	_ = activeService(t, 2, jobs, &fakeMinter{}) // employer B, no vacancy of its own here

	j, err := job.New(job.Draft{Input: jobderive.Input{
		Source: jobSource, ExternalID: "https://acme.test/jobs/1", Title: "Go Engineer", Company: "Acme",
	}})
	if err != nil {
		t.Fatalf("seed job.New: %v", err)
	}
	jobs.seed(j.Fields().PublicSlug, 1, j.Fields()) // owned by employer A (userID 1)

	newTitle := "Hijacked"
	if _, _, err := sA.UpdateVacancy(context.Background(), 2, j.Fields().PublicSlug, VacancyPatch{Title: &newTitle}); err == nil {
		t.Fatal("want an error: userID 2 editing userID 1's vacancy")
	}
}

func TestCloseVacancy_ClosesAnOwnedVacancy(t *testing.T) {
	jobs := newFakeJobRepo()
	s := activeService(t, 1, jobs, &fakeMinter{})
	j, err := job.New(job.Draft{Input: jobderive.Input{
		Source: jobSource, ExternalID: "https://acme.test/jobs/1", Title: "Go Engineer", Company: "Acme",
	}})
	if err != nil {
		t.Fatalf("seed job.New: %v", err)
	}
	slug := j.Fields().PublicSlug
	jobs.seed(slug, 1, j.Fields())

	if err := s.CloseVacancy(context.Background(), 1, slug); err != nil {
		t.Fatalf("CloseVacancy: %v", err)
	}
	if !jobs.closed[slug] {
		t.Error("want the vacancy closed")
	}
}

func TestCloseVacancy_RefusesAnotherEmployersVacancy(t *testing.T) {
	jobs := newFakeJobRepo()
	_ = activeService(t, 1, jobs, &fakeMinter{})
	sB := activeService(t, 2, jobs, &fakeMinter{})

	j, err := job.New(job.Draft{Input: jobderive.Input{
		Source: jobSource, ExternalID: "https://acme.test/jobs/1", Title: "Go Engineer", Company: "Acme",
	}})
	if err != nil {
		t.Fatalf("seed job.New: %v", err)
	}
	jobs.seed(j.Fields().PublicSlug, 1, j.Fields()) // owned by employer A

	if err := sB.CloseVacancy(context.Background(), 2, j.Fields().PublicSlug); !errors.Is(err, ErrJobNotFound) {
		t.Fatalf("err = %v, want ErrJobNotFound", err)
	}
	if jobs.closed[j.Fields().PublicSlug] {
		t.Error("employer B must not be able to close employer A's vacancy")
	}
}
