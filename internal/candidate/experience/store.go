package experience

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/platform/db"
)

// Store-level sentinel errors. Both are facts about the bank rather than failures, and
// both are worded for a model to read: the assistant tools return them verbatim, and
// that message is the model's only route to correcting itself within a turn.
var (
	// ErrNotFound is an entry that does not exist OR belongs to another user. The two are
	// deliberately indistinguishable — ownership is absence, never a forbidden, so a probe
	// cannot use the error to learn that an id is real.
	ErrNotFound = errors.New("experience: no such entry")
	// ErrAlreadyBanked is a claim the owner has already recorded, under any spelling.
	ErrAlreadyBanked = errors.New("experience: this claim is already in the bank")
)

// Repository is the owner-scoped persistence the Store needs, narrow enough to fake in a
// unit test. Every method takes the owner's id, so there is no path that reads or writes
// a row without one.
type Repository interface {
	ListEmployments(ctx context.Context, userID int64) ([]employmentRow, error)
	GetEmployment(ctx context.Context, id uuid.UUID, userID int64) (employmentRow, error)
	FindEmployment(ctx context.Context, userID int64, company, role string) (employmentRow, error)
	CreateEmployment(ctx context.Context, userID int64, e Employment) (employmentRow, error)
	UpdateEmployment(ctx context.Context, id uuid.UUID, userID int64, e Employment) (employmentRow, error)
	FillEmploymentBlanks(ctx context.Context, id uuid.UUID, userID int64, e Employment) (employmentRow, error)
	DeleteEmployment(ctx context.Context, id uuid.UUID, userID int64) (int64, error)

	ListAtoms(ctx context.Context, userID int64) ([]db.ExperienceAtom, error)
	GetAtom(ctx context.Context, id uuid.UUID, userID int64) (db.ExperienceAtom, error)
	InsertAtomIfNew(ctx context.Context, userID int64, a Atom, claimKey string) (db.ExperienceAtom, error)
	UpdateAtom(ctx context.Context, id uuid.UUID, userID int64, a Atom, claimKey string) (db.ExperienceAtom, error)
	// UpdateAtomKeepingProvenance writes the same content but leaves the stored provenance
	// untouched, in one statement. It exists so AuthorRewrite needs no read: see UpdateAtom.
	UpdateAtomKeepingProvenance(ctx context.Context, id uuid.UUID, userID int64, a Atom, claimKey string) (db.ExperienceAtom, error)
	DeleteAtom(ctx context.Context, id uuid.UUID, userID int64) (int64, error)
	// MergeAtoms writes only when both rows' updated_at still match what the caller read —
	// see Store.MergeAtoms and the query's own comment for why.
	MergeAtoms(ctx context.Context, userID int64, keepID, loserID uuid.UUID, keepUpdatedAt, loserUpdatedAt time.Time, a Atom) (db.ExperienceAtom, error)
}

// ProfileSkills is the narrow slice of userprofile the bank needs: fold newly banked
// skills into an existing profile. A user with no profile is not an error — there is
// nothing to fold into, and this must never create one. Kept as an interface (rather than
// importing internal/identity/userprofile directly) so the bank does not need to know the
// profile's full surface just to pass along a skill list — the same reasoning behind
// cvedit.EvidenceGate.
type ProfileSkills interface {
	MergeSkills(ctx context.Context, userID int64, skills []string) error
}

// Store is the bank's domain surface: it sanitizes and validates before anything is
// persisted, derives the claim key, and turns rows back into domain values.
type Store struct {
	repo Repository
	// profileSkills is optional and nil-safe: unset, atom writes behave exactly as they
	// did before this dependency existed.
	profileSkills ProfileSkills
}

// NewStore builds a Store over an owner-scoped Repository.
func NewStore(repo Repository) *Store { return &Store{repo: repo} }

// SetProfileSkills wires the profile-sync dependency. Optional — a Store this is never
// called on keeps working exactly as before.
func (s *Store) SetProfileSkills(p ProfileSkills) *Store {
	s.profileSkills = p
	return s
}

// syncProfileSkills folds a banked atom's skills into the owner's search profile,
// best-effort: the atom is already durably persisted, which matters more than this
// courtesy update, so a failure here is logged and never returned to the caller.
func (s *Store) syncProfileSkills(ctx context.Context, userID int64, skills []string) {
	if s.profileSkills == nil || len(skills) == 0 {
		return
	}
	if err := s.profileSkills.MergeSkills(ctx, userID, skills); err != nil {
		log.Printf("experience: profile skill sync failed for user %d: %v", userID, err)
	}
}

