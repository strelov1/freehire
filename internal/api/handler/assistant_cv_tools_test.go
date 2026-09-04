package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/candidate/cv"
	"github.com/strelov1/freehire/internal/candidate/cvedit"
	"github.com/strelov1/freehire/internal/candidate/experience"
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
	// Through a bank holding what the claim rests on, because the agent must cite: the tool
	// runs as ActorAgent and production always constructs the editor with the gate.
	bank := newStubBank()
	atom := bank.add(3, experience.Atom{Claim: "Senior backend engineer", Provenance: experience.ProvenanceStatedInChat})
	a, repo := cvToolsAPIWithBank(t, oneExperienceCV, bank)

	tool := toolByName(t, a.assistantCVTools(testCVID, 9, uuid.New()), "cv_edit")
	_, err := tool.Run(context.Background(), 3, json.RawMessage(
		`{"ops":[{"kind":"set","path":"summary","value":"Senior backend engineer","evidence_id":"`+atom.ID.String()+`"}]}`))
	if err != nil {
		t.Fatalf("cv_edit: %v", err)
	}
	if !strings.Contains(string(repo.written), "Senior backend engineer") {
		t.Errorf("stored document = %s, want the patched summary", repo.written)
	}
}

// A model packages the batch as a string holding the array often enough to matter: 15 refusals
// in 30 days on prod, each one an otherwise-correct edit that cost the turn a round. Packaging
// says nothing about the document, so the tool reads through it.
func TestCVEditToolAcceptsOpsAsAJSONString(t *testing.T) {
	bank := newStubBank()
	atom := bank.add(3, experience.Atom{Claim: "Senior backend engineer", Provenance: experience.ProvenanceStatedInChat})
	a, repo := cvToolsAPIWithBank(t, oneExperienceCV, bank)

	tool := toolByName(t, a.assistantCVTools(testCVID, 9, uuid.New()), "cv_edit")
	args, err := json.Marshal(map[string]any{
		"ops": `[{"kind":"set","path":"summary","value":"Senior backend engineer","evidence_id":"` + atom.ID.String() + `"}]`,
	})
	if err != nil {
		t.Fatalf("marshal arguments: %v", err)
	}
	if _, err := tool.Run(context.Background(), 3, json.RawMessage(args)); err != nil {
		t.Fatalf("cv_edit: %v", err)
	}
	if !strings.Contains(string(repo.written), "Senior backend engineer") {
		t.Errorf("stored document = %s, want the patched summary", repo.written)
	}
}

// The tolerance is for packaging only. A field the editor does not define is the shape that
// once let an agent clobber the wrong experience entry while reading 200 back, so it stays
// refused, and the refusal still names what it choked on.
func TestCVEditToolStillRefusesAnUnknownField(t *testing.T) {
	a, _ := cvToolsAPI(t, oneExperienceCV)

	tool := toolByName(t, a.assistantCVTools(testCVID, 9, uuid.New()), "cv_edit")
	_, err := tool.Run(context.Background(), 3, json.RawMessage(
		`{"ops":"[{\"kind\":\"set\",\"path\":\"summary\",\"value\":\"x\",\"skill\":0}]"}`))
	if err == nil {
		t.Fatal("an operation carrying an undefined field must be refused, packaged or not")
	}
	if !strings.Contains(err.Error(), "skill") {
		t.Errorf("error = %v, want it to name the offending field", err)
	}
}

