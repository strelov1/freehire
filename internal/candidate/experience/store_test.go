package experience

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/candidate/perioddate"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/stringset"
)

// fakeRepo is an in-memory owner-scoped Repository, so the Store's rules — sanitize
// before persist, validate before write, ownership as absence — are testable without a
// database. The two guarantees a fake CANNOT prove (the unique index behind
// InsertAtomIfNew and the coalesce/nullif blanks fill) are covered by an integration
// test against a real Postgres instead.
type fakeRepo struct {
	employments map[uuid.UUID]employmentRow
	atoms       map[uuid.UUID]db.ExperienceAtom
	order       []uuid.UUID // insertion order, so listing is deterministic

	getEmploymentCalls int   // counts GetEmployment invocations, i.e. ownsEmployment checks
	clock              int64 // monotonic tick behind stamp(), so rows get distinguishable timestamps
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{
		employments: map[uuid.UUID]employmentRow{},
		atoms:       map[uuid.UUID]db.ExperienceAtom{},
	}
}

// stamp mirrors `now()`, except monotonically ticked rather than wall-clock: two rows
// created in the same test must get comparably-ordered, distinguishable timestamps —
// real Postgres does that for free, but pgtype.Timestamptz{Valid: true} alone does not.
func (f *fakeRepo) stamp() pgtype.Timestamptz {
	f.clock++
	return pgtype.Timestamptz{Time: time.Unix(f.clock, 0), Valid: true}
}

func (f *fakeRepo) ListEmployments(_ context.Context, userID int64) ([]employmentRow, error) {
	var out []employmentRow
	for _, id := range f.order {
		if e, ok := f.employments[id]; ok && e.UserID == userID {
			out = append(out, e)
		}
	}
	return out, nil
}

func (f *fakeRepo) GetEmployment(_ context.Context, id uuid.UUID, userID int64) (employmentRow, error) {
	f.getEmploymentCalls++
	if e, ok := f.employments[id]; ok && e.UserID == userID {
		return e, nil
	}
	return employmentRow{}, pgx.ErrNoRows
}

func (f *fakeRepo) FindEmployment(_ context.Context, userID int64, company, role string) (employmentRow, error) {
	for _, id := range f.order {
		e, ok := f.employments[id]
		if ok && e.UserID == userID &&
			strings.EqualFold(e.Company, company) && strings.EqualFold(e.Role, role) {
			return e, nil
		}
	}
	return employmentRow{}, pgx.ErrNoRows
}

func (f *fakeRepo) CreateEmployment(_ context.Context, userID int64, e Employment) (employmentRow, error) {
	id := uuid.New()
	startYear, startMonth := PeriodToColumns(e.Start)
	endYear, endMonth := PeriodToColumns(e.End)
	row := employmentRow{
		ID: id, UserID: userID, Kind: e.Kind, Company: e.Company, Role: e.Role,
		Location:         e.Location,
		PeriodStartYear:  startYear,
		PeriodStartMonth: startMonth,
		PeriodEndYear:    endYear,
		PeriodEndMonth:   endMonth,
		IsCurrent:        e.Current, Summary: e.Summary, Link: e.Link, Stack: e.Stack,
		CreatedAt: f.stamp(), UpdatedAt: f.stamp(),
	}
	f.employments[id] = row
	f.order = append(f.order, id)
	return row, nil
}

func (f *fakeRepo) UpdateEmployment(_ context.Context, id uuid.UUID, userID int64, e Employment) (employmentRow, error) {
	row, ok := f.employments[id]
	if !ok || row.UserID != userID {
		return employmentRow{}, pgx.ErrNoRows
	}
	row.Kind, row.Company, row.Role = e.Kind, e.Company, e.Role
	row.Location = e.Location
	row.PeriodStartYear, row.PeriodStartMonth = PeriodToColumns(e.Start)
	row.PeriodEndYear, row.PeriodEndMonth = PeriodToColumns(e.End)
	row.IsCurrent, row.Summary, row.Link, row.Stack = e.Current, e.Summary, e.Link, e.Stack
	f.employments[id] = row
	return row, nil
}

