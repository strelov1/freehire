package experience

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/stringset"
)

// fakeRepo is an in-memory owner-scoped Repository, so the Store's rules — sanitize
// before persist, validate before write, ownership as absence — are testable without a
// database. The two guarantees a fake CANNOT prove (the unique index behind
// InsertAtomIfNew and the coalesce/nullif blanks fill) are covered by an integration
// test against a real Postgres instead.
type fakeRepo struct {
	employments map[uuid.UUID]db.ExperienceEmployment
	atoms       map[uuid.UUID]db.ExperienceAtom
	order       []uuid.UUID // insertion order, so listing is deterministic
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{
		employments: map[uuid.UUID]db.ExperienceEmployment{},
		atoms:       map[uuid.UUID]db.ExperienceAtom{},
	}
}

func stamp() pgtype.Timestamptz { return pgtype.Timestamptz{Valid: true} }

func (f *fakeRepo) ListEmployments(_ context.Context, userID int64) ([]db.ExperienceEmployment, error) {
	var out []db.ExperienceEmployment
	for _, id := range f.order {
		if e, ok := f.employments[id]; ok && e.UserID == userID {
			out = append(out, e)
		}
	}
	return out, nil
}

func (f *fakeRepo) GetEmployment(_ context.Context, id uuid.UUID, userID int64) (db.ExperienceEmployment, error) {
	if e, ok := f.employments[id]; ok && e.UserID == userID {
		return e, nil
	}
	return db.ExperienceEmployment{}, pgx.ErrNoRows
}

func (f *fakeRepo) FindEmployment(_ context.Context, userID int64, company, role string) (db.ExperienceEmployment, error) {
	for _, id := range f.order {
		e, ok := f.employments[id]
		if ok && e.UserID == userID &&
			strings.EqualFold(e.Company, company) && strings.EqualFold(e.Role, role) {
			return e, nil
		}
	}
	return db.ExperienceEmployment{}, pgx.ErrNoRows
}

func (f *fakeRepo) CreateEmployment(_ context.Context, userID int64, e Employment) (db.ExperienceEmployment, error) {
	id := uuid.New()
	row := db.ExperienceEmployment{
		ID: id, UserID: userID, Kind: e.Kind, Company: e.Company, Role: e.Role,
		Location: e.Location, PeriodStart: e.Start, PeriodEnd: e.End,
		IsCurrent: e.Current, Summary: e.Summary, Stack: e.Stack, CreatedAt: stamp(), UpdatedAt: stamp(),
	}
	f.employments[id] = row
	f.order = append(f.order, id)
	return row, nil
}

func (f *fakeRepo) UpdateEmployment(_ context.Context, id uuid.UUID, userID int64, e Employment) (db.ExperienceEmployment, error) {
	row, ok := f.employments[id]
	if !ok || row.UserID != userID {
		return db.ExperienceEmployment{}, pgx.ErrNoRows
	}
	row.Kind, row.Company, row.Role = e.Kind, e.Company, e.Role
	row.Location, row.PeriodStart, row.PeriodEnd = e.Location, e.Start, e.End
	row.IsCurrent, row.Summary, row.Stack = e.Current, e.Summary, e.Stack
	f.employments[id] = row
	return row, nil
}

