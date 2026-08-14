package cv

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/db"
)

// fakeRepo is an in-memory owner-scoped Repository for unit-testing Store without a DB. mu
// guards every access to rows/writes so it is also safe under the concurrent Tailor() calls
// TestStoreTailorRacesToOneTailoredCopy makes — that test exists to exercise exactly the
// race the real cvs_user_id_job_id_tailored_uniq_idx (0091) closes.
type fakeRepo struct {
	mu   sync.Mutex
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
	// tracerLinks is the candidate's consent for this CV's links to be traced.
	tracerLinks bool
	// report is the last autopilot run's log; undo is the document as it stood before
	// that run started. Both nil until a run happens.
	report []byte
	undo   []byte
}

func newFakeRepo() *fakeRepo { return &fakeRepo{rows: map[uuid.UUID]fakeRow{}} }

func stamp() pgtype.Timestamptz { return pgtype.Timestamptz{Valid: true} }

func (f *fakeRepo) Create(_ context.Context, userID int64, title, templateID string, data []byte) (db.CreateCVRow, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	id := uuid.New()
	f.writes++
	f.rows[id] = fakeRow{seq: f.writes, userID: userID, title: title, templateID: templateID, data: data}
	return db.CreateCVRow{ID: id, Title: title, TemplateID: templateID, CreatedAt: stamp(), UpdatedAt: stamp()}, nil
}

func (f *fakeRepo) List(_ context.Context, userID int64) ([]db.ListCVsByUserRow, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []db.ListCVsByUserRow
	for id, r := range f.rows {
		if r.userID == userID {
			out = append(out, db.ListCVsByUserRow{ID: id, Title: r.title, TemplateID: r.templateID, CreatedAt: stamp(), UpdatedAt: stamp()})
		}
	}
	return out, nil
}

func (f *fakeRepo) Get(_ context.Context, id uuid.UUID, userID int64) (db.GetCVByIDRow, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	r, ok := f.rows[id]
	if !ok || r.userID != userID {
		return db.GetCVByIDRow{}, pgx.ErrNoRows
	}
	return db.GetCVByIDRow{ID: id, Title: r.title, TemplateID: r.templateID, Data: r.data, JobID: pgtype.Int8{Int64: r.jobID, Valid: r.jobID != 0}, AgentSessionID: pgtype.Text{String: r.sessionID, Valid: r.sessionID != ""}, AutopilotReport: r.report, CreatedAt: stamp(), UpdatedAt: stamp()}, nil
}

func (f *fakeRepo) SetSession(_ context.Context, id uuid.UUID, userID int64, sessionID string) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	r, ok := f.rows[id]
	if !ok || r.userID != userID {
		return 0, nil
	}
	r.sessionID = sessionID
	f.rows[id] = r
	return 1, nil
}

func (f *fakeRepo) SetTracerLinks(_ context.Context, id uuid.UUID, userID int64, enabled bool) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	r, ok := f.rows[id]
	if !ok || r.userID != userID {
		return 0, nil
	}
	r.tracerLinks = enabled
	f.rows[id] = r
	return 1, nil
}

func (f *fakeRepo) ListTailored(_ context.Context, userID int64) ([]db.ListTailoredCVsByUserRow, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
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
	f.mu.Lock()
	defer f.mu.Unlock()
	r, ok := f.rows[id]
	if !ok || r.userID != userID {
		return db.UpdateCVRow{}, pgx.ErrNoRows
	}
	f.rows[id] = fakeRow{seq: r.seq, userID: userID, title: title, templateID: templateID, data: data, jobID: r.jobID, sessionID: r.sessionID, report: r.report, undo: r.undo}
	return db.UpdateCVRow{ID: id, Title: title, TemplateID: templateID, CreatedAt: stamp(), UpdatedAt: stamp()}, nil
}

func (f *fakeRepo) Delete(_ context.Context, id uuid.UUID, userID int64) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if r, ok := f.rows[id]; !ok || r.userID != userID {
		return 0, nil
	}
	delete(f.rows, id)
	return 1, nil
}