func (f *fakeRepo) FillEmploymentBlanks(_ context.Context, id uuid.UUID, userID int64, e Employment) (employmentRow, error) {
	row, ok := f.employments[id]
	if !ok || row.UserID != userID {
		return employmentRow{}, pgx.ErrNoRows
	}
	row.Company = orExisting(row.Company, e.Company)
	row.Role = orExisting(row.Role, e.Role)
	row.Location = orExisting(row.Location, e.Location)
	// A period fills as a whole pair exactly when its year is currently unset — mirrors
	// the real query's CASE WHEN (see queries/experience.sql's own comment).
	startYear, startMonth := PeriodToColumns(e.Start)
	if !row.PeriodStartYear.Valid {
		row.PeriodStartYear, row.PeriodStartMonth = startYear, startMonth
	}
	endYear, endMonth := PeriodToColumns(e.End)
	if !row.PeriodEndYear.Valid {
		row.PeriodEndYear, row.PeriodEndMonth = endYear, endMonth
	}
	row.Summary = orExisting(row.Summary, e.Summary)
	row.Link = orExisting(row.Link, e.Link)
	row.Stack = unionSorted(row.Stack, e.Stack)
	f.employments[id] = row
	return row, nil
}

// unionSorted mirrors the query's array_agg(DISTINCT ... ORDER BY ...) union.
func unionSorted(existing, incoming []string) []string {
	set := map[string]struct{}{}
	for _, v := range append(append([]string{}, existing...), incoming...) {
		set[v] = struct{}{}
	}
	return stringset.Sorted(set)
}

func orExisting(existing, incoming string) string {
	if existing != "" {
		return existing
	}
	return incoming
}

func (f *fakeRepo) DeleteEmployment(_ context.Context, id uuid.UUID, userID int64) (int64, error) {
	if e, ok := f.employments[id]; ok && e.UserID == userID {
		delete(f.employments, id)
		for aid, a := range f.atoms {
			if a.EmploymentID != nil && *a.EmploymentID == id {
				delete(f.atoms, aid)
			}
		}
		return 1, nil
	}
	return 0, nil
}

func (f *fakeRepo) ListAtoms(_ context.Context, userID int64) ([]db.ExperienceAtom, error) {
	var out []db.ExperienceAtom
	for _, id := range f.order {
		if a, ok := f.atoms[id]; ok && a.UserID == userID {
			out = append(out, a)
		}
	}
	return out, nil
}

func (f *fakeRepo) GetAtom(_ context.Context, id uuid.UUID, userID int64) (db.ExperienceAtom, error) {
	if a, ok := f.atoms[id]; ok && a.UserID == userID {
		return a, nil
	}
	return db.ExperienceAtom{}, pgx.ErrNoRows
}

func (f *fakeRepo) InsertAtomIfNew(_ context.Context, userID int64, a Atom, claimKey string) (db.ExperienceAtom, error) {
	for _, existing := range f.atoms {
		if existing.UserID == userID && existing.ClaimKey == claimKey {
			return db.ExperienceAtom{}, pgx.ErrNoRows // the unique index would swallow it
		}
	}
	id := uuid.New()
	row := db.ExperienceAtom{
		ID: id, UserID: userID, EmploymentID: a.EmploymentID, Claim: a.Claim, ClaimKey: claimKey,
		Context: a.Context, Metrics: a.Metrics, Skills: a.Skills,
		Provenance: string(a.Provenance), SourceRef: a.SourceRef, CreatedAt: f.stamp(), UpdatedAt: f.stamp(),
	}
	f.atoms[id] = row
	f.order = append(f.order, id)
	return row, nil
}

func (f *fakeRepo) UpdateAtom(_ context.Context, id uuid.UUID, userID int64, a Atom, claimKey string) (db.ExperienceAtom, error) {
	row, ok := f.atoms[id]
	if !ok || row.UserID != userID {
		return db.ExperienceAtom{}, pgx.ErrNoRows
	}
	row.EmploymentID, row.Claim, row.ClaimKey = a.EmploymentID, a.Claim, claimKey
	row.Context, row.Metrics, row.Skills = a.Context, a.Metrics, a.Skills
	row.Provenance = string(a.Provenance)
	row.UpdatedAt = f.stamp()
	f.atoms[id] = row
	return row, nil
}

// UpdateAtomKeepingProvenance mirrors the statement it stands for: it writes the content and
// does not touch the label, so a test can tell a preserved label from a re-written one.
func (f *fakeRepo) UpdateAtomKeepingProvenance(_ context.Context, id uuid.UUID, userID int64, a Atom, claimKey string) (db.ExperienceAtom, error) {
	row, ok := f.atoms[id]
	if !ok || row.UserID != userID {
		return db.ExperienceAtom{}, pgx.ErrNoRows
	}
	row.EmploymentID, row.Claim, row.ClaimKey = a.EmploymentID, a.Claim, claimKey
	row.Context, row.Metrics, row.Skills = a.Context, a.Metrics, a.Skills
	row.UpdatedAt = f.stamp()
	f.atoms[id] = row
	return row, nil
}

