package cv

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/resumeextract"
)

// ErrNotFound is returned when a CV id is missing or owned by another user. The handler
// maps it to 404 (owner isolation never leaks the existence of a foreign CV).
var ErrNotFound = errors.New("cv: not found")

// ErrNoResume is returned by Tailor when the user has neither a base CV nor an extracted
// résumé to seed one from. The handler maps it to 409 (add a résumé first).
var ErrNoResume = errors.New("cv: no résumé to seed a base CV")

// Meta is a CV without its document body — the shape the list and mutation responses use.
type Meta struct {
	ID         int64
	Title      string
	TemplateID string
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// Record is a CV with its full document body.
type Record struct {
	Meta
	// JobID is the vacancy a tailored CV is bound to, or 0 for a base CV (job_id NULL).
	JobID    int64
	Document Document
}

// Repository persists CVs. Every read/update/delete is owner-scoped by (id, userID); a
// foreign or missing id yields pgx.ErrNoRows (Get/Update) or a zero delete count.
type Repository interface {
	Create(ctx context.Context, userID int64, title, templateID string, data []byte) (db.CreateCVRow, error)
	List(ctx context.Context, userID int64) ([]db.ListCVsByUserRow, error)
	Get(ctx context.Context, id, userID int64) (db.GetCVByIDRow, error)
	Update(ctx context.Context, id, userID int64, title, templateID string, data []byte) (db.UpdateCVRow, error)
	Delete(ctx context.Context, id, userID int64) (int64, error)
	GetBase(ctx context.Context, userID int64) (db.GetBaseCVByUserRow, error)
	CreateTailored(ctx context.Context, userID, jobID int64, title, templateID string, data []byte) (db.CreateTailoredCVRow, error)
}

// Seeder provides the user's extracted résumé, used to seed a base CV when they have none.
// resume.Store already satisfies this shape (Structured), so the handler passes it directly.
type Seeder interface {
	Structured(ctx context.Context, userID int64) (resumeextract.Structured, bool, error)
}

// Store is the CV persistence service: it sanitizes and (de)serializes documents around
// the owner-scoped Repository. It holds no rendering concern.
type Store struct {
	repo Repository
}

// NewStore builds the service over an owner-scoped repository.
func NewStore(repo Repository) *Store { return &Store{repo: repo} }

// Create sanitizes and stores a new CV, returning its metadata.
func (s *Store) Create(ctx context.Context, userID int64, title, templateID string, doc Document) (Meta, error) {
	data, err := marshalSanitized(doc)
	if err != nil {
		return Meta{}, err
	}
	row, err := s.repo.Create(ctx, userID, title, templateID, data)
	if err != nil {
		return Meta{}, err
	}
	return Meta{ID: row.ID, Title: row.Title, TemplateID: row.TemplateID,
		CreatedAt: row.CreatedAt.Time, UpdatedAt: row.UpdatedAt.Time}, nil
}

// List returns the user's CVs as metadata, newest edit first.
func (s *Store) List(ctx context.Context, userID int64) ([]Meta, error) {
	rows, err := s.repo.List(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]Meta, len(rows))
	for i, r := range rows {
		out[i] = Meta{ID: r.ID, Title: r.Title, TemplateID: r.TemplateID,
			CreatedAt: r.CreatedAt.Time, UpdatedAt: r.UpdatedAt.Time}
	}
	return out, nil
}

// Get returns one owned CV with its document, or ErrNotFound.
func (s *Store) Get(ctx context.Context, id, userID int64) (Record, error) {
	row, err := s.repo.Get(ctx, id, userID)
	if err != nil {
		return Record{}, mapNotFound(err)
	}
	doc, err := unmarshalDocument(row.Data)
	if err != nil {
		return Record{}, err
	}
	return Record{
		Meta:     Meta{ID: row.ID, Title: row.Title, TemplateID: row.TemplateID, CreatedAt: row.CreatedAt.Time, UpdatedAt: row.UpdatedAt.Time},
		JobID:    int8Value(row.JobID),
		Document: doc,
	}, nil
}

// int8Value unwraps a nullable bigint to its value, mapping NULL to 0.
func int8Value(v pgtype.Int8) int64 {
	if v.Valid {
		return v.Int64
	}
	return 0
}

// Update sanitizes and replaces an owned CV's editable fields, or returns ErrNotFound.
func (s *Store) Update(ctx context.Context, id, userID int64, title, templateID string, doc Document) (Meta, error) {
	data, err := marshalSanitized(doc)
	if err != nil {
		return Meta{}, err
	}
	row, err := s.repo.Update(ctx, id, userID, title, templateID, data)
	if err != nil {
		return Meta{}, mapNotFound(err)
	}
	return Meta{ID: row.ID, Title: row.Title, TemplateID: row.TemplateID,
		CreatedAt: row.CreatedAt.Time, UpdatedAt: row.UpdatedAt.Time}, nil
}

