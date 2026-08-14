package cv

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/pgerr"
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
	// ID is a random UUID, not a counter: a CV is a résumé, and a countable id
	// would both publish how many exist and make one forgotten owner check
	// enumerable. See the opaque-cv-ids change.
	ID         uuid.UUID
	Title      string
	TemplateID string
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// Record is a CV with its full document body.
type Record struct {
	Meta
	// JobID is the vacancy a tailored CV is bound to, or 0 — for a base CV, and also for a
	// tailored copy whose vacancy was pruned (the FK nulls the link). IsTailored is what tells
	// those two apart.
	JobID int64
	// IsTailored records what the row was CREATED as. It does not follow JobID: a pruned vacancy
	// takes the link away but leaves the copy a tailored copy.
	IsTailored bool
	// AgentSessionID is the roy session bound to a tailored CV (empty when none).
	AgentSessionID string
	// TracerLinksEnabled is the candidate's consent for this CV's links to be traced.
	TracerLinksEnabled bool
	// CompanySlug is the vacancy's company, used as the tracer token's readable prefix. Empty
	// for a base CV and for a tailored copy whose vacancy was pruned.
	CompanySlug string
	Document    Document
	// AutopilotReport is the last unattended run's account of itself, one entry per
	// requirement it considered. Empty when no run has happened (or the last was reverted).
	AutopilotReport []AutopilotEntry
}

// TailoredItem is a tailored CV in the re-open list: metadata plus the vacancy (slug, title,
// company) and the bound agent session, so a row renders as a company card that links straight
// back to its tailoring workspace.
type TailoredItem struct {
	Meta
	JobSlug        string
	JobTitle       string
	JobCompany     string
	AgentSessionID string
}

