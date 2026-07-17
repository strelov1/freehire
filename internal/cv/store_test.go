package cv

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/db"
)

// fakeRepo is an in-memory owner-scoped Repository for unit-testing Store without a DB.
type fakeRepo struct {
	rows map[int64]fakeRow
	next int64
}

type fakeRow struct {
	userID     int64
	title      string
	templateID string
	data       []byte
	jobID      int64 // 0 = base CV (job_id NULL); >0 = tailored copy bound to a vacancy
}

func newFakeRepo() *fakeRepo { return &fakeRepo{rows: map[int64]fakeRow{}, next: 1} }

func stamp() pgtype.Timestamptz { return pgtype.Timestamptz{Valid: true} }

func (f *fakeRepo) Create(_ context.Context, userID int64, title, templateID string, data []byte) (db.CreateCVRow, error) {
	id := f.next
	f.next++
	f.rows[id] = fakeRow{userID: userID, title: title, templateID: templateID, data: data}
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

func (f *fakeRepo) Get(_ context.Context, id, userID int64) (db.GetCVByIDRow, error) {
	r, ok := f.rows[id]
	if !ok || r.userID != userID {
		return db.GetCVByIDRow{}, pgx.ErrNoRows
	}
	return db.GetCVByIDRow{ID: id, Title: r.title, TemplateID: r.templateID, Data: r.data, JobID: pgtype.Int8{Int64: r.jobID, Valid: r.jobID != 0}, CreatedAt: stamp(), UpdatedAt: stamp()}, nil
}

func (f *fakeRepo) Update(_ context.Context, id, userID int64, title, templateID string, data []byte) (db.UpdateCVRow, error) {
	r, ok := f.rows[id]
	if !ok || r.userID != userID {
		return db.UpdateCVRow{}, pgx.ErrNoRows
	}
	f.rows[id] = fakeRow{userID: userID, title: title, templateID: templateID, data: data, jobID: r.jobID}
	return db.UpdateCVRow{ID: id, Title: title, TemplateID: templateID, CreatedAt: stamp(), UpdatedAt: stamp()}, nil
}

func (f *fakeRepo) Delete(_ context.Context, id, userID int64) (int64, error) {
	if r, ok := f.rows[id]; !ok || r.userID != userID {
		return 0, nil
	}
	delete(f.rows, id)
	return 1, nil
}

func (f *fakeRepo) GetBase(_ context.Context, userID int64) (db.GetBaseCVByUserRow, error) {
	// Newest base CV = the highest id among the user's non-tailored rows (mirrors the
	// query's updated_at DESC, id DESC tiebreak).
	var bestID int64
	for id, r := range f.rows {
		if r.userID == userID && r.jobID == 0 && id > bestID {
			bestID = id
		}
	}
	if bestID == 0 {
		return db.GetBaseCVByUserRow{}, pgx.ErrNoRows
	}
	r := f.rows[bestID]
	return db.GetBaseCVByUserRow{ID: bestID, Title: r.title, TemplateID: r.templateID, Data: r.data, CreatedAt: stamp(), UpdatedAt: stamp()}, nil
}

func (f *fakeRepo) CreateTailored(_ context.Context, userID, jobID int64, title, templateID string, data []byte) (db.CreateTailoredCVRow, error) {
	id := f.next
	f.next++
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
