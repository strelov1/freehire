package employer

import (
	"context"
	"errors"
	"testing"

	"github.com/strelov1/freehire/internal/ingest/applyform"
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
	forms        map[string]applyform.Form // externalID -> stored form
	formErr      error                     // when set, SaveForm fails with it
}

func newFakeJobRepo() *fakeJobRepo {
	return &fakeJobRepo{
		byExternalID: map[string]int64{},
		bySlug:       map[string]job.Fields{},
		owner:        map[string]int64{},
		closed:       map[string]bool{},
		forms:        map[string]applyform.Form{},
	}
}

// SaveForm keys by external id: a job.New aggregate carries no persistence id, and the
// external id is the identity the fake already indexes by.
func (r *fakeJobRepo) SaveForm(_ context.Context, j job.Job, form applyform.Form) error {
	if r.formErr != nil {
		return r.formErr
	}
	r.forms[j.Fields().ExternalID] = form
	return nil
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

// Reopen undoes only a close the fake recorded, like the real query's
// closed_reason = 'employer_closed' guard.
func (r *fakeJobRepo) Reopen(_ context.Context, actorID int64, slug string) (bool, error) {
	if r.owner[slug] != actorID || !r.closed[slug] {
		return false, nil
	}
	delete(r.closed, slug)
	return true, nil
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
	externalID := in.URL
	if in.ExternalID != "" {
		externalID = in.ExternalID
	}
	j, err := job.New(job.Draft{Input: jobderive.Input{
		Source: jobSource, ExternalID: externalID, Title: in.Title, Company: in.Company, Location: in.Location, Description: in.Description,
	}})
	if err != nil {
		return job.Job{}, job.Extras{}, err
	}
	if m.repo != nil {
		m.repo.byExternalID[externalID] = actorID
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

	if _, _, err := sA.CreateVacancy(context.Background(), 1, VacancyInput{Description: "Build things.", URL: "https://acme.test/jobs/1", Title: "Go Engineer"}); err != nil {
		t.Fatalf("A CreateVacancy: %v", err)
	}
	if _, _, err := sB.CreateVacancy(context.Background(), 2, VacancyInput{Description: "Build things.", URL: "https://bravo.test/jobs/1", Title: "Rust Engineer"}); err != nil {
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

	_, _, err := s.CreateVacancy(context.Background(), 1, VacancyInput{Description: "Build things.", URL: "https://acme.test/jobs/1", Title: "Go Engineer"})
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

	if _, _, err := s.CreateVacancy(context.Background(), 1, VacancyInput{Description: "Build things.", URL: "https://acme.test/jobs/1", Title: "Go Engineer"}); !errors.Is(err, ErrNotActive) {
		t.Fatalf("err = %v, want ErrNotActive", err)
	}
}

func TestCreateVacancy_RefusesAColludingURLFromADifferentEmployer(t *testing.T) {
	jobs := newFakeJobRepo()
	minterA := &fakeMinter{repo: jobs}
	sA := activeService(t, 1, jobs, minterA)
	if _, _, err := sA.CreateVacancy(context.Background(), 1, VacancyInput{Description: "Build things.", URL: "https://shared.example/jobs/1", Title: "Go Engineer"}); err != nil {
		t.Fatalf("employer A CreateVacancy: %v", err)
	}

	minterB := &fakeMinter{repo: jobs}
	sB := activeService(t, 2, jobs, minterB)
	if _, _, err := sB.CreateVacancy(context.Background(), 2, VacancyInput{Description: "Build things.", URL: "https://shared.example/jobs/1", Title: "Different Title"}); !errors.Is(err, ErrURLTaken) {
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

	if _, _, err := s.CreateVacancy(context.Background(), 1, VacancyInput{Description: "Build things.", URL: "https://acme.test/jobs/1", Title: "Go Engineer"}); err != nil {
		t.Fatalf("first CreateVacancy: %v", err)
	}
	if _, _, err := s.CreateVacancy(context.Background(), 1, VacancyInput{Description: "Build things.", URL: "https://acme.test/jobs/1", Title: "Senior Go Engineer"}); err != nil {
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

var permitQuestion = applyform.EmployerQuestion{Label: "Do you hold an EU work permit?", Type: applyform.TypeBoolean, Required: true}

func TestCreateVacancy_OnSiteMintsItsOwnIdentityAndStoresTheQuestions(t *testing.T) {
	jobs := newFakeJobRepo()
	minter := &fakeMinter{repo: jobs}
	s := activeService(t, 1, jobs, minter)

	j, _, err := s.CreateVacancy(context.Background(), 1, VacancyInput{Description: "Build things.", ApplyMode: ApplyFreehire, Title: "Go Engineer", Questions: []applyform.EmployerQuestion{permitQuestion}})
	if err != nil {
		t.Fatalf("CreateVacancy: %v", err)
	}
	in := minter.calls[0]
	if in.URL != "" || !job.IsDirectApply(in.Source, in.ExternalID) {
		t.Fatalf("minted url/source/external id = %q/%q/%q, want no url and a direct-apply id", in.URL, in.Source, in.ExternalID)
	}
	form, ok := jobs.forms[j.Fields().ExternalID]
	if !ok {
		t.Fatal("no form stored for the vacancy")
	}
	if form.Provider != applyform.ProviderFreehire || len(form.Fields) != 1 || form.Fields[0].Label != permitQuestion.Label {
		t.Errorf("stored form = %+v", form)
	}
}

func TestCreateVacancy_TwoOnSiteVacanciesNeverShareAnIdentity(t *testing.T) {
	jobs := newFakeJobRepo()
	minter := &fakeMinter{repo: jobs}
	s := activeService(t, 1, jobs, minter)
	for range 2 {
		if _, _, err := s.CreateVacancy(context.Background(), 1, VacancyInput{Description: "Build things.", ApplyMode: ApplyFreehire, Title: "Go Engineer"}); err != nil {
			t.Fatalf("CreateVacancy: %v", err)
		}
	}
	if minter.calls[0].ExternalID == minter.calls[1].ExternalID {
		t.Errorf("both vacancies minted %q", minter.calls[0].ExternalID)
	}
}

func TestCreateVacancy_RefusesAMismatchedApplyMode(t *testing.T) {
	const d = "Build things."
	cases := map[string]VacancyInput{
		"on-site with a url":      {ApplyMode: ApplyFreehire, URL: "https://acme.test/jobs/1", Title: "T", Description: d},
		"on-site, bad questions":  {ApplyMode: ApplyFreehire, Title: "T", Description: d, Questions: []applyform.EmployerQuestion{{Label: "", Type: applyform.TypeText}}},
		"external with questions": {ApplyMode: ApplyExternal, URL: "https://acme.test/jobs/1", Title: "T", Description: d, Questions: []applyform.EmployerQuestion{permitQuestion}},
		"unknown mode":            {ApplyMode: "email", URL: "https://acme.test/jobs/1", Title: "T", Description: d},
		"default mode, no url":    {Title: "T", Description: d},
	}
	for name, in := range cases {
		t.Run(name, func(t *testing.T) {
			jobs := newFakeJobRepo()
			minter := &fakeMinter{repo: jobs}
			s := activeService(t, 1, jobs, minter)
			if _, _, err := s.CreateVacancy(context.Background(), 1, in); !errors.Is(err, ErrInvalid) {
				t.Errorf("err = %v, want ErrInvalid", err)
			}
			if len(minter.calls) != 0 {
				t.Error("a refused vacancy must not be minted")
			}
		})
	}
}

func onSiteFields(t *testing.T, externalID string) job.Fields {
	t.Helper()
	j, err := job.New(job.Draft{Input: jobderive.Input{Source: jobSource, ExternalID: externalID, Title: "Go Engineer", Company: "Acme"}})
	if err != nil {
		t.Fatal(err)
	}
	return j.Fields()
}

func TestUpdateVacancy_ReplacesTheQuestionsOfAnOnSiteVacancy(t *testing.T) {
	jobs := newFakeJobRepo()
	s := activeService(t, 1, jobs, &fakeMinter{repo: jobs})
	f := onSiteFields(t, job.DirectApplyPrefix+"a")
	jobs.seed("go-engineer", 1, f)

	other := applyform.EmployerQuestion{Label: "Notice period?", Type: applyform.TypeText}
	qs := []applyform.EmployerQuestion{other}
	if _, _, err := s.UpdateVacancy(context.Background(), 1, "go-engineer", VacancyPatch{Questions: &qs}); err != nil {
		t.Fatalf("UpdateVacancy: %v", err)
	}
	if got := jobs.forms[f.ExternalID]; len(got.Fields) != 1 || got.Fields[0].Label != "Notice period?" {
		t.Errorf("stored form = %+v", got)
	}
}

func TestUpdateVacancy_RefusesQuestionsOrAModeChange(t *testing.T) {
	external := ApplyExternal
	freehire := ApplyFreehire
	qs := []applyform.EmployerQuestion{permitQuestion}
	bad := []applyform.EmployerQuestion{{Label: "Q", Type: applyform.TypeSelect, Options: []string{"A"}}}
	cases := map[string]struct {
		externalID string
		patch      VacancyPatch
	}{
		"on-site to external":          {job.DirectApplyPrefix + "a", VacancyPatch{ApplyMode: &external}},
		"external to on-site":          {"https://acme.test/jobs/1", VacancyPatch{ApplyMode: &freehire}},
		"questions on an external one": {"https://acme.test/jobs/1", VacancyPatch{Questions: &qs}},
		"malformed questions":          {job.DirectApplyPrefix + "a", VacancyPatch{Questions: &bad}},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			jobs := newFakeJobRepo()
			s := activeService(t, 1, jobs, &fakeMinter{repo: jobs})
			jobs.seed("go-engineer", 1, onSiteFields(t, tc.externalID))
			if _, _, err := s.UpdateVacancy(context.Background(), 1, "go-engineer", tc.patch); !errors.Is(err, ErrInvalid) {
				t.Errorf("err = %v, want ErrInvalid", err)
			}
			if len(jobs.forms) != 0 {
				t.Error("a refused edit must not store a form")
			}
		})
	}
}

func TestUpdateVacancy_SameModeIsNotAChange(t *testing.T) {
	jobs := newFakeJobRepo()
	s := activeService(t, 1, jobs, &fakeMinter{repo: jobs})
	jobs.seed("go-engineer", 1, onSiteFields(t, job.DirectApplyPrefix+"a"))
	freehire := ApplyFreehire
	if _, _, err := s.UpdateVacancy(context.Background(), 1, "go-engineer", VacancyPatch{ApplyMode: &freehire}); err != nil {
		t.Errorf("restating the current mode: %v", err)
	}
}

// A vacancy whose questions could not be stored must not stay live: the employer's retry
// mints a second one (there is no URL to upsert on), and the first would be a duplicate
// asking none of the questions.
func TestCreateVacancy_OnSiteClosesTheVacancyWhenItsQuestionsCannotBeStored(t *testing.T) {
	jobs := newFakeJobRepo()
	jobs.formErr = errors.New("db down")
	minter := &fakeMinter{repo: jobs}
	s := activeService(t, 1, jobs, minter)

	_, _, err := s.CreateVacancy(context.Background(), 1, VacancyInput{Description: "Build things.", ApplyMode: ApplyFreehire, Title: "Go Engineer"})
	if err == nil {
		t.Fatal("CreateVacancy succeeded with no stored questions")
	}
	if len(jobs.closed) != 1 {
		t.Errorf("closed vacancies = %d, want the half-made one closed", len(jobs.closed))
	}
}

func TestReopenVacancy_UndoesTheEmployersOwnClose(t *testing.T) {
	jobs := newFakeJobRepo()
	s := activeService(t, 1, jobs, &fakeMinter{repo: jobs})
	jobs.seed("go-engineer", 1, onSiteFields(t, job.DirectApplyPrefix+"a"))
	if err := s.CloseVacancy(context.Background(), 1, "go-engineer"); err != nil {
		t.Fatalf("CloseVacancy: %v", err)
	}
	if err := s.ReopenVacancy(context.Background(), 1, "go-engineer"); err != nil {
		t.Fatalf("ReopenVacancy: %v", err)
	}
	if jobs.closed["go-engineer"] {
		t.Error("the vacancy is still closed")
	}
}

func TestReopenVacancy_Refusals(t *testing.T) {
	t.Run("another employer's vacancy", func(t *testing.T) {
		jobs := newFakeJobRepo()
		s := activeService(t, 2, jobs, &fakeMinter{repo: jobs})
		jobs.seed("go-engineer", 1, onSiteFields(t, job.DirectApplyPrefix+"a"))
		jobs.closed["go-engineer"] = true
		if err := s.ReopenVacancy(context.Background(), 2, "go-engineer"); !errors.Is(err, ErrJobNotFound) {
			t.Fatalf("err = %v, want ErrJobNotFound", err)
		}
	})
	t.Run("an open vacancy, or one closed by someone else", func(t *testing.T) {
		jobs := newFakeJobRepo()
		s := activeService(t, 1, jobs, &fakeMinter{repo: jobs})
		jobs.seed("go-engineer", 1, onSiteFields(t, job.DirectApplyPrefix+"a"))
		if err := s.ReopenVacancy(context.Background(), 1, "go-engineer"); !errors.Is(err, ErrInvalid) {
			t.Fatalf("err = %v, want ErrInvalid", err)
		}
	})
}

// Search keeps out a posting with no description, so an employer publishing one would see
// it vanish without a word. The form refuses it instead — on create in either mode, and on
// an edit that would blank it.
func TestVacancy_ADescriptionIsRequired(t *testing.T) {
	for name, in := range map[string]VacancyInput{
		"external, none":     {URL: "https://acme.test/jobs/1", Title: "Go Engineer"},
		"on-site, blank":     {ApplyMode: ApplyFreehire, Title: "Go Engineer", Description: "  "},
		"on-site, tags only": {ApplyMode: ApplyFreehire, Title: "Go Engineer", Description: "<p> </p>"},
	} {
		t.Run(name, func(t *testing.T) {
			jobs := newFakeJobRepo()
			minter := &fakeMinter{repo: jobs}
			s := activeService(t, 1, jobs, minter)
			if _, _, err := s.CreateVacancy(context.Background(), 1, in); !errors.Is(err, ErrInvalid) {
				t.Errorf("err = %v, want ErrInvalid", err)
			}
			if len(minter.calls) != 0 {
				t.Error("a vacancy with no description must not be minted")
			}
		})
	}

	jobs := newFakeJobRepo()
	s := activeService(t, 1, jobs, &fakeMinter{repo: jobs})
	jobs.seed("go-engineer", 1, onSiteFields(t, job.DirectApplyPrefix+"a"))
	blank := " "
	if _, _, err := s.UpdateVacancy(context.Background(), 1, "go-engineer", VacancyPatch{Description: &blank}); !errors.Is(err, ErrInvalid) {
		t.Errorf("blanking the description: err = %v, want ErrInvalid", err)
	}
}
