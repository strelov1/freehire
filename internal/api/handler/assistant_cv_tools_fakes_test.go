package handler

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/candidate/cv"
	"github.com/strelov1/freehire/internal/candidate/cvedit"
	"github.com/strelov1/freehire/internal/platform/db"
)

// cvRepo is a one-document stand-in for cv.Repository: it serves a single owned
// CV and records what an update wrote.
type cvRepo struct {
	id      uuid.UUID
	userID  int64
	jobID   int64
	data    []byte
	written []byte
	// report is what an autopilot run leaves behind: its account of each requirement.
	report []byte
	// autopilotErr, when set, is what SetAutopilotReport returns instead of succeeding —
	// simulating a report write that fails after the document edit has already landed.
	autopilotErr error
}

var testJobID = int64(9)

func (r *cvRepo) Get(_ context.Context, id uuid.UUID, userID int64) (db.GetCVByIDRow, error) {
	if id != r.id || userID != r.userID {
		return db.GetCVByIDRow{}, cv.ErrNotFound
	}
	return db.GetCVByIDRow{ID: r.id, Title: "CV", TemplateID: "classic-ats", Data: r.data,
		JobID:           pgtype.Int8{Int64: r.jobID, Valid: r.jobID != 0},
		AutopilotReport: r.report}, nil
}

func (r *cvRepo) Update(_ context.Context, id uuid.UUID, userID int64, title, templateID string, data []byte) (db.UpdateCVRow, error) {
	if id != r.id || userID != r.userID {
		return db.UpdateCVRow{}, cv.ErrNotFound
	}
	r.written = data
	r.data = data
	return db.UpdateCVRow{ID: id, Title: title, TemplateID: templateID}, nil
}

func (r *cvRepo) Create(context.Context, int64, string, string, []byte) (db.CreateCVRow, error) {
	return db.CreateCVRow{}, nil
}
func (r *cvRepo) List(context.Context, int64) ([]db.ListCVsByUserRow, error) { return nil, nil }
func (r *cvRepo) Delete(context.Context, uuid.UUID, int64) (int64, error)    { return 0, nil }
func (r *cvRepo) GetBase(context.Context, int64) (db.GetBaseCVByUserRow, error) {
	return db.GetBaseCVByUserRow{}, cv.ErrNotFound
}
func (r *cvRepo) ListBases(context.Context, int64) ([]db.ListBaseCVsByUserRow, error) {
	return nil, nil
}
func (r *cvRepo) Duplicate(context.Context, uuid.UUID, int64, string) (db.DuplicateBaseCVRow, error) {
	return db.DuplicateBaseCVRow{}, nil
}
func (r *cvRepo) CreateTailored(context.Context, int64, int64, uuid.UUID, string, string, []byte) (db.CreateTailoredCVRow, error) {
	return db.CreateTailoredCVRow{}, nil
}
func (r *cvRepo) SetSession(context.Context, uuid.UUID, int64, string) (int64, error)  { return 1, nil }
func (r *cvRepo) SetTemplate(context.Context, uuid.UUID, int64, string) (int64, error) { return 1, nil }
func (r *cvRepo) SetTracerLinks(context.Context, uuid.UUID, int64, bool) (int64, error) {
	return 1, nil
}
func (r *cvRepo) ListTailored(context.Context, int64) ([]db.ListTailoredCVsByUserRow, error) {
	return nil, nil
}

func (r *cvRepo) GetTailoredForJob(_ context.Context, userID, jobID int64) (db.GetTailoredCVForJobRow, error) {
	if userID != r.userID || jobID != r.jobID {
		return db.GetTailoredCVForJobRow{}, pgx.ErrNoRows
	}
	return db.GetTailoredCVForJobRow{ID: r.id, Title: "CV", TemplateID: "classic-ats", Data: r.data}, nil
}