func (f *fakeRepo) GetBase(_ context.Context, userID int64) (db.GetBaseCVByUserRow, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
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

// CreateTailored mimics cvs_user_id_job_id_tailored_uniq_idx (migrations/0091): a second
// insert for a (userID, jobID) that already has a tailored row fails exactly as Postgres
// would, with a 23505 pgconn.PgError, instead of silently minting a duplicate.
func (f *fakeRepo) CreateTailored(_ context.Context, userID, jobID int64, title, templateID string, data []byte) (db.CreateTailoredCVRow, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, r := range f.rows {
		if r.userID == userID && r.jobID == jobID && jobID != 0 {
			return db.CreateTailoredCVRow{}, &pgconn.PgError{Code: "23505"}
		}
	}
	id := uuid.New()
	f.writes++
	f.rows[id] = fakeRow{seq: f.writes, userID: userID, title: title, templateID: templateID, data: data, jobID: jobID}
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

// TestRedactedHeaderKeepsOnlyLocation states the rule by the SHAPE of Header rather than by
// naming the fields to clear, so a field added to the struct is covered the moment it is
// declared.
//
// That direction is the whole point. Header reaches a model through GetForModel, and a
// redaction that lists what to remove discloses anything it has not been taught about —
// which is the reading internal/resumeextract already rejected for its own projection: "A
// blacklist — dropping the four known contact keys — would disclose that new field by
// default, which is the wrong way round."
func TestRedactedHeaderKeepsOnlyLocation(t *testing.T) {
	full := headerWithEveryFieldSet(t)

	got := Document{Header: full}.withoutContacts().Header

	v := reflect.ValueOf(got)
	for i := range v.NumField() {
		name := v.Type().Field(i).Name
		if name == "Location" {
			if got.Location != full.Location {
				t.Errorf("Location = %q, want it kept: it is not an identifier", got.Location)
			}
			continue
		}
		if !v.Field(i).IsZero() {
			t.Errorf("Header.%s survived the redaction (%v) — every field but Location must be withheld, "+
				"including one added after this test was written", name, v.Field(i).Interface())
		}
	}
}

// headerWithEveryFieldSet builds a Header with a non-zero value in EVERY field, derived from
// the struct rather than written out, so a newly declared field arrives populated and the
// redaction is actually asked about it. A field of a kind this helper cannot fill fails the
// test on purpose: a new kind in the contact block is a decision, not a default.
func headerWithEveryFieldSet(t *testing.T) Header {
	t.Helper()
	var h Header
	v := reflect.ValueOf(&h).Elem()
	for i := range v.NumField() {
		f, name := v.Field(i), v.Type().Field(i).Name
		switch f.Kind() {
		case reflect.String:
			f.SetString("set-" + name)
		case reflect.Slice:
			f.Set(reflect.MakeSlice(f.Type(), 1, 1))
			if e := f.Index(0); e.Kind() == reflect.String {
				e.SetString("set-" + name)
			}
		default:
			t.Fatalf("Header.%s is a %s, which this test cannot populate — teach it, and decide "+
				"whether the field is an identifier while you are here", name, f.Kind())
		}
	}
	return h
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

func (f *fakeRepo) SetAutopilotReport(_ context.Context, id uuid.UUID, userID int64, report []byte) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	r, ok := f.rows[id]
	if !ok || r.userID != userID {
		return 0, nil
	}
	r.report = report
	f.rows[id] = r
	return 1, nil
}

// MergeAutopilotEntry does the read, the match-or-append and the write inside ONE critical
// section, the same way the real MergeCVAutopilotEntry query does it inside one statement —
// unlike SetAutopilotReport above, which is safe to call as a separate step only because
// Store.MergeAutopilotEntry no longer does that; see its own comment.
func (f *fakeRepo) MergeAutopilotEntry(_ context.Context, id uuid.UUID, userID int64, entry []byte) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	r, ok := f.rows[id]
	if !ok || r.userID != userID {
		return 0, nil
	}
	var incoming AutopilotEntry
	if err := json.Unmarshal(entry, &incoming); err != nil {
		return 0, err
	}
	var entries []AutopilotEntry
	if len(r.report) > 0 {
		if err := json.Unmarshal(r.report, &entries); err != nil {
			return 0, err
		}
	}
	target := strings.ToLower(strings.TrimSpace(incoming.Requirement))
	replaced := false
	for i, e := range entries {
		if strings.ToLower(strings.TrimSpace(e.Requirement)) == target {
			entries[i] = incoming
			replaced = true
			break
		}
	}
	if !replaced {
		entries = append(entries, incoming)
	}
	blob, err := json.Marshal(entries)
	if err != nil {
		return 0, err
	}
	r.report = blob
	f.rows[id] = r
	return 1, nil
}

func (f *fakeRepo) GetTailoredForJob(_ context.Context, userID, jobID int64) (db.GetTailoredCVForJobRow, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	// Newest by insertion order, mirroring the query's `updated_at DESC, id DESC`.
	var bestID uuid.UUID
	var best *fakeRow
	for id, r := range f.rows {
		row := r
		if row.userID != userID || row.jobID != jobID {
			continue
		}
		if best == nil || row.seq > best.seq {
			best, bestID = &row, id
		}
	}
	if best == nil {
		return db.GetTailoredCVForJobRow{}, pgx.ErrNoRows
	}
	return db.GetTailoredCVForJobRow{
		ID: bestID, Title: best.title, TemplateID: best.templateID, Data: best.data,
		AgentSessionID: pgtype.Text{String: best.sessionID, Valid: best.sessionID != ""},
		CreatedAt:      stamp(), UpdatedAt: stamp(),
	}, nil
}