// ListEmployments returns the owner's places of work in reverse chronological order
// (current roles first). The SQL query already orders on the structured period columns
// (migration 0135), but Store re-sorts in Go too so a fake Repository (see fakeRepo,
// which returns rows in plain insertion order) and the real one agree on ordering, and
// so a tie beyond the date itself resolves deterministically.
func (s *Store) ListEmployments(ctx context.Context, userID int64) ([]Employment, error) {
	rows, err := s.repo.ListEmployments(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("list employments: %w", err)
	}
	out := make([]Employment, 0, len(rows))
	for _, row := range rows {
		out = append(out, employmentFromRow(row))
	}
	sortEmploymentsChronological(out)
	return out, nil
}

// GetEmployment returns one employment the caller owns.
func (s *Store) GetEmployment(ctx context.Context, id uuid.UUID, userID int64) (Employment, error) {
	row, err := s.repo.GetEmployment(ctx, id, userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return Employment{}, ErrNotFound
	}
	if err != nil {
		return Employment{}, fmt.Errorf("get employment: %w", err)
	}
	return employmentFromRow(row), nil
}

// FindEmployment returns the owner's employment with this company and role, compared
// case-insensitively. It is import's match, and ErrNotFound simply means "create one".
func (s *Store) FindEmployment(ctx context.Context, userID int64, company, role string) (Employment, error) {
	row, err := s.repo.FindEmployment(ctx, userID, company, role)
	if errors.Is(err, pgx.ErrNoRows) {
		return Employment{}, ErrNotFound
	}
	if err != nil {
		return Employment{}, fmt.Errorf("find employment: %w", err)
	}
	return employmentFromRow(row), nil
}

// CreateEmployment sanitizes, validates and persists a new place of work.
func (s *Store) CreateEmployment(ctx context.Context, userID int64, e Employment) (Employment, error) {
	e.Sanitize()
	if err := e.Validate(); err != nil {
		return Employment{}, err
	}
	row, err := s.repo.CreateEmployment(ctx, userID, e)
	if err != nil {
		return Employment{}, fmt.Errorf("create employment: %w", err)
	}
	return employmentFromRow(row), nil
}

// UpdateEmployment replaces an owned employment's fields with what the user typed,
// blanks included — this is the profile UI's edit, where the user means what they wrote.
func (s *Store) UpdateEmployment(ctx context.Context, id uuid.UUID, userID int64, e Employment) (Employment, error) {
	e.Sanitize()
	if err := e.Validate(); err != nil {
		return Employment{}, err
	}
	row, err := s.repo.UpdateEmployment(ctx, id, userID, e)
	if errors.Is(err, pgx.ErrNoRows) {
		return Employment{}, ErrNotFound
	}
	if err != nil {
		return Employment{}, fmt.Errorf("update employment: %w", err)
	}
	return employmentFromRow(row), nil
}

// FillEmploymentBlanks writes only into the fields the bank has nothing for. It is
// import's write, and the asymmetry with UpdateEmployment is the point: a CV must not
// undo a correction its owner made by hand.
func (s *Store) FillEmploymentBlanks(ctx context.Context, id uuid.UUID, userID int64, e Employment) (Employment, error) {
	e.Sanitize()
	row, err := s.repo.FillEmploymentBlanks(ctx, id, userID, e)
	if errors.Is(err, pgx.ErrNoRows) {
		return Employment{}, ErrNotFound
	}
	if err != nil {
		return Employment{}, fmt.Errorf("fill employment blanks: %w", err)
	}
	return employmentFromRow(row), nil
}