func (f *fakeRepo) DeleteAtom(_ context.Context, id uuid.UUID, userID int64) (int64, error) {
	if a, ok := f.atoms[id]; ok && a.UserID == userID {
		delete(f.atoms, id)
		return 1, nil
	}
	return 0, nil
}

func (f *fakeRepo) MergeAtoms(_ context.Context, userID int64, keepID, loserID uuid.UUID, keepUpdatedAt, loserUpdatedAt time.Time, a Atom) (db.ExperienceAtom, error) {
	loser, ok := f.atoms[loserID]
	if !ok || loser.UserID != userID || !loser.UpdatedAt.Time.Equal(loserUpdatedAt) {
		return db.ExperienceAtom{}, pgx.ErrNoRows
	}
	keep, ok := f.atoms[keepID]
	if !ok || keep.UserID != userID || !keep.UpdatedAt.Time.Equal(keepUpdatedAt) {
		return db.ExperienceAtom{}, pgx.ErrNoRows
	}
	delete(f.atoms, loserID)
	keep.Context, keep.Metrics, keep.Skills = a.Context, a.Metrics, a.Skills
	keep.Provenance = string(a.Provenance)
	keep.UpdatedAt = f.stamp()
	f.atoms[keepID] = keep
	return keep, nil
}

const (
	owner    = int64(1)
	stranger = int64(2)
)

func newStore() (*Store, *fakeRepo) {
	repo := newFakeRepo()
	return NewStore(repo), repo
}

// fakeProfileSkills is a spy ProfileSkills, so the Store's sync-on-write can be asserted
// without a real userprofile.Service.
type fakeProfileSkills struct {
	calls []profileSkillsCall
	err   error
}

type profileSkillsCall struct {
	userID int64
	skills []string
}

func (f *fakeProfileSkills) MergeSkills(_ context.Context, userID int64, skills []string) error {
	f.calls = append(f.calls, profileSkillsCall{userID: userID, skills: skills})
	return f.err
}

func TestStoreAddAtomSyncsSkillsToProfile(t *testing.T) {
	s, _ := newStore()
	profile := &fakeProfileSkills{}
	s.SetProfileSkills(profile)

	if _, err := s.AddAtom(context.Background(), owner, Atom{
		Claim: "Cut latency", Skills: []string{"k8s"}, Provenance: ProvenanceManual,
	},
		AuthorCandidate,
	); err != nil {
		t.Fatalf("AddAtom: %v", err)
	}
	if len(profile.calls) != 1 {
		t.Fatalf("MergeSkills called %d times, want 1", len(profile.calls))
	}
	if profile.calls[0].userID != owner {
		t.Errorf("userID = %d, want %d", profile.calls[0].userID, owner)
	}
	if len(profile.calls[0].skills) != 1 || profile.calls[0].skills[0] != "kubernetes" {
		t.Errorf("skills = %v, want the canonicalized [kubernetes]", profile.calls[0].skills)
	}
}

func TestStoreUpdateAtomSyncsSkillsToProfile(t *testing.T) {
	s, _ := newStore()
	atom, err := s.AddAtom(context.Background(), owner, Atom{Claim: "Cut latency", Provenance: ProvenanceManual}, AuthorCandidate)
	if err != nil {
		t.Fatalf("AddAtom: %v", err)
	}

	profile := &fakeProfileSkills{}
	s.SetProfileSkills(profile)
	if _, err := s.UpdateAtom(context.Background(), atom.ID, owner, Atom{
		Claim: "Cut latency further", Skills: []string{"golang"}, Provenance: ProvenanceManual,
	},
		AuthorCandidate,
	); err != nil {
		t.Fatalf("UpdateAtom: %v", err)
	}
	if len(profile.calls) != 1 {
		t.Fatalf("MergeSkills called %d times, want 1", len(profile.calls))
	}
	if len(profile.calls[0].skills) != 1 || profile.calls[0].skills[0] != "go" {
		t.Errorf("skills = %v, want the canonicalized [go]", profile.calls[0].skills)
	}
}

func TestStoreAddAtomToleratesProfileSyncFailure(t *testing.T) {
	s, _ := newStore()
	s.SetProfileSkills(&fakeProfileSkills{err: errors.New("boom")})

	got, err := s.AddAtom(context.Background(), owner, Atom{
		Claim: "Cut latency", Skills: []string{"k8s"}, Provenance: ProvenanceManual,
	},
		AuthorCandidate,
	)
	if err != nil {
		t.Fatalf("AddAtom: %v, want the atom write to succeed despite the sync failure", err)
	}
	if got.ID == uuid.Nil {
		t.Error("stored atom has no id")
	}
}

