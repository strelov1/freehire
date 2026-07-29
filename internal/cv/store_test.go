package cv

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/db"
)

// fakeRepo is an in-memory owner-scoped Repository for unit-testing Store without a DB.
type fakeRepo struct {
	rows map[uuid.UUID]fakeRow
	// writes counts insertions, so a row can record the order it arrived in.
	writes int
}

type fakeRow struct {
	// seq is the insertion order. The real query breaks the "newest base CV" tie
	// with `updated_at DESC, id DESC`; random ids carry no order, so the fake keeps
	// the one thing that still models "newest" — when the row was written.
	seq        int
	userID     int64
	title      string
	templateID string
	data       []byte
	jobID      int64 // 0 = base CV (job_id NULL); >0 = tailored copy bound to a vacancy
	sessionID  string
	// report is the last autopilot run's log; undo is the document as it stood before
	// that run started. Both nil until a run happens.
	report []byte
	undo   []byte
}

func newFakeRepo() *fakeRepo { return &fakeRepo{rows: map[uuid.UUID]fakeRow{}} }

func stamp() pgtype.Timestamptz { return pgtype.Timestamptz{Valid: true} }

func (f *fakeRepo) Create(_ context.Context, userID int64, title, templateID string, data []byte) (db.CreateCVRow, error) {
	id := uuid.New()
	f.writes++
	f.rows[id] = fakeRow{seq: f.writes, userID: userID, title: title, templateID: templateID, data: data}
	return db.CreateCVRow{ID: id, Title: title, TemplateID: templateID, CreatedAt: stamp(), UpdatedAt: stamp()}, nil
}

func (f *fakeRepo) List(_ context.Context, userID int64) ([]db.ListCVsByUserRow, error) {
	var out []db.ListCVsByUserRow
	for id, r := range f.rows {
		if r.userID == userID {
			out = append(out, db.ListCVsByUserRow{ID: id, Title: r.title, TemplateID: r.templateID, CreatedAt: stamp(), UpdatedAt: stamp()})
		}
	}
	return out, nil
}

func (f *fakeRepo) Get(_ context.Context, id uuid.UUID, userID int64) (db.GetCVByIDRow, error) {
	r, ok := f.rows[id]
	if !ok || r.userID != userID {
		return db.GetCVByIDRow{}, pgx.ErrNoRows
	}
	return db.GetCVByIDRow{ID: id, Title: r.title, TemplateID: r.templateID, Data: r.data, JobID: pgtype.Int8{Int64: r.jobID, Valid: r.jobID != 0}, AgentSessionID: pgtype.Text{String: r.sessionID, Valid: r.sessionID != ""}, AutopilotReport: r.report, AutopilotRevertable: r.undo != nil, CreatedAt: stamp(), UpdatedAt: stamp()}, nil
}

func (f *fakeRepo) SetSession(_ context.Context, id uuid.UUID, userID int64, sessionID string) (int64, error) {
	r, ok := f.rows[id]
	if !ok || r.userID != userID {
		return 0, nil
	}
	r.sessionID = sessionID
	f.rows[id] = r
	return 1, nil
}

func (f *fakeRepo) SetTemplate(_ context.Context, id uuid.UUID, userID int64, templateID string) (int64, error) {
	r, ok := f.rows[id]
	if !ok || r.userID != userID {
		return 0, nil
	}
	r.templateID = templateID
	f.rows[id] = r
	return 1, nil
}

func (f *fakeRepo) ListTailored(_ context.Context, userID int64) ([]db.ListTailoredCVsByUserRow, error) {
	var out []db.ListTailoredCVsByUserRow
	for id, r := range f.rows {
		if r.userID == userID && r.jobID != 0 {
			out = append(out, db.ListTailoredCVsByUserRow{
				ID: id, Title: r.title, TemplateID: r.templateID,
				AgentSessionID: pgtype.Text{String: r.sessionID, Valid: r.sessionID != ""},
				JobSlug:        "job-slug", CreatedAt: stamp(), UpdatedAt: stamp(),
			})
		}
	}
	return out, nil
}