func (f *fakeRepo) FillEmploymentBlanks(_ context.Context, id uuid.UUID, userID int64, e Employment) (db.ExperienceEmployment, error) {
	row, ok := f.employments[id]
	if !ok || row.UserID != userID {
		return db.ExperienceEmployment{}, pgx.ErrNoRows
	}
	row.Company = orExisting(row.Company, e.Company)
	row.Role = orExisting(row.Role, e.Role)
	row.Location = orExisting(row.Location, e.Location)
	row.PeriodStart = orExisting(row.PeriodStart, e.Start)
	row.PeriodEnd = orExisting(row.PeriodEnd, e.End)
	row.Summary = orExisting(row.Summary, e.Summary)
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
		Provenance: string(a.Provenance), SourceRef: a.SourceRef, CreatedAt: stamp(), UpdatedAt: stamp(),
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

const (
	owner    = int64(1)
	stranger = int64(2)
)

func newStore() (*Store, *fakeRepo) {
	repo := newFakeRepo()
	return NewStore(repo), repo
}

func TestStoreAddAtomSanitizesAndStamps(t *testing.T) {
	s, _ := newStore()

	got, err := s.AddAtom(context.Background(), owner, Atom{
		Claim:      "  Cut message-posting latency 20s to 1s  ",
		Skills:     []string{"k8s", "blorptech"},
		Metrics:    []string{"20s -> 1s", "  "},
		Provenance: ProvenanceStatedInChat,
	})
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

	if _, err := s.AddAtom(context.Background(), owner, Atom{Claim: "   ", Provenance: ProvenanceManual}); !errors.Is(err, ErrEmptyClaim) {
		t.Errorf("AddAtom(no claim) = %v, want ErrEmptyClaim", err)
	}
	if _, err := s.AddAtom(context.Background(), owner, Atom{Claim: "Did a thing", Provenance: "vibes"}); !errors.Is(err, ErrInvalidProvenance) {
		t.Errorf("AddAtom(bad provenance) = %v, want ErrInvalidProvenance", err)
	}
}

// The same achievement, differently spelled, is one atom. The Store reports that as a
// fact rather than an error: the caller — often a model mid-turn — should learn the
// claim is already banked, not that something went wrong.
func TestStoreAddAtomReportsAnAlreadyBankedClaim(t *testing.T) {
	s, _ := newStore()
	ctx := context.Background()

	if _, err := s.AddAtom(ctx, owner, Atom{Claim: "Cut latency 20s to 1s", Provenance: ProvenanceCVImport}); err != nil {
		t.Fatalf("first AddAtom: %v", err)
	}
	_, err := s.AddAtom(ctx, owner, Atom{Claim: "cut  latency 20s to 1s.", Provenance: ProvenanceStatedInChat})
	if !errors.Is(err, ErrAlreadyBanked) {
		t.Errorf("second AddAtom = %v, want ErrAlreadyBanked", err)
	}
}

// Two users may each bank the same claim; the key is scoped to its owner.
func TestStoreAddAtomIsScopedPerOwner(t *testing.T) {
	s, _ := newStore()
	ctx := context.Background()

	if _, err := s.AddAtom(ctx, owner, Atom{Claim: "Led the migration", Provenance: ProvenanceManual}); err != nil {
		t.Fatalf("owner AddAtom: %v", err)
	}
	if _, err := s.AddAtom(ctx, stranger, Atom{Claim: "Led the migration", Provenance: ProvenanceManual}); err != nil {
		t.Errorf("stranger AddAtom = %v, want success — the claim key is owner-scoped", err)
	}
}

func TestStoreOwnershipIsAbsence(t *testing.T) {
	s, _ := newStore()
	ctx := context.Background()

	atom, err := s.AddAtom(ctx, owner, Atom{Claim: "Cut latency", Provenance: ProvenanceManual})
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

	atom, err := s.AddAtom(ctx, owner, Atom{Claim: "Cut latency", Provenance: ProvenanceManual})
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
		Start: "2023-09", End: "Present", Current: true,
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
	}); err != nil {
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
	})
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("AddAtom with a foreign employment = %v, want ErrNotFound", err)
	}

	mine, err := s.AddAtom(ctx, owner, Atom{Claim: "Cut latency", Provenance: ProvenanceManual})
	if err != nil {
		t.Fatalf("AddAtom: %v", err)
	}
	mine.EmploymentID = &theirs.ID
	if _, err := s.UpdateAtom(ctx, mine.ID, owner, mine); !errors.Is(err, ErrNotFound) {
		t.Errorf("UpdateAtom onto a foreign employment = %v, want ErrNotFound", err)
	}

	// A ghost id is refused for the same reason, and by the same check.
	ghost := uuid.New()
	if _, err := s.AddAtom(ctx, owner, Atom{
		EmploymentID: &ghost, Claim: "Ran the cluster", Provenance: ProvenanceManual,
	}); !errors.Is(err, ErrNotFound) {
		t.Errorf("AddAtom with an unknown employment = %v, want ErrNotFound", err)
	}
}