func (r *cvRepo) SetAutopilotReport(_ context.Context, id uuid.UUID, userID int64, report []byte) (int64, error) {
	if r.autopilotErr != nil {
		return 0, r.autopilotErr
	}
	if id != r.id || userID != r.userID {
		return 0, nil
	}
	r.report = report
	return 1, nil
}

func (r *cvRepo) MergeAutopilotEntry(_ context.Context, id uuid.UUID, userID int64, entry []byte) (int64, error) {
	if r.autopilotErr != nil {
		return 0, r.autopilotErr
	}
	if id != r.id || userID != r.userID {
		return 0, nil
	}
	var incoming cv.AutopilotEntry
	if err := json.Unmarshal(entry, &incoming); err != nil {
		return 0, err
	}
	var entries []cv.AutopilotEntry
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
	return 1, nil
}

// GetAppearanceDefaults/UpsertAppearanceDefaults are unused by anything in this file — no
// case here saves or reads appearance defaults — so GetAppearanceDefaults always reports
// "nothing saved" and Upsert is never expected to be called.
func (r *cvRepo) GetAppearanceDefaults(context.Context, int64) (db.CvAppearanceDefault, error) {
	return db.CvAppearanceDefault{}, pgx.ErrNoRows
}

func (r *cvRepo) UpsertAppearanceDefaults(_ context.Context, userID int64, templateID string, style, margins []byte) (db.CvAppearanceDefault, error) {
	return db.CvAppearanceDefault{UserID: userID, TemplateID: templateID, Style: style, Margins: margins}, nil
}

// testCVID is the CV every case in this file addresses. Fixed so a failure names a
// stable value rather than a fresh random one.
var testCVID = uuid.MustParse("55555555-5555-4555-8555-555555555555")

// cvToolsAPI wires an API whose CV store serves one document owned by user 3.
func cvToolsAPI(t *testing.T, doc string) (*assistantHandlers, *cvRepo) {
	t.Helper()
	// An EMPTY bank, not a nil gate. Production always constructs the editor with one
	// (Register passes bankGate{bank}), so a nil gate is a configuration that does not ship
	// — a fixture using it asserts behaviour nobody can reach. An empty bank is the honest
	// stand-in: the gate is present and refuses every citation, which is what a user with
	// nothing banked actually experiences.
	repo := &cvRepo{id: testCVID, userID: 3, jobID: 9, data: []byte(doc)}
	bank := newStubBank()
	editor := cvedit.NewEditor(&memRevisions{cv: repo}, bankGate{bank: bank})
	return &assistantHandlers{experience: bank, cv: &cvHandlers{cvStore: cv.NewStore(repo), editor: editor}}, repo
}

// cvToolsAPIWithBank is cvToolsAPI for the cases that exercise the evidence gate. The
// editor is CONSTRUCTED with the gate, the way the production assembly builds it — the
// gate is not something a caller can attach to an editor after the fact.
func cvToolsAPIWithBank(t *testing.T, doc string, bank experienceBankTools) (*assistantHandlers, *cvRepo) {
	t.Helper()
	repo := &cvRepo{id: testCVID, userID: 3, jobID: 9, data: []byte(doc)}
	editor := cvedit.NewEditor(&memRevisions{cv: repo}, bankGate{bank: bank})
	return &assistantHandlers{
		experience: bank,
		cv:         &cvHandlers{cvStore: cv.NewStore(repo), editor: editor},
	}, repo
}

// memRevisions is an in-memory cvedit.Repository over the same cvRepo the CV store uses, so
// a tool test exercises the real editor — policy, evidence gate, apply, coalescing — without
// a database. Writes land in cvRepo.written, which is what the cases assert on.
type memRevisions struct {
	cv        *cvRepo
	revisions []cvedit.Revision
}

func (m *memRevisions) Edit(ctx context.Context, _ uuid.UUID, _ int64, fn func(context.Context, cvedit.Tx) error) error {
	return fn(ctx, m)
}

