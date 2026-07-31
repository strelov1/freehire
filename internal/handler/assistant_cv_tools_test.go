package handler

import (
	"context"
	"encoding/json"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/cv"
	"github.com/strelov1/freehire/internal/cvedit"
	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/experience"
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
}

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
func (r *cvRepo) CreateTailored(context.Context, int64, int64, string, string, []byte) (db.CreateTailoredCVRow, error) {
	return db.CreateTailoredCVRow{}, nil
}
func (r *cvRepo) SetSession(context.Context, uuid.UUID, int64, string) (int64, error)  { return 1, nil }
func (r *cvRepo) SetTemplate(context.Context, uuid.UUID, int64, string) (int64, error) { return 1, nil }
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
	if id != r.id || userID != r.userID {
		return 0, nil
	}
	r.report = report
	return 1, nil
}

// testCVID is the CV every case in this file addresses. Fixed so a failure names a
// stable value rather than a fresh random one.
var testCVID = uuid.MustParse("55555555-5555-4555-8555-555555555555")

// cvToolsAPI wires an API whose CV store serves one document owned by user 3.
func cvToolsAPI(t *testing.T, doc string) (*assistantHandlers, *cvRepo) {
	t.Helper()
	repo := &cvRepo{id: testCVID, userID: 3, jobID: 9, data: []byte(doc)}
	editor := cvedit.NewEditor(&memRevisions{cv: repo}, nil)
	return &assistantHandlers{cv: &cvHandlers{cvStore: cv.NewStore(repo), editor: editor}}, repo
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

func (m *memRevisions) Amend(_ context.Context, id uuid.UUID, ops []cvedit.Op, title string) (cvedit.Revision, error) {
	for i, r := range m.revisions {
		if r.ID == id {
			m.revisions[i].Ops = ops
			m.revisions[i].Title = title
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

func (m *memRevisions) Trim(context.Context, int32) error { return nil }

const oneExperienceCV = `{"header":{"full_name":"Ada Lovelace","email":"ada@example.com"},` +
	`"summary":"Backend engineer","experience":[{"company":"Acme","title":"Engineer","bullets":["Shipped a thing"]}]}`

func TestCVGetToolReadsTheBoundDocument(t *testing.T) {
	a, _ := cvToolsAPI(t, oneExperienceCV)

	tool := toolByName(t, a.assistantCVTools(testCVID, 9, uuid.New()), "cv_get")
	out, err := tool.Run(context.Background(), 3, json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("cv_get: %v", err)
	}
	payload, _ := json.Marshal(out)
	if !strings.Contains(string(payload), "Backend engineer") {
		t.Errorf("payload = %s, want the stored document", payload)
	}
}

// contactfulCV carries every identifier the agent must never receive, so a leak on any
// one of the four fails the case below.
const contactfulCV = `{"header":{"full_name":"Ada Lovelace","email":"ada@example.com",` +
	`"phone":"+44 7700 900000","links":["https://github.com/ada"]},` +
	`"summary":"Backend engineer","experience":[{"company":"Acme","title":"Engineer","bullets":["Shipped a thing"]}]}`

// The tailoring agent runs a model over the CV, and the model reads attacker-controlled
// text (job descriptions, browsed pages) that can talk it into writing what it holds
// somewhere it does not belong. The contact block must therefore never enter its context.
//
// This has to hold for the in-process tool, which carries no API key and issues no HTTP
// request: the guard cannot be a test on how the caller arrived.
func TestCVGetToolOmitsTheContactBlock(t *testing.T) {
	a, _ := cvToolsAPI(t, contactfulCV)

	tool := toolByName(t, a.assistantCVTools(testCVID, 9, uuid.New()), "cv_get")
	out, err := tool.Run(context.Background(), 3, json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("cv_get: %v", err)
	}
	raw, _ := json.Marshal(out)
	payload := string(raw)

	for _, identifier := range []string{"Ada Lovelace", "ada@example.com", "+44 7700 900000", "github.com/ada"} {
		if strings.Contains(payload, identifier) {
			t.Errorf("cv_get returned %q; the agent's model must never see it\npayload = %s", identifier, payload)
		}
	}
	if !strings.Contains(payload, "Backend engineer") {
		t.Errorf("payload = %s, want the body to survive the redaction", payload)
	}
}

func TestCVEditToolAppliesAPatch(t *testing.T) {
	a, repo := cvToolsAPI(t, oneExperienceCV)

	tool := toolByName(t, a.assistantCVTools(testCVID, 9, uuid.New()), "cv_edit")
	_, err := tool.Run(context.Background(), 3, json.RawMessage(
		`{"ops":[{"kind":"set","path":"summary","value":"Senior backend engineer"}]}`))
	if err != nil {
		t.Fatalf("cv_edit: %v", err)
	}
	if !strings.Contains(string(repo.written), "Senior backend engineer") {
		t.Errorf("stored document = %s, want the patched summary", repo.written)
	}
}

func TestCVEditToolRefusesToRewriteContactDetails(t *testing.T) {
	a, _ := cvToolsAPI(t, oneExperienceCV)

	tool := toolByName(t, a.assistantCVTools(testCVID, 9, uuid.New()), "cv_edit")
	_, err := tool.Run(context.Background(), 3, json.RawMessage(
		`{"ops":[{"kind":"set","path":"header.email","value":"someone@else.test"}]}`))
	if err == nil {
		t.Fatal("the agent must never rewrite the candidate's contact identifiers")
	}
	// The refusal names what the agent MAY edit: for a model the message is its only route
	// to correcting itself inside the turn.
	if !strings.Contains(err.Error(), "experience") {
		t.Errorf("error = %v, want it to name what the agent may edit instead", err)
	}
}

func TestCVEditToolRejectsAMisaddressedPatch(t *testing.T) {
	a, _ := cvToolsAPI(t, oneExperienceCV)

	tool := toolByName(t, a.assistantCVTools(testCVID, 9, uuid.New()), "cv_edit")
	_, err := tool.Run(context.Background(), 3, json.RawMessage(
		`{"ops":[{"kind":"insert","path":"experience[42].bullets[0]","value":"Did a thing"}]}`))
	if err == nil {
		t.Fatal("an edit addressing an experience entry that does not exist must fail, not silently edit the wrong one")
	}
}

// cvEditPatchSchema returns the JSON Schema cv_edit advertises for its patch argument.
// cvEditOpSchema returns the JSON Schema cv_edit advertises for ONE operation in its batch.
func cvEditOpSchema(t *testing.T) map[string]any {
	t.Helper()
	a, _ := cvToolsAPI(t, oneExperienceCV)
	tool := toolByName(t, a.assistantCVTools(testCVID, 9, uuid.New()), "cv_edit")
	props, _ := tool.Schema["properties"].(map[string]any)
	ops, _ := props["ops"].(map[string]any)
	item, _ := ops["items"].(map[string]any)
	fields, _ := item["properties"].(map[string]any)
	if len(fields) == 0 {
		t.Fatal("cv_edit advertises its operations as bare objects; the model has to guess their fields")
	}
	return fields
}

func TestCVEditToolSchemaTypesEveryOperationField(t *testing.T) {
	fields := cvEditOpSchema(t)

	// The model fills a shape it cannot see by analogy, and gets it wrong: an earlier
	// version advertised the edit as a bare object and received a bullet's TEXT where an
	// index belonged. A rejected call still costs a whole turn.
	for _, want := range []string{"kind", "path", "value", "to", "evidence_id"} {
		if _, ok := fields[want]; !ok {
			t.Errorf("cv_edit's operation schema omits %q", want)
		}
	}
}

func TestCVEditToolSchemaOffersEveryOperationKind(t *testing.T) {
	fields := cvEditOpSchema(t)
	kind, _ := fields["kind"].(map[string]any)
	got, _ := kind["enum"].([]string)

	// The enum comes from the cvedit package rather than being restated here, so the two
	// cannot drift — an operation the model cannot see is one it cannot use.
	if len(got) != len(cvedit.OpKinds) {
		t.Fatalf("kind enum = %v, want %v", got, cvedit.OpKinds)
	}
	sort.Strings(got)
	want := append([]string{}, cvedit.OpKinds...)
	sort.Strings(want)
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("kind enum = %v, want %v", got, want)
		}
	}
}

// The addressable shapes are generated from the document's own structure, so a field added
// to the CV becomes addressable without anyone editing the tool. The schema has to carry
// them: a path the model cannot see is a part of the CV it cannot reach.
func TestCVEditToolSchemaNamesTheAddressableShapes(t *testing.T) {
	fields := cvEditOpSchema(t)
	path, _ := fields["path"].(map[string]any)
	description, _ := path["description"].(string)

	for _, want := range []string{
		"experience[i].bullets[j]",
		"skills[i].items[j]",
		"education[i].institution", // unreachable under the old named vocabulary
		"certifications[i].issuer", //
		"style.font_size",          //
	} {
		if !strings.Contains(description, want) {
			t.Errorf("the path description does not offer %q", want)
		}
	}
}

func TestCVEditWithoutAnyEvidenceIdNamesWhereItGoes(t *testing.T) {
	a, _ := cvToolsAPI(t, oneExperienceCV)
	bank := newStubBank()
	a.experience = bank
	a.cv.editor.WithEvidenceGate(bankGate{bank: bank})

	tool := toolByName(t, a.assistantCVTools(testCVID, 9, uuid.New()), "cv_edit")
	_, err := tool.Run(context.Background(), 3, json.RawMessage(
		`{"ops":[{"kind":"insert","path":"experience[0].bullets[0]","value":"Led the migration."}]}`))
	if err == nil {
		t.Fatal("a bullet with no evidence was accepted")
	}
	if !strings.Contains(err.Error(), "experience_search") {
		t.Errorf("error %q does not tell the model how to get an id", err)
	}
}

func TestCVEditWritesACitedBullet(t *testing.T) {
	a, repo := cvToolsAPI(t, oneExperienceCV)
	bank := newStubBank()
	atom := bank.add(3, experience.Atom{
		Claim:      "Ran the payments Kafka cluster.",
		Provenance: experience.ProvenanceStatedInChat,
	})
	a.experience = bank
	a.cv.editor.WithEvidenceGate(bankGate{bank: bank})

	tool := toolByName(t, a.assistantCVTools(testCVID, 9, uuid.New()), "cv_edit")
	_, err := tool.Run(context.Background(), 3, json.RawMessage(
		`{"ops":[{"kind":"insert","path":"experience[0].bullets[0]","value":"Ran the payments Kafka cluster.",`+
			`"evidence_id":"`+atom.ID.String()+`"}]}`))
	if err != nil {
		t.Fatalf("a cited bullet was refused: %v", err)
	}
	if !strings.Contains(string(repo.written), "Kafka") {
		t.Errorf("the bullet was not written: %s", repo.written)
	}
}

// A batch is one entry in the candidate's history and one round of the turn's budget, which
// is why closing a requirement no longer costs three calls.
func TestCVEditAppliesAWholeBatchAtOnce(t *testing.T) {
	a, repo := cvToolsAPI(t, oneExperienceCV)

	tool := toolByName(t, a.assistantCVTools(testCVID, 9, uuid.New()), "cv_edit")
	_, err := tool.Run(context.Background(), 3, json.RawMessage(
		`{"ops":[{"kind":"set","path":"summary","value":"Platform engineer"},`+
			`{"kind":"insert","path":"experience[0].bullets[1]","value":"Ran the cluster"},`+
			`{"kind":"set","path":"experience[0].company","value":"Acme Corp"}],"note":"under the Kubernetes requirement"}`))
	if err != nil {
		t.Fatalf("cv_edit: %v", err)
	}
	written := string(repo.written)
	for _, want := range []string{"Platform engineer", "Ran the cluster", "Acme Corp"} {
		if !strings.Contains(written, want) {
			t.Errorf("stored document is missing %q: %s", want, written)
		}
	}
}