// Delete removes an owned CV, or returns ErrNotFound when nothing matched.
func (s *Store) Delete(ctx context.Context, id, userID int64) error {
	n, err := s.repo.Delete(ctx, id, userID)
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// Patch loads an owned CV, applies one field-level edit, and persists the sanitized result.
// It returns ErrInvalidPatch (leaving the stored CV untouched) when the patch addresses a
// field or index that does not exist, or ErrNotFound for a foreign/missing id.
func (s *Store) Patch(ctx context.Context, id, userID int64, p Patch) (Meta, error) {
	rec, err := s.Get(ctx, id, userID)
	if err != nil {
		return Meta{}, err
	}
	doc, err := Apply(rec.Document, p)
	if err != nil {
		return Meta{}, err
	}
	return s.Update(ctx, id, userID, rec.Title, rec.TemplateID, doc)
}

// BaseCV returns the user's base CV (job_id IS NULL) with its document; ok is false when the
// user has only tailored CVs or none at all.
func (s *Store) BaseCV(ctx context.Context, userID int64) (Record, bool, error) {
	row, err := s.repo.GetBase(ctx, userID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Record{}, false, nil
		}
		return Record{}, false, err
	}
	doc, err := unmarshalDocument(row.Data)
	if err != nil {
		return Record{}, false, err
	}
	return Record{
		Meta:     Meta{ID: row.ID, Title: row.Title, TemplateID: row.TemplateID, CreatedAt: row.CreatedAt.Time, UpdatedAt: row.UpdatedAt.Time},
		Document: doc,
	}, true, nil
}

// CreateTailored sanitizes and stores a vacancy-bound (job_id) copy of a document.
func (s *Store) CreateTailored(ctx context.Context, userID, jobID int64, title, templateID string, doc Document) (Meta, error) {
	data, err := marshalSanitized(doc)
	if err != nil {
		return Meta{}, err
	}
	row, err := s.repo.CreateTailored(ctx, userID, jobID, title, templateID, data)
	if err != nil {
		return Meta{}, err
	}
	return Meta{ID: row.ID, Title: row.Title, TemplateID: row.TemplateID,
		CreatedAt: row.CreatedAt.Time, UpdatedAt: row.UpdatedAt.Time}, nil
}

// Tailor ensures the user has a base CV — seeding one from their extracted résumé when they
// have none — then creates a vacancy-bound tailored copy of it. It returns the base and the
// tailored metadata, or ErrNoResume when there is nothing to seed a base from. The base CV is
// never modified: tailoring only ever writes the new tailored row.
func (s *Store) Tailor(ctx context.Context, userID, jobID int64, tailoredTitle string, seeder Seeder) (Meta, Meta, error) {
	base, ok, err := s.BaseCV(ctx, userID)
	if err != nil {
		return Meta{}, Meta{}, err
	}
	if !ok {
		st, hasResume, err := seeder.Structured(ctx, userID)
		if err != nil {
			return Meta{}, Meta{}, err
		}
		if !hasResume {
			return Meta{}, Meta{}, ErrNoResume
		}
		doc := Seed(st)
		meta, err := s.Create(ctx, userID, defaultBaseTitle, DefaultTemplateID, doc)
		if err != nil {
			return Meta{}, Meta{}, err
		}
		base = Record{Meta: meta, Document: doc}
	}
	tailored, err := s.CreateTailored(ctx, userID, jobID, tailoredTitle, base.TemplateID, base.Document)
	if err != nil {
		return Meta{}, Meta{}, err
	}
	return base.Meta, tailored, nil
}

// defaultBaseTitle names a base CV seeded from the résumé (the user can rename it later).
const defaultBaseTitle = "My CV"

func marshalSanitized(doc Document) ([]byte, error) {
	doc.Sanitize()
	return json.Marshal(doc)
}

func unmarshalDocument(data []byte) (Document, error) {
	var doc Document
	if err := json.Unmarshal(data, &doc); err != nil {
		return Document{}, err
	}
	return doc, nil
}

func mapNotFound(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	return err
}

// --- db-backed repository ---

type queriesRepository struct{ q *db.Queries }

// NewQueriesRepository adapts the generated *db.Queries to the owner-scoped Repository.
func NewQueriesRepository(q *db.Queries) Repository { return queriesRepository{q: q} }

func (r queriesRepository) Create(ctx context.Context, userID int64, title, templateID string, data []byte) (db.CreateCVRow, error) {
	return r.q.CreateCV(ctx, db.CreateCVParams{UserID: userID, Title: title, TemplateID: templateID, Data: data})
}

func (r queriesRepository) List(ctx context.Context, userID int64) ([]db.ListCVsByUserRow, error) {
	return r.q.ListCVsByUser(ctx, userID)
}

func (r queriesRepository) Get(ctx context.Context, id, userID int64) (db.GetCVByIDRow, error) {
	return r.q.GetCVByID(ctx, db.GetCVByIDParams{ID: id, UserID: userID})
}

func (r queriesRepository) Update(ctx context.Context, id, userID int64, title, templateID string, data []byte) (db.UpdateCVRow, error) {
	return r.q.UpdateCV(ctx, db.UpdateCVParams{ID: id, UserID: userID, Title: title, TemplateID: templateID, Data: data})
}

func (r queriesRepository) Delete(ctx context.Context, id, userID int64) (int64, error) {
	return r.q.DeleteCV(ctx, db.DeleteCVParams{ID: id, UserID: userID})
}

func (r queriesRepository) GetBase(ctx context.Context, userID int64) (db.GetBaseCVByUserRow, error) {
	return r.q.GetBaseCVByUser(ctx, userID)
}

func (r queriesRepository) CreateTailored(ctx context.Context, userID, jobID int64, title, templateID string, data []byte) (db.CreateTailoredCVRow, error) {
	return r.q.CreateTailoredCV(ctx, db.CreateTailoredCVParams{
		UserID: userID, Title: title, TemplateID: templateID, Data: data,
		JobID: pgtype.Int8{Int64: jobID, Valid: true},
	})
}
