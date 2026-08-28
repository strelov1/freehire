package handler

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/strelov1/freehire/internal/candidate/experience"
	"github.com/strelov1/freehire/internal/candidate/fitanalysis"
	"github.com/strelov1/freehire/internal/candidate/matchanalysis"
	"github.com/strelov1/freehire/internal/platform/db"
)

// analysisCache serves one cached fit analysis, so cv_context can be exercised without a
// database behind it.
type analysisCache struct{ analysis string }

func (c analysisCache) GetUserJobAnalysis(context.Context, db.GetUserJobAnalysisParams) (db.GetUserJobAnalysisRow, error) {
	return db.GetUserJobAnalysisRow{Analysis: []byte(c.analysis)}, nil
}
func (c analysisCache) UpsertUserJobAnalysis(context.Context, db.UpsertUserJobAnalysisParams) error {
	return nil
}
func (c analysisCache) ListUserJobAnalyses(context.Context, int64) ([]db.ListUserJobAnalysesRow, error) {
	return nil, nil
}

// bankStub answers retrieval from a fixed atom list, recording the queries it was asked.
type bankStub struct {
	atoms   []experience.Atom
	queries []experience.Query
}

func (b *bankStub) Retrieve(_ context.Context, _ int64, q experience.Query, limit int) ([]experience.Match, error) {
	b.queries = append(b.queries, q)
	var out []experience.Match
	for _, a := range b.atoms {
		// The stub stands in for scoring, not for it: an atom matches when the query text
		// names one of its skills, which is enough to tell "found" from "not found" apart.
		for _, s := range a.Skills {
			if strings.Contains(strings.ToLower(q.Text), s) {
				out = append(out, experience.Match{Atom: a, Score: 10})
				break
			}
		}
	}
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (b *bankStub) ListEmployments(context.Context, int64) ([]experience.Employment, error) {
	return nil, nil
}
func (b *bankStub) ListAtoms(context.Context, int64) ([]experience.Atom, error) { return b.atoms, nil }
func (b *bankStub) GetAtom(context.Context, uuid.UUID, int64) (experience.Atom, error) {
	return experience.Atom{}, experience.ErrNotFound
}
func (b *bankStub) AddAtom(_ context.Context, _ int64, a experience.Atom) (experience.Atom, error) {
	return a, nil
}
func (b *bankStub) UpdateAtom(_ context.Context, _ uuid.UUID, _ int64, a experience.Atom) (experience.Atom, error) {
	return a, nil
}
func (b *bankStub) MergeAtoms(context.Context, int64, uuid.UUID, uuid.UUID) (experience.Atom, error) {
	return experience.Atom{}, experience.ErrNotFound
}

const twoRequirementAnalysis = `{
	"requirement_match": [
		{"text":"Kafka in production","priority":"required","status":"missing-have"},
		{"text":"Team leadership","priority":"preferred","status":"missing-gap"}
	],
	"verdict":"strong fit","overall_score":81
}`

// contextToolAPI wires cv_context over a stubbed analysis cache, bank and job row.
func contextToolAPI(t *testing.T, description string, atoms []experience.Atom) (*assistantHandlers, *bankStub) {
	t.Helper()
	bank := &bankStub{atoms: atoms}
	// No cvHandlers in sight: cv_context reads the fit service and one job row, and now says
	// so — the point of moving these reads off the CV surface.
	h := &assistantHandlers{
		fit:        fitanalysis.New(analysisCache{analysis: twoRequirementAnalysis}, nil, matchanalysis.NewAnalyzer(nil)),
		jobs:       jobStub{job: db.Job{Title: "Senior Backend Engineer", Company: "Acme", PublicSlug: "senior-backend-acme", Description: description}},
		experience: bank,
	}
	return h, bank
}

// jobStub serves the one vacancy the tailoring context is about.
type jobStub struct{ job db.Job }

func (s jobStub) GetJob(context.Context, int64) (db.Job, error) { return s.job, nil }

const htmlDescription = `<div><p>We need <strong>Kafka</strong> in production.</p><ul><li>Lead a team</li></ul></div>`

// The posting is the largest and least trusted text in the turn. It reaches the model as
// words, not markup — the same treatment get_job already gives it.
func TestCVContextRendersTheDescriptionWithoutMarkup(t *testing.T) {
	a, _ := contextToolAPI(t, htmlDescription, nil)

	tool := toolByName(t, a.assistantCVTools(testCVID, 9, uuid.New()), "cv_context")
	out, err := tool.Run(context.Background(), 3, json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("cv_context: %v", err)
	}
	payload, _ := json.Marshal(out)
	if strings.Contains(string(payload), "<p>") || strings.Contains(string(payload), "<ul>") {
		t.Errorf("description reached the model as markup:\n%s", payload)
	}
	if !strings.Contains(string(payload), "Kafka") {
		t.Errorf("description lost its words:\n%s", payload)
	}
}

func TestCVContextBoundsALongDescription(t *testing.T) {
	long := "<p>" + strings.Repeat("word ", 20000) + "</p>"
	a, _ := contextToolAPI(t, long, nil)

	tool := toolByName(t, a.assistantCVTools(testCVID, 9, uuid.New()), "cv_context")
	out, err := tool.Run(context.Background(), 3, json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("cv_context: %v", err)
	}
	payload, _ := json.Marshal(out)
	if len(payload) > 2*tailorDescriptionLimit {
		t.Errorf("context is %d bytes for a %d-byte bound; the opening round is the most expensive one in the turn", len(payload), tailorDescriptionLimit)
	}
}

// The retrieval that answers "can I evidence this requirement?" is a local scan over the
// candidate's own atoms. Making the agent ask for it one requirement at a time is what spent
// ten tool rounds and left no room to edit.
func TestCVContextCarriesTheBanksAnswerPerRequirement(t *testing.T) {
	atomID := uuid.MustParse("11111111-1111-4111-8111-111111111111")
	a, bank := contextToolAPI(t, htmlDescription, []experience.Atom{{
		ID:         atomID,
		Claim:      "Ran the payments Kafka cluster through a 10x traffic year.",
		Skills:     []string{"kafka"},
		Provenance: experience.ProvenanceStatedInChat,
	}})

	tool := toolByName(t, a.assistantCVTools(testCVID, 9, uuid.New()), "cv_context")
	out, err := tool.Run(context.Background(), 3, json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("cv_context: %v", err)
	}
	payload, _ := json.Marshal(out)

	// The evidenced requirement arrives with the id a cv_edit must cite.
	if !strings.Contains(string(payload), atomID.String()) {
		t.Errorf("the Kafka requirement carries no evidence id:\n%s", payload)
	}
	// "Looked and found nothing" must be distinguishable from "did not look".
	var decoded struct {
		MissingHave []struct {
			Text     string            `json:"text"`
			Evidence []json.RawMessage `json:"evidence"`
		} `json:"missing_have"`
		MissingGap []struct {
			Text     string            `json:"text"`
			Evidence []json.RawMessage `json:"evidence"`
		} `json:"missing_gap"`
	}
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatalf("context is not decodable: %v\n%s", err, payload)
	}
	if len(decoded.MissingHave) != 1 || len(decoded.MissingHave[0].Evidence) != 1 {
		t.Errorf("missing_have = %+v, want the Kafka requirement with one piece of evidence", decoded.MissingHave)
	}
	if len(decoded.MissingGap) != 1 || decoded.MissingGap[0].Evidence == nil {
		t.Errorf("missing_gap = %+v, want an EMPTY evidence list rather than an absent one", decoded.MissingGap)
	}
	if len(bank.queries) != 2 {
		t.Errorf("retrieval ran %d times, want once per reported requirement", len(bank.queries))
	}
}