func (m *memRevisions) List(_ context.Context, _ uuid.UUID, _ int64, limit int32) ([]cvedit.Revision, error) {
	out := make([]cvedit.Revision, 0, len(m.revisions))
	for i := len(m.revisions) - 1; i >= 0 && len(out) < int(limit); i-- {
		out = append(out, m.revisions[i])
	}
	return out, nil
}

func (m *memRevisions) Get(_ context.Context, id uuid.UUID, _ int64) (cvedit.Revision, error) {
	for _, r := range m.revisions {
		if r.ID == id {
			return r, nil
		}
	}
	return cvedit.Revision{}, cvedit.ErrNothingToUndo
}

func (m *memRevisions) State(context.Context) (cvedit.State, time.Time, error) {
	var doc cv.Document
	blob := m.cv.written
	if blob == nil {
		blob = m.cv.data
	}
	if err := json.Unmarshal(blob, &doc); err != nil {
		return cvedit.State{}, time.Time{}, err
	}
	return cvedit.State{Title: "CV", TemplateID: "classic-ats", Document: doc}, time.Time{}, nil
}

func (m *memRevisions) MarkHandEdited(context.Context) error { return nil }

func (m *memRevisions) Save(_ context.Context, s cvedit.State) (cv.Meta, error) {
	blob, err := json.Marshal(s.Document)
	if err != nil {
		return cv.Meta{}, err
	}
	m.cv.written = blob
	return cv.Meta{ID: m.cv.id, Title: s.Title, TemplateID: s.TemplateID}, nil
}

func (m *memRevisions) Newest(context.Context) (cvedit.Revision, bool, error) {
	if len(m.revisions) == 0 {
		return cvedit.Revision{}, false, nil
	}
	return m.revisions[len(m.revisions)-1], true, nil
}

func (m *memRevisions) Revision(_ context.Context, id uuid.UUID) (cvedit.Revision, bool, error) {
	for _, r := range m.revisions {
		if r.ID == id {
			return r, true, nil
		}
	}
	return cvedit.Revision{}, false, nil
}

func (m *memRevisions) Insert(_ context.Context, rev cvedit.Revision) (cvedit.Revision, error) {
	rev.ID = uuid.New()
	m.revisions = append(m.revisions, rev)
	return rev, nil
}

func (m *memRevisions) Amend(_ context.Context, id uuid.UUID, ops []cvedit.Op, title, note string) (cvedit.Revision, error) {
	for i, r := range m.revisions {
		if r.ID == id {
			m.revisions[i].Ops = ops
			m.revisions[i].Title = title
			m.revisions[i].Note = note
			return m.revisions[i], nil
		}
	}
	return cvedit.Revision{}, cvedit.ErrNothingToUndo
}

func (m *memRevisions) MarkReverted(_ context.Context, id uuid.UUID) (bool, error) {
	for i, r := range m.revisions {
		if r.ID == id && r.RevertedAt == nil {
			now := time.Now()
			m.revisions[i].RevertedAt = &now
			return true, nil
		}
	}
	return false, nil
}

func (m *memRevisions) InBatch(_ context.Context, batchID uuid.UUID) ([]cvedit.Revision, error) {
	var out []cvedit.Revision
	for i := len(m.revisions) - 1; i >= 0; i-- {
		if m.revisions[i].BatchID == batchID && m.revisions[i].RevertedAt == nil {
			out = append(out, m.revisions[i])
		}
	}
	return out, nil
}

func (m *memRevisions) Feed(_ context.Context, limit int32) ([]cvedit.Revision, error) {
	out := make([]cvedit.Revision, 0, len(m.revisions))
	for i := len(m.revisions) - 1; i >= 0 && len(out) < int(limit); i-- {
		out = append(out, m.revisions[i])
	}
	return out, nil
}

func (m *memRevisions) Trim(context.Context, int32) error { return nil }