func TestStoreAddAtomWithoutProfileSkillsDependency(t *testing.T) {
	s, _ := newStore() // no SetProfileSkills call — mirrors every other existing caller/test

	if _, err := s.AddAtom(context.Background(), owner, Atom{
		Claim: "Cut latency", Skills: []string{"k8s"}, Provenance: ProvenanceManual,
	},
		AuthorCandidate,
	); err != nil {
		t.Fatalf("AddAtom: %v, want it to work with no ProfileSkills dependency set", err)
	}
}

func TestStoreAddAtomSanitizesAndStamps(t *testing.T) {
	s, _ := newStore()

	got, err := s.AddAtom(context.Background(), owner, Atom{
		Claim:      "  Cut message-posting latency 20s to 1s  ",
		Skills:     []string{"k8s", "blorptech"},
		Metrics:    []string{"20s -> 1s", "  "},
		Provenance: ProvenanceStatedInChat,
	},
		AuthorQuoted,
	)
	if err != nil {
		t.Fatalf("AddAtom: %v", err)
	}
	if got.Claim != "Cut message-posting latency 20s to 1s" {
		t.Errorf("claim = %q, want trimmed", got.Claim)
	}
	if len(got.Skills) != 1 || got.Skills[0] != "kubernetes" {
		t.Errorf("skills = %q, want [kubernetes] — aliases canonicalize, unknowns drop", got.Skills)
	}
	if len(got.Metrics) != 1 {
		t.Errorf("metrics = %q, want the blank dropped", got.Metrics)
	}
	if got.ID == uuid.Nil {
		t.Error("stored atom has no id")
	}
}

func TestStoreAddAtomRejectsInvalid(t *testing.T) {
	s, _ := newStore()

	if _, err := s.AddAtom(context.Background(), owner, Atom{Claim: "   ", Provenance: ProvenanceManual}, AuthorCandidate); !errors.Is(err, ErrEmptyClaim) {
		t.Errorf("AddAtom(no claim) = %v, want ErrEmptyClaim", err)
	}
}

// TestAddAtomIgnoresABodySuppliedProvenance is the wall Author was introduced for. A caller
// that fills the field in — a mis-wired handler, a cmd worker, the CLI, a future assistant
// tool — must not be able to name a model's invention as something the candidate asserted, and
// so make it CV-publishable through the evidence gate.
func TestAddAtomIgnoresABodySuppliedProvenance(t *testing.T) {
	s, _ := newStore()

	got, err := s.AddAtom(context.Background(), owner,
		Atom{Claim: "Rebuilt the billing pipeline", Provenance: ProvenanceManual}, AuthorAgent)
	if err != nil {
		t.Fatalf("AddAtom: %v", err)
	}
	if got.Provenance != ProvenanceAgentInferred {
		t.Errorf("Provenance = %q, want agent_inferred — the author decides, not the struct", got.Provenance)
	}
	if got.Provenance.Publishable() {
		t.Error("a model's own reading came back CV-publishable because the caller asked for it")
	}

	// The nonsense a caller could previously smuggle in is now simply inert.
	got, err = s.AddAtom(context.Background(), owner,
		Atom{Claim: "Ran the migration", Provenance: "vibes"}, AuthorCandidate)
	if err != nil {
		t.Fatalf("AddAtom with a junk provenance: %v", err)
	}
	if got.Provenance != ProvenanceManual {
		t.Errorf("Provenance = %q, want manual", got.Provenance)
	}
}

// The same achievement, differently spelled, is one atom. The Store reports that as a
// fact rather than an error: the caller — often a model mid-turn — should learn the
// claim is already banked, not that something went wrong.
func TestStoreAddAtomReportsAnAlreadyBankedClaim(t *testing.T) {
	s, _ := newStore()
	ctx := context.Background()

	if _, err := s.AddAtom(ctx, owner, Atom{Claim: "Cut latency 20s to 1s", Provenance: ProvenanceCVImport}, AuthorCandidate); err != nil {
		t.Fatalf("first AddAtom: %v", err)
	}
	_, err := s.AddAtom(ctx, owner, Atom{Claim: "cut  latency 20s to 1s.", Provenance: ProvenanceStatedInChat}, AuthorQuoted)
	if !errors.Is(err, ErrAlreadyBanked) {
		t.Errorf("second AddAtom = %v, want ErrAlreadyBanked", err)
	}
}