// A bank that cannot be read is not a reason to withhold the vacancy's requirements.
func TestCVContextSurvivesAnUnavailableBank(t *testing.T) {
	a, _ := contextToolAPI(t, htmlDescription, nil)
	a.experience = nil

	tool := toolByName(t, a.assistantCVTools(testCVID, 9, uuid.New()), "cv_context")
	out, err := tool.Run(context.Background(), 3, json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("cv_context with no bank: %v", err)
	}
	if !strings.Contains(string(mustJSON(t, out)), "Kafka in production") {
		t.Error("the requirements went missing when the bank did")
	}
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return b
}

// The agent's context is narrower than the endpoint's on purpose: dimension comments,
// strengths, gaps and the recommendation were 3 KB of a measured 11.4 KB result, and none of
// them is something a CV edit can be made from. The candidate reads them in the panel.
func TestCVContextLeavesTheNarrativeToThePanel(t *testing.T) {
	a, _ := contextToolAPI(t, htmlDescription, nil)

	tool := toolByName(t, a.assistantCVTools(testCVID, 9, uuid.New()), "cv_context")
	out, err := tool.Run(context.Background(), 3, json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("cv_context: %v", err)
	}
	payload := string(mustJSON(t, out))
	for _, unwanted := range []string{`"dimensions"`, `"strengths"`, `"gaps"`, `"recommendation"`} {
		if strings.Contains(payload, unwanted) {
			t.Errorf("the agent's context still carries %s — it cannot edit a CV from it and must not restate it:\n%s", unwanted, payload)
		}
	}
	// What it does work from stays.
	for _, wanted := range []string{`"missing_have"`, `"missing_gap"`, `"job"`, `"verdict"`} {
		if !strings.Contains(payload, wanted) {
			t.Errorf("the agent's context lost %s:\n%s", wanted, payload)
		}
	}
}

// TestCVContextReportsAnUnwiredDeployment mirrors the interview tool's guard. The tool runs
// inside the SSE writer's goroutine, where Registry.Call's error path cannot reach a panic and
// Fiber's recover is not listening — so a collaborator nobody wired must be a sentence the
// model can act on, not a segfault that takes the process down.
func TestCVContextReportsAnUnwiredDeployment(t *testing.T) {
	bare := &assistantHandlers{}

	_, err := toolByName(t, bare.assistantCVTools(testCVID, 9, uuid.New()), "cv_context").
		Run(context.Background(), 3, json.RawMessage(`{}`))
	if err == nil {
		t.Fatal("cv_context answered from a handler wired to nothing")
	}
}