func (f *fakeRepo) Update(_ context.Context, id uuid.UUID, userID int64, title, templateID string, data []byte) (db.UpdateCVRow, error) {
	r, ok := f.rows[id]
	if !ok || r.userID != userID {
		return db.UpdateCVRow{}, pgx.ErrNoRows
	}
	f.rows[id] = fakeRow{seq: r.seq, userID: userID, title: title, templateID: templateID, data: data, jobID: r.jobID, sessionID: r.sessionID, report: r.report, undo: r.undo}
	return db.UpdateCVRow{ID: id, Title: title, TemplateID: templateID, CreatedAt: stamp(), UpdatedAt: stamp()}, nil
}

func (f *fakeRepo) Delete(_ context.Context, id uuid.UUID, userID int64) (int64, error) {
	if r, ok := f.rows[id]; !ok || r.userID != userID {
		return 0, nil
	}
	delete(f.rows, id)
	return 1, nil
}

func (f *fakeRepo) GetBase(_ context.Context, userID int64) (db.GetBaseCVByUserRow, error) {
	// Newest base CV = the most recently written of the user's non-tailored rows.
	var bestID uuid.UUID
	best := 0
	for id, r := range f.rows {
		if r.userID == userID && r.jobID == 0 && r.seq > best {
			best, bestID = r.seq, id
		}
	}
	if best == 0 {
		return db.GetBaseCVByUserRow{}, pgx.ErrNoRows
	}
	r := f.rows[bestID]
	return db.GetBaseCVByUserRow{ID: bestID, Title: r.title, TemplateID: r.templateID, Data: r.data, CreatedAt: stamp(), UpdatedAt: stamp()}, nil
}

func (f *fakeRepo) CreateTailored(_ context.Context, userID, jobID int64, title, templateID string, data []byte) (db.CreateTailoredCVRow, error) {
	id := uuid.New()
	f.rows[id] = fakeRow{userID: userID, title: title, templateID: templateID, data: data, jobID: jobID}
	return db.CreateTailoredCVRow{ID: id, Title: title, TemplateID: templateID, CreatedAt: stamp(), UpdatedAt: stamp()}, nil
}

func TestStoreCreateGetRoundTripSanitized(t *testing.T) {
	s := NewStore(newFakeRepo())
	ctx := context.Background()

	doc := Document{Header: Header{FullName: strings.Repeat("a", maxNameRunes+40)}}
	doc.Summary = "Systems engineer."

	meta, err := s.Create(ctx, 7, "General", "classic-ats", doc)
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	rec, err := s.Get(ctx, meta.ID, 7)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got := len([]rune(rec.Document.Header.FullName)); got > maxNameRunes {
		t.Errorf("stored document not sanitized: name %d runes", got)
	}
	if rec.Document.Summary != "Systems engineer." {
		t.Errorf("document body not round-tripped: %q", rec.Document.Summary)
	}
	if rec.Title != "General" || rec.TemplateID != "classic-ats" {
		t.Errorf("metadata not round-tripped: %+v", rec.Meta)
	}
}