// Two users may each bank the same claim; the key is scoped to its owner.
func TestStoreAddAtomIsScopedPerOwner(t *testing.T) {
	s, _ := newStore()
	ctx := context.Background()

	if _, err := s.AddAtom(ctx, owner, Atom{Claim: "Led the migration", Provenance: ProvenanceManual}, AuthorCandidate); err != nil {
		t.Fatalf("owner AddAtom: %v", err)
	}
	if _, err := s.AddAtom(ctx, stranger, Atom{Claim: "Led the migration", Provenance: ProvenanceManual}, AuthorCandidate); err != nil {
		t.Errorf("stranger AddAtom = %v, want success — the claim key is owner-scoped", err)
	}
}

func TestStoreOwnershipIsAbsence(t *testing.T) {
	s, _ := newStore()
	ctx := context.Background()

	atom, err := s.AddAtom(ctx, owner, Atom{Claim: "Cut latency", Provenance: ProvenanceManual}, AuthorCandidate)
	if err != nil {
		t.Fatalf("AddAtom: %v", err)
	}

	if _, err := s.GetAtom(ctx, atom.ID, stranger); !errors.Is(err, ErrNotFound) {
		t.Errorf("GetAtom(stranger) = %v, want ErrNotFound — never a forbidden", err)
	}
	if err := s.DeleteAtom(ctx, atom.ID, stranger); !errors.Is(err, ErrNotFound) {
		t.Errorf("DeleteAtom(stranger) = %v, want ErrNotFound", err)
	}
	if _, err := s.GetAtom(ctx, atom.ID, owner); err != nil {
		t.Errorf("the owner's atom survived a stranger's delete? GetAtom(owner) = %v", err)
	}
}

func TestStoreDeleteAtomIsTheOnlyRemoval(t *testing.T) {
	s, _ := newStore()
	ctx := context.Background()

	atom, err := s.AddAtom(ctx, owner, Atom{Claim: "Cut latency", Provenance: ProvenanceManual}, AuthorCandidate)
	if err != nil {
		t.Fatalf("AddAtom: %v", err)
	}
	if err := s.DeleteAtom(ctx, atom.ID, owner); err != nil {
		t.Fatalf("DeleteAtom: %v", err)
	}
	atoms, err := s.ListAtoms(ctx, owner)
	if err != nil {
		t.Fatalf("ListAtoms: %v", err)
	}
	if len(atoms) != 0 {
		t.Errorf("ListAtoms = %d atoms, want 0", len(atoms))
	}
}

func TestStoreCreateEmployment(t *testing.T) {
	s, _ := newStore()
	ctx := context.Background()

	got, err := s.CreateEmployment(ctx, owner, Employment{
		Kind: KindJob, Company: "  RingCentral  ", Role: "Senior Software Engineer",
		Start: &perioddate.PeriodDate{Year: 2023, Month: 9}, Current: true,
	})
	if err != nil {
		t.Fatalf("CreateEmployment: %v", err)
	}
	if got.Company != "RingCentral" {
		t.Errorf("company = %q, want trimmed", got.Company)
	}
	if got.ID == uuid.Nil {
		t.Error("stored employment has no id")
	}

	if _, err := s.CreateEmployment(ctx, owner, Employment{Kind: "hobby", Company: "X"}); !errors.Is(err, ErrInvalidKind) {
		t.Errorf("CreateEmployment(bad kind) = %v, want ErrInvalidKind", err)
	}
	if _, err := s.CreateEmployment(ctx, owner, Employment{Kind: KindJob}); !errors.Is(err, ErrEmptyEmployment) {
		t.Errorf("CreateEmployment(nothing named) = %v, want ErrEmptyEmployment", err)
	}
}

// Deleting a job takes its bullets with it: they are evidence OF that role.
func TestStoreDeleteEmploymentTakesItsAtoms(t *testing.T) {
	s, _ := newStore()
	ctx := context.Background()

	job, err := s.CreateEmployment(ctx, owner, Employment{Kind: KindJob, Company: "RingCentral", Role: "SWE"})
	if err != nil {
		t.Fatalf("CreateEmployment: %v", err)
	}
	if _, err := s.AddAtom(ctx, owner, Atom{
		EmploymentID: &job.ID, Claim: "Cut latency", Provenance: ProvenanceManual,
	},
		AuthorCandidate,
	); err != nil {
		t.Fatalf("AddAtom: %v", err)
	}

	if err := s.DeleteEmployment(ctx, job.ID, owner); err != nil {
		t.Fatalf("DeleteEmployment: %v", err)
	}
	atoms, err := s.ListAtoms(ctx, owner)
	if err != nil {
		t.Fatalf("ListAtoms: %v", err)
	}
	if len(atoms) != 0 {
		t.Errorf("ListAtoms = %d atoms, want 0 — the atoms belonged to the deleted role", len(atoms))
	}
}