// The model names positions against the document it read, so a batch removing two lines of one
// list must mean what it says. The conversion lives at this boundary and nowhere deeper: the
// editor's other callers state their indices sequentially.
func TestCVEditToolRemovesTwoPositionsOfOneList(t *testing.T) {
	const fourBullets = `{"header":{"full_name":"Ada Lovelace"},"summary":"Backend engineer",` +
		`"experience":[{"company":"Acme","title":"Engineer","bullets":["A","B","C","D"]}]}`
	a, repo := cvToolsAPI(t, fourBullets)

	tool := toolByName(t, a.assistantCVTools(testCVID, 9, uuid.New()), "cv_edit")
	// Non-adjacent, and named against the document the model read.
	_, err := tool.Run(context.Background(), 3, json.RawMessage(
		`{"ops":[{"kind":"remove","path":"experience[0].bullets[1]"},{"kind":"remove","path":"experience[0].bullets[3]"}]}`))
	if err != nil {
		t.Fatalf("cv_edit: %v", err)
	}

	var stored struct {
		Experience []struct {
			Bullets []string `json:"bullets"`
		} `json:"experience"`
	}
	if err := json.Unmarshal(repo.written, &stored); err != nil {
		t.Fatalf("decode stored document: %v", err)
	}
	want := []string{"A", "C"}
	if got := stored.Experience[0].Bullets; !reflect.DeepEqual(got, want) {
		t.Errorf("bullets = %q, want %q", got, want)
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
	bank := newStubBank()
	a, _ := cvToolsAPIWithBank(t, oneExperienceCV, bank)

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
	bank := newStubBank()
	atom := bank.add(3, experience.Atom{
		Claim:      "Ran the payments Kafka cluster.",
		Provenance: experience.ProvenanceStatedInChat,
	})
	a, repo := cvToolsAPIWithBank(t, oneExperienceCV, bank)

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

func TestCVEditToolRefusesAnOverCapInsert(t *testing.T) {
	prev := cv.MaxBullets
	cv.SetMaxBullets(20)
	t.Cleanup(func() { cv.SetMaxBullets(prev) })

	bullets := make([]string, cv.MaxBullets)
	for i := range bullets {
		bullets[i] = fmt.Sprintf("Original bullet %d", i+1)
	}
	doc, err := json.Marshal(cv.Document{
		Experience: []cv.ExperienceItem{{
			Role: "Staff Engineer", Company: "Contoso", Bullets: bullets,
		}},
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	bank := newStubBank()
	atom := bank.add(3, experience.Atom{Claim: "extra", Provenance: experience.ProvenanceStatedInChat})
	a, repo := cvToolsAPIWithBank(t, string(doc), bank)
	before := append([]byte(nil), repo.data...)

	tool := toolByName(t, a.assistantCVTools(testCVID, 9, uuid.New()), "cv_edit")
	_, err = tool.Run(context.Background(), 3, json.RawMessage(
		`{"ops":[{"kind":"insert","path":"experience[0].bullets[0]","value":"extra claim",`+
			`"evidence_id":"`+atom.ID.String()+`"}]}`))
	if err == nil {
		t.Fatal("over-cap insert was accepted")
	}
	if !errors.Is(err, cvedit.ErrListCap) && !strings.Contains(err.Error(), cvedit.ListCapCode) {
		t.Fatalf("error = %v, want ErrListCap / %s", err, cvedit.ListCapCode)
	}
	if repo.written != nil {
		t.Fatalf("CV was written on refuse: %s", repo.written)
	}
	if string(repo.data) != string(before) {
		t.Fatal("stored CV data changed on a refused over-cap edit")
	}
}

func TestCVEditToolDescriptionNamesTheBulletCeiling(t *testing.T) {
	tool := toolByName(t, (&assistantHandlers{}).assistantCVTools(testCVID, 9, uuid.New()), "cv_edit")
	want := fmt.Sprintf("%d", cv.MaxBullets)
	if !strings.Contains(tool.Description, want) {
		t.Fatalf("cv_edit description missing live MaxBullets %s: %s", want, tool.Description)
	}
	if !strings.Contains(strings.ToLower(tool.Description), "refused") {
		t.Fatalf("cv_edit description should say an over-cap insert is refused: %s", tool.Description)
	}
}

func TestCVEditToolDescriptionRoutesProjectsAndTemplateHeading(t *testing.T) {
	tool := toolByName(t, (&assistantHandlers{}).assistantCVTools(testCVID, 9, uuid.New()), "cv_edit")
	d := tool.Description
	if !strings.Contains(d, "projects[i]") {
		t.Fatalf("cv_edit description must mention projects[i] for portfolio work: %s", d)
	}
	if !strings.Contains(d, "experience[i]") {
		t.Fatalf("cv_edit description must mention experience[i] for job roles: %s", d)
	}
	lower := strings.ToLower(d)
	if !strings.Contains(lower, "heading") || !strings.Contains(lower, "template") {
		t.Fatalf("cv_edit description must say the template owns the Projects heading: %s", d)
	}
}

// cv_edit and tailor_report write two different columns through two different tool calls;
// nothing ties them together unless cv_edit is told which requirement its batch just closed.
// requirement/requirement_status is that link — it must land in the SAME call as the edit,
// because that is the one moment the model still has the requirement in mind.
func TestCVEditToolMergesTheClosedRequirementIntoTheReport(t *testing.T) {
	bank := newStubBank()
	atom := bank.add(3, experience.Atom{Claim: "Senior backend engineer", Provenance: experience.ProvenanceStatedInChat})
	a, repo := cvToolsAPIWithBank(t, oneExperienceCV, bank)

	tool := toolByName(t, a.assistantCVTools(testCVID, 9, uuid.New()), "cv_edit")
	_, err := tool.Run(context.Background(), 3, json.RawMessage(
		`{"ops":[{"kind":"set","path":"summary","value":"Senior backend engineer","evidence_id":"`+atom.ID.String()+`"}],`+
			`"requirement":"PostgreSQL experience","requirement_status":"closed_bank"}`))
	if err != nil {
		t.Fatalf("cv_edit: %v", err)
	}

	var report []cv.AutopilotEntry
	if err := json.Unmarshal(repo.report, &report); err != nil {
		t.Fatalf("report unreadable: %v (raw %s)", err, repo.report)
	}
	if len(report) != 1 || report[0].Requirement != "PostgreSQL experience" || report[0].Status != cv.AutopilotClosedBank {
		t.Errorf("report = %+v, want one entry closing \"PostgreSQL experience\"", report)
	}
}

// The merge's replace path (proven at the store level in internal/candidate/cv/autopilot_test.go) has
// to be reachable through cv_edit itself, not just through Store.MergeAutopilotEntry directly
// — this is what proves the handler wiring, not just the store logic, does the right thing
// when a report entry already exists.
func TestCVEditToolReplacesAnExistingOpenReportEntry(t *testing.T) {
	bank := newStubBank()
	atom := bank.add(3, experience.Atom{Claim: "Senior backend engineer", Provenance: experience.ProvenanceStatedInChat})
	a, repo := cvToolsAPIWithBank(t, oneExperienceCV, bank)
	seed, err := json.Marshal([]cv.AutopilotEntry{
		{Requirement: "PostgreSQL experience", Status: cv.AutopilotOpen},
		{Requirement: "Team leadership", Status: cv.AutopilotClosedBank},
	})
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
	repo.report = seed

	tool := toolByName(t, a.assistantCVTools(testCVID, 9, uuid.New()), "cv_edit")
	_, err = tool.Run(context.Background(), 3, json.RawMessage(
		`{"ops":[{"kind":"set","path":"summary","value":"Senior backend engineer","evidence_id":"`+atom.ID.String()+`"}],`+
			`"requirement":"PostgreSQL experience","requirement_status":"closed_bank"}`))
	if err != nil {
		t.Fatalf("cv_edit: %v", err)
	}

	var report []cv.AutopilotEntry
	if err := json.Unmarshal(repo.report, &report); err != nil {
		t.Fatalf("report unreadable: %v (raw %s)", err, repo.report)
	}
	if len(report) != 2 {
		t.Fatalf("report has %d entries, want 2 (the edit must replace, not duplicate)", len(report))
	}
	if report[0].Requirement != "PostgreSQL experience" || report[0].Status != cv.AutopilotClosedBank {
		t.Errorf("report[0] = %+v, want PostgreSQL experience closed_bank", report[0])
	}
	if report[1].Requirement != "Team leadership" || report[1].Status != cv.AutopilotClosedBank {
		t.Errorf("unrelated entry was disturbed: %+v", report[1])
	}
}

// A report-merge failure arrives strictly AFTER cvedit.Commit has already durably landed the
// document edit. Failing the whole tool call at that point would tell the model the edit never
// happened; a well-behaved model retries, and Commit has no content-level dedup against a
// retried insert — so the merge must be best-effort, the same way UndoCVRevisionBatch treats
// the sibling SetAutopilotReport call after RevertBatch has already succeeded.
func TestCVEditToolSucceedsWhenTheReportMergeFailsAfterTheEditLands(t *testing.T) {
	bank := newStubBank()
	atom := bank.add(3, experience.Atom{Claim: "Senior backend engineer", Provenance: experience.ProvenanceStatedInChat})
	a, repo := cvToolsAPIWithBank(t, oneExperienceCV, bank)
	repo.autopilotErr = errors.New("connection reset")

	tool := toolByName(t, a.assistantCVTools(testCVID, 9, uuid.New()), "cv_edit")
	_, err := tool.Run(context.Background(), 3, json.RawMessage(
		`{"ops":[{"kind":"set","path":"summary","value":"Senior backend engineer","evidence_id":"`+atom.ID.String()+`"}],`+
			`"requirement":"PostgreSQL experience","requirement_status":"closed_bank"}`))
	if err != nil {
		t.Fatalf("cv_edit returned an error for a report-merge failure after a successful edit: %v", err)
	}
	if repo.written == nil {
		t.Fatal("the edit did not land — Commit should have run before the failing merge")
	}
}

// The schema's enum keeps a well-behaved model from offering open/not_reached, but the
// handler must refuse the value too — a model can send whatever JSON it wants regardless of
// what the schema advertises.
func TestCVEditToolRejectsAnExplicitOpenRequirementStatus(t *testing.T) {
	bank := newStubBank()
	atom := bank.add(3, experience.Atom{Claim: "Senior backend engineer", Provenance: experience.ProvenanceStatedInChat})
	a, repo := cvToolsAPIWithBank(t, oneExperienceCV, bank)

	tool := toolByName(t, a.assistantCVTools(testCVID, 9, uuid.New()), "cv_edit")
	_, err := tool.Run(context.Background(), 3, json.RawMessage(
		`{"ops":[{"kind":"set","path":"summary","value":"Senior backend engineer","evidence_id":"`+atom.ID.String()+`"}],`+
			`"requirement":"PostgreSQL experience","requirement_status":"open"}`))
	if err == nil {
		t.Fatal("an explicit \"open\" status was accepted; cv_edit must not be able to reopen a requirement")
	}
	if repo.written != nil {
		t.Errorf("document was written = %s, want the batch refused before the edit applied", repo.written)
	}
	if repo.report != nil {
		t.Errorf("report = %s, want it untouched when the call is refused", repo.report)
	}
}

// A batch that names no requirement is the common case (rewording, reordering, adding a
// technology tag) and must leave the report exactly as it was — merging a blank requirement
// would either error or, worse, silently create a nameless row the panel cannot render.
func TestCVEditToolWithNoRequirementLeavesTheReportUntouched(t *testing.T) {
	bank := newStubBank()
	atom := bank.add(3, experience.Atom{Claim: "Senior backend engineer", Provenance: experience.ProvenanceStatedInChat})
	a, repo := cvToolsAPIWithBank(t, oneExperienceCV, bank)

	tool := toolByName(t, a.assistantCVTools(testCVID, 9, uuid.New()), "cv_edit")
	_, err := tool.Run(context.Background(), 3, json.RawMessage(
		`{"ops":[{"kind":"set","path":"summary","value":"Senior backend engineer","evidence_id":"`+atom.ID.String()+`"}]}`))
	if err != nil {
		t.Fatalf("cv_edit: %v", err)
	}
	if repo.report != nil {
		t.Errorf("report = %s, want it untouched when the call named no requirement", repo.report)
	}
}

// requirement_status only makes sense as an outcome the edit just produced — open and
// not_reached describe requirements NOT closed, so cv_edit (which only ever closes one) must
// not advertise or accept them; a model that could send "open" here could silently reopen a
// requirement tailor_report already closed, through a call that carries no report review.
func TestCVEditToolRequirementStatusOffersOnlyClosingOutcomes(t *testing.T) {
	a, _ := cvToolsAPI(t, oneExperienceCV)
	tool := toolByName(t, a.assistantCVTools(testCVID, 9, uuid.New()), "cv_edit")
	props, _ := tool.Schema["properties"].(map[string]any)
	status, _ := props["requirement_status"].(map[string]any)
	got, _ := status["enum"].([]string)

	want := []string{string(cv.AutopilotClosedBank), string(cv.AutopilotClosedCandidate)}
	if len(got) != len(want) {
		t.Fatalf("requirement_status enum = %v, want %v", got, want)
	}
	for _, w := range want {
		found := false
		for _, g := range got {
			if g == w {
				found = true
			}
		}
		if !found {
			t.Errorf("requirement_status enum = %v, missing %q", got, w)
		}
	}
}

func TestCVEditToolRequirementWithoutStatusIsRefused(t *testing.T) {
	bank := newStubBank()
	atom := bank.add(3, experience.Atom{Claim: "Senior backend engineer", Provenance: experience.ProvenanceStatedInChat})
	a, repo := cvToolsAPIWithBank(t, oneExperienceCV, bank)

	tool := toolByName(t, a.assistantCVTools(testCVID, 9, uuid.New()), "cv_edit")
	_, err := tool.Run(context.Background(), 3, json.RawMessage(
		`{"ops":[{"kind":"set","path":"summary","value":"Senior backend engineer","evidence_id":"`+atom.ID.String()+`"}],`+
			`"requirement":"PostgreSQL experience"}`))
	if err == nil {
		t.Fatal("a requirement with no status was accepted; the report would hold an invalid entry")
	}
	if repo.report != nil {
		t.Errorf("report = %s, want it untouched when the call is refused", repo.report)
	}
}

// A batch is one entry in the candidate's history and one round of the turn's budget, which
// is why closing a requirement no longer costs three calls.
func TestCVEditAppliesAWholeBatchAtOnce(t *testing.T) {
	// Every claim-bearing op in the batch cites, because one uncited op refuses the whole
	// batch — that rule is why a batch is worth testing through the real gate at all.
	bank := newStubBank()
	atom := bank.add(3, experience.Atom{Claim: "Ran the cluster", Provenance: experience.ProvenanceCVImport})
	id := atom.ID.String()
	a, repo := cvToolsAPIWithBank(t, oneExperienceCV, bank)

	tool := toolByName(t, a.assistantCVTools(testCVID, 9, uuid.New()), "cv_edit")
	_, err := tool.Run(context.Background(), 3, json.RawMessage(
		`{"ops":[{"kind":"set","path":"summary","value":"Platform engineer","evidence_id":"`+id+`"},`+
			`{"kind":"insert","path":"experience[0].bullets[1]","value":"Ran the cluster","evidence_id":"`+id+`"},`+
			`{"kind":"set","path":"experience[0].company","value":"Acme Corp","evidence_id":"`+id+`"}],"note":"under the Kubernetes requirement"}`))
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

// request_confirmation has no side effect: the client renders it as buttons, and the
// candidate's answer arrives as an ordinary chat message on their next turn, not as a
// second tool call this one waits for.
func TestRequestConfirmationToolIsRegisteredForTailoring(t *testing.T) {
	a, _ := cvToolsAPI(t, oneExperienceCV)

	tool := toolByName(t, a.assistantCVTools(testCVID, 9, uuid.New()), "request_confirmation")
	out, err := tool.Run(context.Background(), 3, json.RawMessage(
		`{"claim":"Built Reelmente.app with React and Next.js","question":"Is that right?"}`))
	if err != nil {
		t.Fatalf("request_confirmation: %v", err)
	}
	payload, _ := json.Marshal(out)
	if !strings.Contains(string(payload), "awaiting_candidate_response") {
		t.Errorf("payload = %s, want status awaiting_candidate_response", payload)
	}
}

// The tool writes nothing and reads nothing — a claim rejected outright (no employment,
// no evidence yet) must still be askable about.
func TestRequestConfirmationToolTouchesNoStore(t *testing.T) {
	a, repo := cvToolsAPI(t, oneExperienceCV)
	before := string(repo.written)

	tool := toolByName(t, a.assistantCVTools(testCVID, 9, uuid.New()), "request_confirmation")
	if _, err := tool.Run(context.Background(), 3, json.RawMessage(
		`{"claim":"Anything","question":"Confirm?"}`)); err != nil {
		t.Fatalf("request_confirmation: %v", err)
	}
	if string(repo.written) != before {
		t.Errorf("request_confirmation wrote to the CV store; it must have no side effect (before=%q after=%q)", before, repo.written)
	}
}