func TestStoreGetForModelStripsTheContactBlock(t *testing.T) {
	s := NewStore(newFakeRepo())
	ctx := context.Background()

	doc := Document{Header: Header{
		FullName: "Ada Lovelace",
		Email:    "ada@example.com",
		Phone:    "+44 7700 900000",
		Location: "London, UK",
		Links:    []string{"https://github.com/ada"},
	}}
	doc.Summary = "Backend engineer."

	meta, err := s.Create(ctx, 7, "General", "classic-ats", doc)
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	rec, err := s.GetForModel(ctx, meta.ID, 7)
	if err != nil {
		t.Fatalf("get for model: %v", err)
	}
	if h := rec.Document.Header; h.FullName != "" || h.Email != "" || h.Phone != "" || h.Links != nil {
		t.Errorf("contact block reached a model reader: %+v", h)
	}
	// Location is not an identifier, and the agent reasons about it when the vacancy is
	// tied to a place.
	if rec.Document.Header.Location != "London, UK" {
		t.Errorf("location = %q, want it kept", rec.Document.Header.Location)
	}
	if rec.Document.Summary != "Backend engineer." {
		t.Errorf("body did not survive the redaction: %q", rec.Document.Summary)
	}

	// The redaction is on the copy handed out, never on what is stored.
	stored, err := s.Get(ctx, meta.ID, 7)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if stored.Document.Header.FullName != "Ada Lovelace" || stored.Document.Header.Email != "ada@example.com" {
		t.Errorf("stored contacts were mutated: %+v", stored.Document.Header)
	}
}

func TestStoreGetForModelForeignUserIsNotFound(t *testing.T) {
	s := NewStore(newFakeRepo())
	ctx := context.Background()

	meta, err := s.Create(ctx, 1, "Mine", "classic-ats", Document{})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := s.GetForModel(ctx, meta.ID, 2); !errors.Is(err, ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

func TestStoreGetForeignUserIsNotFound(t *testing.T) {
	s := NewStore(newFakeRepo())
	ctx := context.Background()

	meta, err := s.Create(ctx, 1, "Mine", "classic-ats", Document{})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := s.Get(ctx, meta.ID, 2); !errors.Is(err, ErrNotFound) {
		t.Errorf("foreign Get err = %v, want ErrNotFound", err)
	}
	if _, err := s.Update(ctx, meta.ID, 2, "x", "classic-ats", Document{}); !errors.Is(err, ErrNotFound) {
		t.Errorf("foreign Update err = %v, want ErrNotFound", err)
	}
	if err := s.Delete(ctx, meta.ID, 2); !errors.Is(err, ErrNotFound) {
		t.Errorf("foreign Delete err = %v, want ErrNotFound", err)
	}
}

func TestStoreDeleteThenGetIsNotFound(t *testing.T) {
	s := NewStore(newFakeRepo())
	ctx := context.Background()

	meta, _ := s.Create(ctx, 3, "Mine", "classic-ats", Document{})
	if err := s.Delete(ctx, meta.ID, 3); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := s.Get(ctx, meta.ID, 3); !errors.Is(err, ErrNotFound) {
		t.Errorf("post-delete Get err = %v, want ErrNotFound", err)
	}
}

func (f *fakeRepo) SnapshotForAutopilot(_ context.Context, id uuid.UUID, userID int64) (int64, error) {
	r, ok := f.rows[id]
	if !ok || r.userID != userID {
		return 0, nil
	}
	r.undo = r.data
	f.rows[id] = r
	return 1, nil
}

func (f *fakeRepo) SetAutopilotReport(_ context.Context, id uuid.UUID, userID int64, report []byte) (int64, error) {
	r, ok := f.rows[id]
	if !ok || r.userID != userID {
		return 0, nil
	}
	r.report = report
	f.rows[id] = r
	return 1, nil
}

func (f *fakeRepo) RevertAutopilot(_ context.Context, id uuid.UUID, userID int64) (db.RevertCVAutopilotRow, error) {
	r, ok := f.rows[id]
	// The real statement is owner-scoped AND snapshot-scoped: no snapshot matches no row.
	if !ok || r.userID != userID || r.undo == nil {
		return db.RevertCVAutopilotRow{}, pgx.ErrNoRows
	}
	r.data, r.undo, r.report = r.undo, nil, nil
	f.rows[id] = r
	return db.RevertCVAutopilotRow{ID: id, Title: r.title, TemplateID: r.templateID, CreatedAt: stamp(), UpdatedAt: stamp()}, nil
}