// An atom may only attach to a place its own owner owns. Without the check the atom does
// not leak to the other user — it VANISHES from its own owner's view, because every read
// is scoped by user_id and the grouping then finds no place to file it under. A silently
// lost achievement is the worst failure this package has.
func TestStoreAtomCannotAttachToAnotherOwnersEmployment(t *testing.T) {
	s, _ := newStore()
	ctx := context.Background()

	theirs, err := s.CreateEmployment(ctx, stranger, Employment{
		Kind: KindJob, Company: "Someone Else", Role: "SWE",
	})
	if err != nil {
		t.Fatalf("CreateEmployment: %v", err)
	}

	_, err = s.AddAtom(ctx, owner, Atom{
		EmploymentID: &theirs.ID, Claim: "Cut latency", Provenance: ProvenanceManual,
	},
		AuthorCandidate,
	)
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("AddAtom with a foreign employment = %v, want ErrNotFound", err)
	}

	mine, err := s.AddAtom(ctx, owner, Atom{Claim: "Cut latency", Provenance: ProvenanceManual}, AuthorCandidate)
	if err != nil {
		t.Fatalf("AddAtom: %v", err)
	}
	mine.EmploymentID = &theirs.ID
	if _, err := s.UpdateAtom(ctx, mine.ID, owner, mine, AuthorCandidate); !errors.Is(err, ErrNotFound) {
		t.Errorf("UpdateAtom onto a foreign employment = %v, want ErrNotFound", err)
	}

	// A ghost id is refused for the same reason, and by the same check.
	ghost := uuid.New()
	if _, err := s.AddAtom(ctx, owner, Atom{
		EmploymentID: &ghost, Claim: "Ran the cluster", Provenance: ProvenanceManual,
	},
		AuthorCandidate,
	); !errors.Is(err, ErrNotFound) {
		t.Errorf("AddAtom with an unknown employment = %v, want ErrNotFound", err)
	}
}

// Save-as-project on the Experience UI: create a project employment, then attach an
// unplaced atom. SeedHistory must then list the claim under projects, not as placeless.
func TestStorePromoteUnplacedAtomToProject(t *testing.T) {
	s, _ := newStore()
	ctx := context.Background()

	atom, err := s.AddAtom(ctx, owner, Atom{
		Claim:      "Reverse-engineered game data for a Portuguese localization mod",
		Provenance: ProvenanceStatedInChat,
	},
		AuthorQuoted,
	)
	if err != nil {
		t.Fatalf("AddAtom: %v", err)
	}

	project, err := s.CreateEmployment(ctx, owner, Employment{
		Kind: KindProject, Company: "My Time at Sandrock", Link: "https://www.nexusmods.com/example",
	})
	if err != nil {
		t.Fatalf("CreateEmployment: %v", err)
	}

	atom.EmploymentID = &project.ID
	if _, err := s.UpdateAtom(ctx, atom.ID, owner, atom, AuthorCandidate); err != nil {
		t.Fatalf("UpdateAtom attach: %v", err)
	}

	hist, err := s.SeedHistory(ctx, owner)
	if err != nil {
		t.Fatalf("SeedHistory: %v", err)
	}
	if !hist.HasProjectEmployments || len(hist.Projects) != 1 {
		t.Fatalf("projects = %+v, want one banked project", hist.Projects)
	}
	if hist.Projects[0].Name != "My Time at Sandrock" || hist.Projects[0].Link != "https://www.nexusmods.com/example" {
		t.Errorf("project identity = %+v", hist.Projects[0])
	}
	if len(hist.Projects[0].Highlights) != 1 || !strings.Contains(hist.Projects[0].Highlights[0], "Portuguese") {
		t.Errorf("project highlights = %q, want the attached claim", hist.Projects[0].Highlights)
	}
	for _, e := range hist.Experience {
		if e.Company == "" && e.Title == "" && len(e.Highlights) > 0 {
			t.Fatalf("placeless experience still present: %+v", e)
		}
	}
}

