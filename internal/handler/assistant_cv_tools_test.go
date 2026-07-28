package handler

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/cv"
	"github.com/strelov1/freehire/internal/db"
)

// cvRepo is a one-document stand-in for cv.Repository: it serves a single owned
// CV and records what an update wrote.
type cvRepo struct {
	id      int64
	userID  int64
	jobID   int64
	data    []byte
	written []byte
}

func (r *cvRepo) Get(_ context.Context, id, userID int64) (db.GetCVByIDRow, error) {
	if id != r.id || userID != r.userID {
		return db.GetCVByIDRow{}, cv.ErrNotFound
	}
	return db.GetCVByIDRow{ID: r.id, Title: "CV", TemplateID: "classic-ats", Data: r.data, JobID: pgtype.Int8{Int64: r.jobID, Valid: r.jobID != 0}}, nil
}

func (r *cvRepo) Update(_ context.Context, id, userID int64, title, templateID string, data []byte) (db.UpdateCVRow, error) {
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
func (r *cvRepo) Delete(context.Context, int64, int64) (int64, error)        { return 0, nil }
func (r *cvRepo) GetBase(context.Context, int64) (db.GetBaseCVByUserRow, error) {
	return db.GetBaseCVByUserRow{}, cv.ErrNotFound
}
func (r *cvRepo) CreateTailored(context.Context, int64, int64, string, string, []byte) (db.CreateTailoredCVRow, error) {
	return db.CreateTailoredCVRow{}, nil
}
func (r *cvRepo) SetSession(context.Context, int64, int64, string) (int64, error)  { return 1, nil }
func (r *cvRepo) SetTemplate(context.Context, int64, int64, string) (int64, error) { return 1, nil }
func (r *cvRepo) ListTailored(context.Context, int64) ([]db.ListTailoredCVsByUserRow, error) {
	return nil, nil
}

// cvToolsAPI wires an API whose CV store serves one document owned by user 3.
func cvToolsAPI(t *testing.T, doc string) (*assistantHandlers, *cvRepo) {
	t.Helper()
	repo := &cvRepo{id: 5, userID: 3, jobID: 9, data: []byte(doc)}
	return &assistantHandlers{cv: &cvHandlers{cvStore: cv.NewStore(repo)}}, repo
}

const oneExperienceCV = `{"header":{"full_name":"Ada Lovelace","email":"ada@example.com"},` +
	`"summary":"Backend engineer","experience":[{"company":"Acme","title":"Engineer","bullets":["Shipped a thing"]}]}`

func TestCVGetToolReadsTheBoundDocument(t *testing.T) {
	a, _ := cvToolsAPI(t, oneExperienceCV)

	tool := toolByName(t, a.assistantCVTools(5, 9), "cv_get")
	out, err := tool.Run(context.Background(), 3, json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("cv_get: %v", err)
	}
	payload, _ := json.Marshal(out)
	if !strings.Contains(string(payload), "Backend engineer") {
		t.Errorf("payload = %s, want the stored document", payload)
	}
}

func TestCVEditToolAppliesAPatch(t *testing.T) {
	a, repo := cvToolsAPI(t, oneExperienceCV)

	tool := toolByName(t, a.assistantCVTools(5, 9), "cv_edit")
	_, err := tool.Run(context.Background(), 3, json.RawMessage(
		`{"patch":{"op":"set_summary","value":"Senior backend engineer"}}`))
	if err != nil {
		t.Fatalf("cv_edit: %v", err)
	}
	if !strings.Contains(string(repo.written), "Senior backend engineer") {
		t.Errorf("stored document = %s, want the patched summary", repo.written)
	}
}

func TestCVEditToolRefusesToRewriteContactDetails(t *testing.T) {
	a, _ := cvToolsAPI(t, oneExperienceCV)

	tool := toolByName(t, a.assistantCVTools(5, 9), "cv_edit")
	_, err := tool.Run(context.Background(), 3, json.RawMessage(
		`{"patch":{"op":"set_header_field","field":"email","value":"someone@else.test"}}`))
	if err == nil {
		t.Fatal("the agent must never rewrite the candidate's contact identifiers")
	}
	if !strings.Contains(err.Error(), "contact") {
		t.Errorf("error = %v, want it to say contact fields are not editable", err)
	}
}

func TestCVEditToolRejectsAMisaddressedPatch(t *testing.T) {
	a, _ := cvToolsAPI(t, oneExperienceCV)

	tool := toolByName(t, a.assistantCVTools(5, 9), "cv_edit")
	_, err := tool.Run(context.Background(), 3, json.RawMessage(
		`{"patch":{"op":"add_bullet","experience":42,"value":"Did a thing"}}`))
	if err == nil {
		t.Fatal("a patch addressing an experience entry that does not exist must fail, not silently edit the wrong one")
	}
}

func TestCVToolsAreBoundToTheSessionsCV(t *testing.T) {
	// The CV id is closed over by the session's binding, never taken as an
	// argument, so the model cannot address another CV even by guessing an id.
	a, _ := cvToolsAPI(t, oneExperienceCV)

	for _, tool := range a.assistantCVTools(5, 9) {
		props, _ := tool.Schema["properties"].(map[string]any)
		if _, ok := props["cv_id"]; ok {
			t.Errorf("tool %q takes a cv_id argument; the session binding must be the only addressing", tool.Name)
		}
	}
}

func TestCVEditToolOnAForeignCVFails(t *testing.T) {
	a, _ := cvToolsAPI(t, oneExperienceCV)

	tool := toolByName(t, a.assistantCVTools(5, 9), "cv_edit")
	// user 99 does not own CV 5: the store's owner check must refuse it.
	_, err := tool.Run(context.Background(), 99, json.RawMessage(
		`{"patch":{"op":"set_summary","value":"Hijacked"}}`))
	if err == nil {
		t.Fatal("a tool run for a different user must not reach another user's CV")
	}
}