// DeleteEmployment removes an owned employment and, with it, the atoms that were
// evidence of that role.
func (s *Store) DeleteEmployment(ctx context.Context, id uuid.UUID, userID int64) error {
	n, err := s.repo.DeleteEmployment(ctx, id, userID)
	if err != nil {
		return fmt.Errorf("delete employment: %w", err)
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// ListAtoms returns every atom the owner has. Retrieval scores the whole set, so this is
// the read it is built on.
func (s *Store) ListAtoms(ctx context.Context, userID int64) ([]Atom, error) {
	rows, err := s.repo.ListAtoms(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("list atoms: %w", err)
	}
	out := make([]Atom, 0, len(rows))
	for _, row := range rows {
		out = append(out, atomFromRow(row))
	}
	return out, nil
}

// GetAtom returns one atom the caller owns.
func (s *Store) GetAtom(ctx context.Context, id uuid.UUID, userID int64) (Atom, error) {
	row, err := s.repo.GetAtom(ctx, id, userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return Atom{}, ErrNotFound
	}
	if err != nil {
		return Atom{}, fmt.Errorf("get atom: %w", err)
	}
	return atomFromRow(row), nil
}

// AddAtom sanitizes, validates and banks a piece of evidence. A claim the owner already
// recorded — under any spelling — comes back as ErrAlreadyBanked rather than a second
// row: the unique index on (user_id, claim_key) swallows the insert, and the absent row
// is the signal.
func (s *Store) AddAtom(ctx context.Context, userID int64, a Atom, author Author) (Atom, error) {
	// The label is DERIVED from who is asserting the claim, never taken from the value the
	// caller built. See Author: this is the wall the CV evidence gate stands on, and a
	// struct field a caller can fill in is not a wall.
	a.Provenance = ProvenanceFor(author, "")
	a.Sanitize()
	if err := a.Validate(); err != nil {
		return Atom{}, err
	}
	if err := s.ownsEmployment(ctx, userID, a.EmploymentID); err != nil {
		return Atom{}, err
	}
	return s.insertAtom(ctx, userID, a)
}

// insertAtom writes a sanitized, validated, ownership-checked atom. Split out of AddAtom so
// Import — which reconciles the employment once per entry and already knows the resulting
// id is the caller's own — can bank every claim under it without AddAtom's ownsEmployment
// re-verifying the same id on every single bullet.
func (s *Store) insertAtom(ctx context.Context, userID int64, a Atom) (Atom, error) {
	row, err := s.repo.InsertAtomIfNew(ctx, userID, a, ClaimKey(a.Claim))
	if errors.Is(err, pgx.ErrNoRows) {
		return Atom{}, ErrAlreadyBanked
	}
	if err != nil {
		return Atom{}, fmt.Errorf("add atom: %w", err)
	}
	added := atomFromRow(row)
	s.syncProfileSkills(ctx, userID, added.Skills)
	return added, nil
}

// UpdateAtom replaces an owned atom's content. The claim key moves with the claim, so
// the uniqueness guarantee holds after an edit as well as after an insert.
func (s *Store) UpdateAtom(ctx context.Context, id uuid.UUID, userID int64, a Atom, author Author) (Atom, error) {
	// AuthorRewrite keeps the standing the claim already had, and it does so WITHOUT reading
	// it: the statement simply omits the column. Reading the label and writing it back would
	// leave a window in which a concurrent write lands and the rewrite then restores a label
	// it never saw — the same laundering, taken at a different door. A corrupt stored label is
	// safe to leave alone, because Provenance.Publishable fails closed on anything it does not
	// recognise.
	keepStoredLabel := author == AuthorRewrite
	if keepStoredLabel {
		// Nothing writes this column on the rewrite path, so the caller's value must not be
		// JUDGED either — "a body-supplied provenance is inert" has to mean inert, not
		// "rejected on a technicality" by Validate below. It is normalised to what an
		// unlabelled rewrite means; the statement omits the column, so this never reaches
		// the row.
		a.Provenance = ProvenanceFor(AuthorRewrite, "")
	} else {
		a.Provenance = ProvenanceFor(author, "")
	}

	a.Sanitize()
	if err := a.Validate(); err != nil {
		return Atom{}, err
	}
	if err := s.ownsEmployment(ctx, userID, a.EmploymentID); err != nil {
		return Atom{}, err
	}

	write := s.repo.UpdateAtom
	if keepStoredLabel {
		write = s.repo.UpdateAtomKeepingProvenance
	}
	row, err := write(ctx, id, userID, a, ClaimKey(a.Claim))
	if errors.Is(err, pgx.ErrNoRows) {
		return Atom{}, ErrNotFound
	}
	if err != nil {
		return Atom{}, fmt.Errorf("update atom: %w", err)
	}
	updated := atomFromRow(row)
	s.syncProfileSkills(ctx, userID, updated.Skills)
	return updated, nil
}

// DeleteAtom removes an owned atom. This is the only path that takes evidence out of the
// bank — import never deletes.
func (s *Store) DeleteAtom(ctx context.Context, id uuid.UUID, userID int64) error {
	n, err := s.repo.DeleteAtom(ctx, id, userID)
	if err != nil {
		return fmt.Errorf("delete atom: %w", err)
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// MergeAtoms folds two owned atoms into one: the richer keep absorbs the loser's
// metrics, skills and richer context, then the loser is deleted. One CTE write
// makes the two halves atomic. Claim text stays on the keep — chat may rewrite it
// afterward with UpdateAtom if the owner wants a richer sentence.
func (s *Store) MergeAtoms(ctx context.Context, userID int64, aID, bID uuid.UUID) (Atom, error) {
	if aID == uuid.Nil || bID == uuid.Nil || aID == bID {
		return Atom{}, ErrInvalidMerge
	}
	rowA, err := s.repo.GetAtom(ctx, aID, userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return Atom{}, ErrNotFound
	}
	if err != nil {
		return Atom{}, fmt.Errorf("get atom a: %w", err)
	}
	rowB, err := s.repo.GetAtom(ctx, bID, userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return Atom{}, ErrNotFound
	}
	if err != nil {
		return Atom{}, fmt.Errorf("get atom b: %w", err)
	}

	left := mergeCandidate{Atom: atomFromRow(rowA), CreatedAt: timestamptz(rowA.CreatedAt), UpdatedAt: timestamptz(rowA.UpdatedAt)}
	right := mergeCandidate{Atom: atomFromRow(rowB), CreatedAt: timestamptz(rowB.CreatedAt), UpdatedAt: timestamptz(rowB.UpdatedAt)}
	if err := validateMergePair(aID, bID, left.Atom, right.Atom); err != nil {
		return Atom{}, err
	}

	keep, lose := left, right
	if !chooseKeep(left, right) {
		keep, lose = right, left
	}
	merged := unionForMerge(keep.Atom, lose.Atom)
	if err := merged.Validate(); err != nil {
		return Atom{}, err
	}

	// keep.UpdatedAt/lose.UpdatedAt pin the write to the exact rows just read: the choice of
	// keep/lose and the union above happen here in Go, not inside the write's transaction,
	// so a write landing in that gap must not be silently overwritten by an update computed
	// from a now-stale snapshot (see the query's own comment).
	row, err := s.repo.MergeAtoms(ctx, userID, keep.ID, lose.ID, keep.UpdatedAt, lose.UpdatedAt, merged)
	if errors.Is(err, pgx.ErrNoRows) {
		return Atom{}, ErrMergeConflict
	}
	if err != nil {
		return Atom{}, fmt.Errorf("merge atoms: %w", err)
	}
	return atomFromRow(row), nil
}

// ownsEmployment refuses an atom pointed at a place its owner does not own — including one
// that does not exist. The foreign key alone does not cover this: it references
// experience_employments(id) with no user in it.
//
// The damage this prevents is not a leak but a disappearance. Every read is scoped by
// user_id, so an atom attached to someone else's place is absent from its owner's
// employment list, files under no place when the surfaces group by it, and drops out of
// their view entirely — a silently lost achievement, which is the worst thing this package
// can do to someone.
func (s *Store) ownsEmployment(ctx context.Context, userID int64, employmentID *uuid.UUID) error {
	if employmentID == nil {
		return nil
	}
	_, err := s.repo.GetEmployment(ctx, *employmentID, userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("verify employment: %w", err)
	}
	return nil
}

func employmentFromRow(row employmentRow) Employment {
	return Employment{
		ID: row.ID, Kind: row.Kind, Company: row.Company, Role: row.Role,
		Location: row.Location,
		Start:    PeriodFromColumns(row.PeriodStartYear, row.PeriodStartMonth),
		End:      PeriodFromColumns(row.PeriodEndYear, row.PeriodEndMonth),
		Current:  row.IsCurrent, Summary: row.Summary, Link: row.Link, Stack: row.Stack,
	}
}

func atomFromRow(row db.ExperienceAtom) Atom {
	return Atom{
		ID: row.ID, EmploymentID: row.EmploymentID, Claim: row.Claim, Context: row.Context,
		Metrics: row.Metrics, Skills: row.Skills,
		Provenance: Provenance(row.Provenance), SourceRef: row.SourceRef,
	}
}

func timestamptz(ts pgtype.Timestamptz) time.Time {
	if !ts.Valid {
		return time.Time{}
	}
	return ts.Time
}