func TestStoreMergeAtomsUnionsAndDeletesLoser(t *testing.T) {
	s, _ := newStore()
	ctx := context.Background()

	a, err := s.AddAtom(ctx, owner, Atom{
		Claim:  "Built a Chromium plugin with faster-whisper for live and batch",
		Skills: []string{"python"}, Provenance: ProvenanceAgentInferred,
	},
		AuthorAgent,
	)
	if err != nil {
		t.Fatalf("AddAtom a: %v", err)
	}
	b, err := s.AddAtom(ctx, owner, Atom{
		Claim:   "Built a Chromium plugin with VAD filtering on faster-whisper",
		Context: "small/medium/large-v3 model profiles", Metrics: []string{"VAD"},
		Skills: []string{"nlp"}, Provenance: ProvenanceAgentInferred,
	},
		AuthorAgent,
	)
	if err != nil {
		t.Fatalf("AddAtom b: %v", err)
	}

	kept, err := s.MergeAtoms(ctx, owner, a.ID, b.ID)
	if err != nil {
		t.Fatalf("MergeAtoms: %v", err)
	}
	if kept.Context == "" || len(kept.Metrics) == 0 {
		t.Errorf("kept missing richness: context=%q metrics=%q", kept.Context, kept.Metrics)
	}
	if ClaimKey(kept.Claim) == "" {
		t.Error("claim emptied")
	}

	atoms, err := s.ListAtoms(ctx, owner)
	if err != nil {
		t.Fatalf("ListAtoms: %v", err)
	}
	if len(atoms) != 1 {
		t.Fatalf("atoms after merge = %d, want 1", len(atoms))
	}
	if atoms[0].ID != kept.ID {
		t.Errorf("surviving id = %s, want keep %s", atoms[0].ID, kept.ID)
	}
}

// raceInjectingRepo wraps fakeRepo and, the moment Store.MergeAtoms' GetAtom(trigger)
// returns, simulates a write landing in the exact gap MergeAtoms cannot close on its own:
// keep/lose selection and the merged-fields union happen in Go, between the two reads and
// the eventual write, with no transaction holding the rows still. victim is mutated to
// stand in for that concurrent write.
type raceInjectingRepo struct {
	*fakeRepo
	trigger uuid.UUID
	victim  uuid.UUID
}

func (r *raceInjectingRepo) GetAtom(ctx context.Context, id uuid.UUID, userID int64) (db.ExperienceAtom, error) {
	row, err := r.fakeRepo.GetAtom(ctx, id, userID)
	if id == r.trigger {
		v := r.atoms[r.victim]
		v.Context = "raced in from another request"
		v.UpdatedAt = r.stamp()
		r.atoms[r.victim] = v
	}
	return row, err
}

// The race the review flagged: a write landing between Store.MergeAtoms' reads and its
// write must not be silently discarded by an update computed from the now-stale snapshot.
func TestStoreMergeAtomsRejectsAConcurrentWriteInTheReadWriteGap(t *testing.T) {
	s, repo := newStore()
	ctx := context.Background()

	a, err := s.AddAtom(ctx, owner, Atom{Claim: "Did the thing", Provenance: ProvenanceManual}, AuthorCandidate)
	if err != nil {
		t.Fatalf("AddAtom a: %v", err)
	}
	b, err := s.AddAtom(ctx, owner, Atom{Claim: "Did the other thing", Provenance: ProvenanceManual}, AuthorCandidate)
	if err != nil {
		t.Fatalf("AddAtom b: %v", err)
	}

	racy := NewStore(&raceInjectingRepo{fakeRepo: repo, trigger: b.ID, victim: a.ID})
	if _, err := racy.MergeAtoms(ctx, owner, a.ID, b.ID); !errors.Is(err, ErrMergeConflict) {
		t.Fatalf("MergeAtoms racing a concurrent edit = %v, want ErrMergeConflict", err)
	}

	// The concurrent edit must survive untouched — not overwritten by a merge built from
	// the snapshot read just before it landed.
	got, err := s.GetAtom(ctx, a.ID, owner)
	if err != nil {
		t.Fatalf("GetAtom a: %v", err)
	}
	if got.Context != "raced in from another request" {
		t.Errorf("context = %q, want the concurrent edit preserved, not clobbered by the aborted merge", got.Context)
	}
	// The would-be loser must survive too — an aborted merge deletes nothing.
	if _, err := s.GetAtom(ctx, b.ID, owner); err != nil {
		t.Errorf("GetAtom b: %v, want the loser to survive an aborted merge", err)
	}
}

