package experience

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/strelov1/freehire/internal/db"
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
	ListEmployments(ctx context.Context, userID int64) ([]db.ExperienceEmployment, error)
	GetEmployment(ctx context.Context, id uuid.UUID, userID int64) (db.ExperienceEmployment, error)
	FindEmployment(ctx context.Context, userID int64, company, role string) (db.ExperienceEmployment, error)
	CreateEmployment(ctx context.Context, userID int64, e Employment) (db.ExperienceEmployment, error)
	UpdateEmployment(ctx context.Context, id uuid.UUID, userID int64, e Employment) (db.ExperienceEmployment, error)
	FillEmploymentBlanks(ctx context.Context, id uuid.UUID, userID int64, e Employment) (db.ExperienceEmployment, error)
	DeleteEmployment(ctx context.Context, id uuid.UUID, userID int64) (int64, error)

	ListAtoms(ctx context.Context, userID int64) ([]db.ExperienceAtom, error)
	GetAtom(ctx context.Context, id uuid.UUID, userID int64) (db.ExperienceAtom, error)
	InsertAtomIfNew(ctx context.Context, userID int64, a Atom, claimKey string) (db.ExperienceAtom, error)
	UpdateAtom(ctx context.Context, id uuid.UUID, userID int64, a Atom, claimKey string) (db.ExperienceAtom, error)
	DeleteAtom(ctx context.Context, id uuid.UUID, userID int64) (int64, error)
}

// Store is the bank's domain surface: it sanitizes and validates before anything is
// persisted, derives the claim key, and turns rows back into domain values.
type Store struct{ repo Repository }

// NewStore builds a Store over an owner-scoped Repository.
func NewStore(repo Repository) *Store { return &Store{repo: repo} }

// ListEmployments returns the owner's places of work, current roles first.
func (s *Store) ListEmployments(ctx context.Context, userID int64) ([]Employment, error) {
	rows, err := s.repo.ListEmployments(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("list employments: %w", err)
	}
	out := make([]Employment, 0, len(rows))
	for _, row := range rows {
		out = append(out, employmentFromRow(row))
	}
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
func (s *Store) AddAtom(ctx context.Context, userID int64, a Atom) (Atom, error) {
	a.Sanitize()
	if err := a.Validate(); err != nil {
		return Atom{}, err
	}
	row, err := s.repo.InsertAtomIfNew(ctx, userID, a, ClaimKey(a.Claim))
	if errors.Is(err, pgx.ErrNoRows) {
		return Atom{}, ErrAlreadyBanked
	}
	if err != nil {
		return Atom{}, fmt.Errorf("add atom: %w", err)
	}
	return atomFromRow(row), nil
}

// UpdateAtom replaces an owned atom's content. The claim key moves with the claim, so
// the uniqueness guarantee holds after an edit as well as after an insert.
func (s *Store) UpdateAtom(ctx context.Context, id uuid.UUID, userID int64, a Atom) (Atom, error) {
	a.Sanitize()
	if err := a.Validate(); err != nil {
		return Atom{}, err
	}
	row, err := s.repo.UpdateAtom(ctx, id, userID, a, ClaimKey(a.Claim))
	if errors.Is(err, pgx.ErrNoRows) {
		return Atom{}, ErrNotFound
	}
	if err != nil {
		return Atom{}, fmt.Errorf("update atom: %w", err)
	}
	return atomFromRow(row), nil
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

func employmentFromRow(row db.ExperienceEmployment) Employment {
	return Employment{
		ID: row.ID, Kind: row.Kind, Company: row.Company, Role: row.Role,
		Location: row.Location, Start: row.PeriodStart, End: row.PeriodEnd,
		Current: row.IsCurrent, Summary: row.Summary, Stack: row.Stack,
	}
}

func atomFromRow(row db.ExperienceAtom) Atom {
	return Atom{
		ID: row.ID, EmploymentID: row.EmploymentID, Claim: row.Claim, Context: row.Context,
		Metrics: row.Metrics, Skills: row.Skills,
		Provenance: Provenance(row.Provenance), SourceRef: row.SourceRef,
	}
}
