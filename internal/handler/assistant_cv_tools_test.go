package handler

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/cv"
	"github.com/strelov1/freehire/internal/db"
)

// cvRepo is a one-document stand-in for cv.Repository: it serves a single owned
// CV and records what an update wrote.
type cvRepo struct {
	id      uuid.UUID
	userID  int64
	jobID   int64
	data    []byte
	written []byte
}

func (r *cvRepo) Get(_ context.Context, id uuid.UUID, userID int64) (db.GetCVByIDRow, error) {
	if id != r.id || userID != r.userID {
		return db.GetCVByIDRow{}, cv.ErrNotFound
	}
	return db.GetCVByIDRow{ID: r.id, Title: "CV", TemplateID: "classic-ats", Data: r.data, JobID: pgtype.Int8{Int64: r.jobID, Valid: r.jobID != 0}}, nil
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
func (r *cvRepo) CreateTailored(context.Context, int64, int64, string, string, []byte) (db.CreateTailoredCVRow, error) {
	return db.CreateTailoredCVRow{}, nil
}
func (r *cvRepo) SetSession(context.Context, uuid.UUID, int64, string) (int64, error)  { return 1, nil }
func (r *cvRepo) SetTemplate(context.Context, uuid.UUID, int64, string) (int64, error) { return 1, nil }
func (r *cvRepo) ListTailored(context.Context, int64) ([]db.ListTailoredCVsByUserRow, error) {
	return nil, nil
}

// testCVID is the CV every case in this file addresses. Fixed so a failure names a
// stable value rather than a fresh random one.
var testCVID = uuid.MustParse("55555555-5555-4555-8555-555555555555")

// cvToolsAPI wires an API whose CV store serves one document owned by user 3.
func cvToolsAPI(t *testing.T, doc string) (*assistantHandlers, *cvRepo) {
	t.Helper()
	repo := &cvRepo{id: testCVID, userID: 3, jobID: 9, data: []byte(doc)}
	return &assistantHandlers{cv: &cvHandlers{cvStore: cv.NewStore(repo)}}, repo
}

const oneExperienceCV = `{"header":{"full_name":"Ada Lovelace","email":"ada@example.com"},` +
	`"summary":"Backend engineer","experience":[{"company":"Acme","title":"Engineer","bullets":["Shipped a thing"]}]}`

func TestCVGetToolReadsTheBoundDocument(t *testing.T) {
	a, _ := cvToolsAPI(t, oneExperienceCV)

	tool := toolByName(t, a.assistantCVTools(testCVID, 9), "cv_get")
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

	tool := toolByName(t, a.assistantCVTools(testCVID, 9), "cv_edit")
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

	tool := toolByName(t, a.assistantCVTools(testCVID, 9), "cv_edit")
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

	tool := toolByName(t, a.assistantCVTools(testCVID, 9), "cv_edit")
	_, err := tool.Run(context.Background(), 3, json.RawMessage(
		`{"patch":{"op":"add_bullet","experience":42,"value":"Did a thing"}}`))
	if err == nil {
		t.Fatal("a patch addressing an experience entry that does not exist must fail, not silently edit the wrong one")
	}
}

// cvEditPatchSchema returns the JSON Schema cv_edit advertises for its patch argument.
func cvEditPatchSchema(t *testing.T) map[string]any {
	t.Helper()
	a, _ := cvToolsAPI(t, oneExperienceCV)
	tool := toolByName(t, a.assistantCVTools(testCVID, 9), "cv_edit")
	props, _ := tool.Schema["properties"].(map[string]any)
	patch, _ := props["patch"].(map[string]any)
	fields, _ := patch["properties"].(map[string]any)
	if len(fields) == 0 {
		t.Fatal("cv_edit advertises the patch as a bare object; the model has to guess its fields")
	}
	return fields
}

func TestCVEditToolSchemaTypesEveryPatchField(t *testing.T) {
	// A patch advertised as a shapeless object made the model fill it in by analogy:
	// it sent replace_bullet with the new TEXT in `bullet` — the index field — and no
	// `value` at all. Typing each field is what stops that before the call is spent.
	fields := cvEditPatchSchema(t)

	for field, want := range map[string]string{
		"op":         "string",
		"experience": "integer",
		"bullet":     "integer",
		"field":      "string",
		"value":      "string",
		"group":      "string",
		"order":      "array",
		"items":      "array",
		"stack":      "array",
	} {
		spec, ok := fields[field].(map[string]any)
		if !ok {
			t.Errorf("patch schema does not declare %q", field)
			continue
		}
		if spec["type"] != want {
			t.Errorf("patch field %q is typed %v, want %s", field, spec["type"], want)
		}
	}
}

func TestCVEditToolSchemaOffersEveryPatchOp(t *testing.T) {
	// The prose list drifted once already: it named seven ops while Apply accepted
	// eight, so set_stack was unreachable. The enum comes from the cv package so the
	// two cannot diverge again.
	fields := cvEditPatchSchema(t)

	op, _ := fields["op"].(map[string]any)
	enum, _ := op["enum"].([]string)
	if len(enum) != len(cv.PatchOps) {
		t.Fatalf("op enum = %v, want every op in cv.PatchOps (%v)", enum, cv.PatchOps)
	}
	for i, want := range cv.PatchOps {
		if enum[i] != want {
			t.Errorf("op enum[%d] = %q, want %q", i, enum[i], want)
		}
	}
}

func TestCVEditToolSchemaOffersOnlyTheEditableHeaderField(t *testing.T) {
	// Contact identifiers are refused at runtime, so advertising them only buys a
	// wasted turn: location is the one header field a tailoring session can write.
	fields := cvEditPatchSchema(t)

	field, _ := fields["field"].(map[string]any)
	enum, _ := field["enum"].([]string)
	if len(enum) != 1 || enum[0] != "location" {
		t.Errorf("field enum = %v, want only location", enum)
	}
}

func TestCVToolsAreBoundToTheSessionsCV(t *testing.T) {
	// The CV id is closed over by the session's binding, never taken as an
	// argument, so the model cannot address another CV even by guessing an id.
	a, _ := cvToolsAPI(t, oneExperienceCV)

	for _, tool := range a.assistantCVTools(testCVID, 9) {
		props, _ := tool.Schema["properties"].(map[string]any)
		if _, ok := props["cv_id"]; ok {
			t.Errorf("tool %q takes a cv_id argument; the session binding must be the only addressing", tool.Name)
		}
	}
}

func TestCVEditToolOnAForeignCVFails(t *testing.T) {
	a, _ := cvToolsAPI(t, oneExperienceCV)

	tool := toolByName(t, a.assistantCVTools(testCVID, 9), "cv_edit")
	// user 99 does not own CV 5: the store's owner check must refuse it.
	_, err := tool.Run(context.Background(), 99, json.RawMessage(
		`{"patch":{"op":"set_summary","value":"Hijacked"}}`))
	if err == nil {
		t.Fatal("a tool run for a different user must not reach another user's CV")
	}
}