func TestStoreMergeAtomsNotFoundAndCrossEmployment(t *testing.T) {
	s, _ := newStore()
	ctx := context.Background()

	roleA, err := s.CreateEmployment(ctx, owner, Employment{Kind: KindJob, Company: "A", Role: "SWE"})
	if err != nil {
		t.Fatalf("CreateEmployment A: %v", err)
	}
	roleB, err := s.CreateEmployment(ctx, owner, Employment{Kind: KindJob, Company: "B", Role: "SWE"})
	if err != nil {
		t.Fatalf("CreateEmployment B: %v", err)
	}
	a, err := s.AddAtom(ctx, owner, Atom{
		EmploymentID: &roleA.ID, Claim: "Did the thing at A", Provenance: ProvenanceManual,
	},
		AuthorCandidate,
	)
	if err != nil {
		t.Fatalf("AddAtom a: %v", err)
	}
	b, err := s.AddAtom(ctx, owner, Atom{
		EmploymentID: &roleB.ID, Claim: "Did the thing at B", Provenance: ProvenanceManual,
	},
		AuthorCandidate,
	)
	if err != nil {
		t.Fatalf("AddAtom b: %v", err)
	}

	if _, err := s.MergeAtoms(ctx, owner, a.ID, b.ID); !errors.Is(err, ErrMergeCrossEmployment) {
		t.Errorf("cross employment = %v, want ErrMergeCrossEmployment", err)
	}
	if _, err := s.MergeAtoms(ctx, owner, a.ID, uuid.New()); !errors.Is(err, ErrNotFound) {
		t.Errorf("missing = %v, want ErrNotFound", err)
	}
	if _, err := s.MergeAtoms(ctx, stranger, a.ID, b.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("not yours = %v, want ErrNotFound", err)
	}
	if _, err := s.MergeAtoms(ctx, owner, a.ID, a.ID); !errors.Is(err, ErrInvalidMerge) {
		t.Errorf("same id = %v, want ErrInvalidMerge", err)
	}
}

// TestRewriteKeepsTheStoredLabelWithoutReadingIt is the atomicity half of the anti-laundering
// rule. AuthorRewrite must reach a statement that leaves provenance alone, not read it and
// write it back: a read-then-write leaves a window in which a concurrent write lands and the
// rewrite restores a label it never saw.
//
// The fake proves it structurally — UpdateAtomKeepingProvenance never touches the column, so
// if the store took the reading path the assertion below would see the caller's value instead.
func TestRewriteKeepsTheStoredLabelWithoutReadingIt(t *testing.T) {
	s, _ := newStore()
	ctx := context.Background()

	// Banked by the agent: unpublishable, which is the label a laundering edit would want gone.
	atom, err := s.AddAtom(ctx, owner, Atom{Claim: "Ran the migration"}, AuthorAgent)
	if err != nil {
		t.Fatalf("AddAtom: %v", err)
	}
	if atom.Provenance != ProvenanceAgentInferred {
		t.Fatalf("seed provenance = %q, want agent_inferred", atom.Provenance)
	}

	// A keyed rewrite, asking loudly for a promotion it must not get.
	atom.Claim = "Ran the migration across three regions"
	atom.Provenance = ProvenanceManual
	got, err := s.UpdateAtom(ctx, atom.ID, owner, atom, AuthorRewrite)
	if err != nil {
		t.Fatalf("UpdateAtom: %v", err)
	}
	if got.Claim != "Ran the migration across three regions" {
		t.Errorf("Claim = %q, want the rewritten words — a rewrite still rewrites", got.Claim)
	}
	if got.Provenance != ProvenanceAgentInferred {
		t.Errorf("Provenance = %q, want agent_inferred untouched", got.Provenance)
	}
	if got.Provenance.Publishable() {
		t.Error("a rewrite promoted a model's reading to CV-publishable")
	}
}

// TestRewriteIgnoresAJunkProvenanceRatherThanRefusing closes the gap the atomic rewrite opened:
// with the label no longer overwritten before Validate, a caller's nonsense survived long
// enough to be rejected. "Inert" has to mean inert — the field is not written, so it is not
// judged either, and the stored standing is untouched.
func TestRewriteIgnoresAJunkProvenanceRatherThanRefusing(t *testing.T) {
	s, _ := newStore()
	ctx := context.Background()

	atom, err := s.AddAtom(ctx, owner, Atom{Claim: "Ran the migration"}, AuthorAgent)
	if err != nil {
		t.Fatalf("AddAtom: %v", err)
	}

	atom.Claim = "Ran the migration across three regions"
	atom.Provenance = "vibes"
	got, err := s.UpdateAtom(ctx, atom.ID, owner, atom, AuthorRewrite)
	if err != nil {
		t.Fatalf("a junk provenance must be ignored, not refused: %v", err)
	}
	if got.Claim != "Ran the migration across three regions" {
		t.Errorf("Claim = %q, want the rewritten words", got.Claim)
	}
	if got.Provenance != ProvenanceAgentInferred {
		t.Errorf("Provenance = %q, want the stored agent_inferred", got.Provenance)
	}
}