// Repository persists CVs. Every read/update/delete is owner-scoped by (id, userID); a
// foreign or missing id yields pgx.ErrNoRows (Get/Update) or a zero delete count.
// Repository deliberately has NO method that writes a CV's document. internal/cvedit owns
// that, and one declared here — even unused — is an invitation to write a document with no
// revision, no policy and no evidence gate behind it.
type Repository interface {
	Create(ctx context.Context, userID int64, title, templateID string, data []byte) (db.CreateCVRow, error)
	List(ctx context.Context, userID int64) ([]db.ListCVsByUserRow, error)
	Get(ctx context.Context, id uuid.UUID, userID int64) (db.GetCVByIDRow, error)
	Delete(ctx context.Context, id uuid.UUID, userID int64) (int64, error)
	GetBase(ctx context.Context, userID int64) (db.GetBaseCVByUserRow, error)
	CreateTailored(ctx context.Context, userID, jobID int64, title, templateID string, data []byte) (db.CreateTailoredCVRow, error)
	SetSession(ctx context.Context, id uuid.UUID, userID int64, sessionID string) (int64, error)
	SetTracerLinks(ctx context.Context, id uuid.UUID, userID int64, enabled bool) (int64, error)
	ListTailored(ctx context.Context, userID int64) ([]db.ListTailoredCVsByUserRow, error)
	SetAutopilotReport(ctx context.Context, id uuid.UUID, userID int64, report []byte) (int64, error)
	GetTailoredForJob(ctx context.Context, userID, jobID int64) (db.GetTailoredCVForJobRow, error)
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
func (s *Store) Get(ctx context.Context, id uuid.UUID, userID int64) (Record, error) {
	row, err := s.repo.Get(ctx, id, userID)
	if err != nil {
		return Record{}, mapNotFound(err)
	}
	doc, err := unmarshalDocument(row.Data)
	if err != nil {
		return Record{}, err
	}
	return Record{
		Meta:               Meta{ID: row.ID, Title: row.Title, TemplateID: row.TemplateID, CreatedAt: row.CreatedAt.Time, UpdatedAt: row.UpdatedAt.Time},
		JobID:              int8Value(row.JobID),
		IsTailored:         row.IsTailored,
		AgentSessionID:     textValue(row.AgentSessionID),
		TracerLinksEnabled: row.TracerLinksEnabled,
		CompanySlug:        row.CompanySlug,
		Document:           doc,
		AutopilotReport:    decodeAutopilotReport(row.AutopilotReport),
	}, nil
}

// GetForModel returns one owned CV with the contact block stripped, for every reader
// that puts the document in front of a model.
//
// It is a second accessor rather than a flag on Get because the rule is about who is
// READING, not about how the request arrived. The redaction used to be a handler-level
// test on whether the caller held an API key; when the assistant moved in-process it
// carried no key at all, the test stopped firing, and a requirement that was still on
// the books quietly stopped being met. Naming the safe path makes it the one an agent
// surface reaches for, and leaves Get to the owner's own full-fidelity reads (the
// editor, the PDF render).
func (s *Store) GetForModel(ctx context.Context, id uuid.UUID, userID int64) (Record, error) {
	rec, err := s.Get(ctx, id, userID)
	if err != nil {
		return Record{}, err
	}
	rec.Document = rec.Document.withoutContacts()
	return rec, nil
}

// int8Value unwraps a nullable bigint to its value, mapping NULL to 0.
func int8Value(v pgtype.Int8) int64 {
	if v.Valid {
		return v.Int64
	}
	return 0
}

// textValue unwraps a nullable text column to its value, mapping NULL to "".
func textValue(v pgtype.Text) string {
	if v.Valid {
		return v.String
	}
	return ""
}

// SetSession binds (or rebinds) the agent session to an owned CV, or returns ErrNotFound.
func (s *Store) SetSession(ctx context.Context, id uuid.UUID, userID int64, sessionID string) error {
	n, err := s.repo.SetSession(ctx, id, userID, sessionID)
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// SetTracerLinks turns link tracing on or off for one CV. Owner-scoped: a foreign or missing id
// is ErrNotFound.
//
// It writes the column directly rather than going through cvedit, the only other writer of a
// stored CV. A cvedit write becomes a revision with a computed inverse, and consent to track a
// third party must not be something an undo of an unrelated edit can grant or revoke.
func (s *Store) SetTracerLinks(ctx context.Context, id uuid.UUID, userID int64, enabled bool) error {
	n, err := s.repo.SetTracerLinks(ctx, id, userID, enabled)
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// ListTailored returns the user's tailored CVs (the re-open list): metadata plus the vacancy
// slug and the bound agent session, newest edit first.
func (s *Store) ListTailored(ctx context.Context, userID int64) ([]TailoredItem, error) {
	rows, err := s.repo.ListTailored(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]TailoredItem, len(rows))
	for i, r := range rows {
		out[i] = TailoredItem{
			Meta:           Meta{ID: r.ID, Title: r.Title, TemplateID: r.TemplateID, CreatedAt: r.CreatedAt.Time, UpdatedAt: r.UpdatedAt.Time},
			JobSlug:        r.JobSlug,
			JobTitle:       r.JobTitle,
			JobCompany:     r.JobCompany,
			AgentSessionID: textValue(r.AgentSessionID),
		}
	}
	return out, nil
}

// Update sanitizes and replaces an owned CV's editable fields, or returns ErrNotFound.

// Delete removes an owned CV, or returns ErrNotFound when nothing matched.
func (s *Store) Delete(ctx context.Context, id uuid.UUID, userID int64) error {
	n, err := s.repo.Delete(ctx, id, userID)
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
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
// have none — then creates a vacancy-bound tailored copy of it. preferred is the vacancy's
// skilltag.PreferredFromText map; when non-empty the copy is surface-aligned before insert
// so the stored document never held the unaligned wording. It returns the base and tailored
// metadata plus whether THIS call is the one that just created the tailored copy (false when an
// existing one was reused instead). It returns ErrNoResume when there is nothing to seed a base
// from. The base CV is never modified: tailoring only ever writes the new tailored row.
func (s *Store) Tailor(ctx context.Context, userID, jobID int64, tailoredTitle string, seeder Seeder, preferred map[string]string) (Meta, Meta, bool, error) {
	base, ok, err := s.BaseCV(ctx, userID)
	if err != nil {
		return Meta{}, Meta{}, false, err
	}
	if !ok {
		st, hasResume, err := seeder.Structured(ctx, userID)
		if err != nil {
			return Meta{}, Meta{}, false, err
		}
		if !hasResume {
			return Meta{}, Meta{}, false, ErrNoResume
		}
		doc := Seed(st)
		meta, err := s.Create(ctx, userID, defaultBaseTitle, DefaultTemplateID, doc)
		if err != nil {
			return Meta{}, Meta{}, false, err
		}
		base = Record{Meta: meta, Document: doc}
	}
	// One tailored copy per vacancy. The workspace's address carries no CV reference, so a
	// reload re-runs this exact request; minting a second copy leaves the candidate's
	// conversation bound to the first one, which is what "my chat disappeared" looks like.
	if existing, err := s.tailoredForJob(ctx, userID, jobID); err == nil {
		return base.Meta, existing, false, nil
	} else if !errors.Is(err, ErrNotFound) {
		return Meta{}, Meta{}, false, err
	}

	doc, _ := Align(base.Document, preferred)
	tailored, err := s.CreateTailored(ctx, userID, jobID, tailoredTitle, base.TemplateID, doc)
	if err != nil {
		// A concurrent call for the same vacancy can win the insert between our SELECT above
		// and this one: cvs_user_id_job_id_tailored_uniq_idx (0091) turns the loser into a
		// unique violation instead of a duplicate row. Re-fetch and return the winner's copy.
		if pgerr.IsUniqueViolation(err) {
			existing, ferr := s.tailoredForJob(ctx, userID, jobID)
			if ferr != nil {
				return Meta{}, Meta{}, false, ferr
			}
			return base.Meta, existing, false, nil
		}
		return Meta{}, Meta{}, false, err
	}
	return base.Meta, tailored, true, nil
}

// tailoredForJob returns the user's existing tailored copy for a vacancy, or ErrNotFound.
func (s *Store) tailoredForJob(ctx context.Context, userID, jobID int64) (Meta, error) {
	row, err := s.repo.GetTailoredForJob(ctx, userID, jobID)
	if err != nil {
		return Meta{}, mapNotFound(err)
	}
	return Meta{
		ID: row.ID, Title: row.Title, TemplateID: row.TemplateID,
		CreatedAt: row.CreatedAt.Time, UpdatedAt: row.UpdatedAt.Time,
	}, nil
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

func (r queriesRepository) Get(ctx context.Context, id uuid.UUID, userID int64) (db.GetCVByIDRow, error) {
	return r.q.GetCVByID(ctx, db.GetCVByIDParams{ID: id, UserID: userID})
}

func (r queriesRepository) Delete(ctx context.Context, id uuid.UUID, userID int64) (int64, error) {
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

func (r queriesRepository) SetSession(ctx context.Context, id uuid.UUID, userID int64, sessionID string) (int64, error) {
	return r.q.SetCVSession(ctx, db.SetCVSessionParams{
		ID: id, UserID: userID,
		AgentSessionID: pgtype.Text{String: sessionID, Valid: sessionID != ""},
	})
}

func (r queriesRepository) SetTracerLinks(ctx context.Context, id uuid.UUID, userID int64, enabled bool) (int64, error) {
	return r.q.SetCVTracerLinks(ctx, db.SetCVTracerLinksParams{ID: id, UserID: userID, TracerLinksEnabled: enabled})
}

func (r queriesRepository) ListTailored(ctx context.Context, userID int64) ([]db.ListTailoredCVsByUserRow, error) {
	return r.q.ListTailoredCVsByUser(ctx, userID)
}

func (r queriesRepository) SetAutopilotReport(ctx context.Context, id uuid.UUID, userID int64, report []byte) (int64, error) {
	return r.q.SetCVAutopilotReport(ctx, db.SetCVAutopilotReportParams{ID: id, UserID: userID, AutopilotReport: report})
}

func (r queriesRepository) GetTailoredForJob(ctx context.Context, userID, jobID int64) (db.GetTailoredCVForJobRow, error) {
	return r.q.GetTailoredCVForJob(ctx, db.GetTailoredCVForJobParams{
		UserID: userID, JobID: pgtype.Int8{Int64: jobID, Valid: true},
	})
}
