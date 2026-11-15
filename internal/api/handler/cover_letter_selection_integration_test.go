//go:build integration

package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/strelov1/freehire/internal/candidate/experience"
	"github.com/strelov1/freehire/internal/platform/llm"
	"github.com/tmc/langchaingo/llms"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/strelov1/freehire/internal/ai/plan"
	"github.com/strelov1/freehire/internal/candidate/coverletter"
	"github.com/strelov1/freehire/internal/platform/db"
)

func TestCoverLetterEvidenceRoutesPersistAndConflict(t *testing.T) {
	f := newCoverLetterFixture(t, startPostgres(t))
	path := "/api/v1/me/cvs/" + f.tailored.ID.String() + "/cover-letter/evidence"
	resp := doCV(t, f.app, http.MethodGet, path, f.token, nil)
	if resp.StatusCode != 200 {
		t.Fatalf("GET status %d", resp.StatusCode)
	}
	resp.Body.Close()
	resp = doCV(t, f.app, http.MethodPut, path, f.token, map[string]any{"revision": 0, "atom_ids": []string{}})
	if resp.StatusCode != 200 {
		t.Fatalf("PUT status %d", resp.StatusCode)
	}
	resp.Body.Close()
	resp = doCV(t, f.app, http.MethodPut, path, f.token, map[string]any{"revision": 0, "atom_ids": []string{}})
	if resp.StatusCode != 409 {
		t.Fatalf("stale PUT status %d", resp.StatusCode)
	}
	resp.Body.Close()
	resp = doCV(t, f.app, http.MethodPut, path, f.token, map[string]any{"revision": 1, "atom_ids": []string{uuid.NewString()}})
	if resp.StatusCode != 422 {
		t.Fatalf("foreign fact status %d", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestEmptySelectionRefusedBeforeModelOrCharge(t *testing.T) {
	pool := startPostgres(t)
	f := newCoverLetterFixture(t, pool)
	makePro(t, pool, f.userID)
	f.h.plans = plan.NewStore(db.New(pool), pool, plan.DefaultConfig().WithProCeiling(plan.FeatureCoverLetter, 0).Enforcing())
	ctx := context.Background()
	if _, err := f.letters.SaveSelection(ctx, f.userID, f.jobID, 0, nil, nil); err != nil {
		t.Fatal(err)
	}
	for _, suffix := range []string{"", "/stream"} {
		resp := doCV(t, f.app, fiber.MethodPost, "/api/v1/me/cvs/"+f.tailored.ID.String()+"/cover-letter"+suffix, f.token, nil)
		if resp.StatusCode != 409 {
			t.Fatalf("%s status %d", suffix, resp.StatusCode)
		}
		var body map[string]any
		_ = json.NewDecoder(resp.Body).Decode(&body)
		resp.Body.Close()
	}
}

type evidenceModel struct {
	*letterModel
	seen   []string
	change func()
}

func (m *evidenceModel) GenerateContent(ctx context.Context, msgs []llms.MessageContent, opts ...llms.CallOption) (*llms.ContentResponse, error) {
	payload, _ := json.Marshal(msgs)
	m.seen = append(m.seen, string(payload))
	if m.change != nil {
		change := m.change
		m.change = nil
		change()
	}
	return m.letterModel.GenerateContent(ctx, msgs, opts...)
}

func TestSelectedEvidenceConstrainsPromptsAndStoresInitialSnapshot(t *testing.T) {
	f := newCoverLetterFixture(t, startPostgres(t))
	ctx := context.Background()
	chosen, err := f.bank.AddAtom(ctx, f.userID, experience.Atom{Claim: "SELECTED achievement"}, experience.AuthorCandidate)
	if err != nil {
		t.Fatal(err)
	}
	other, err := f.bank.AddAtom(ctx, f.userID, experience.Atom{Claim: "EXCLUDED secret achievement"}, experience.AuthorCandidate)
	if err != nil {
		t.Fatal(err)
	}
	bank := []experience.Atom{chosen, other}
	if _, err = f.letters.SaveSelection(ctx, f.userID, f.jobID, 0, []uuid.UUID{chosen.ID}, bank); err != nil {
		t.Fatal(err)
	}
	seedAnalysis(t, f.h, f.userID, f.jobID)
	model := &evidenceModel{letterModel: chainFor(chosen.ID, "AUDITED")}
	model.change = func() {
		if _, e := f.letters.SaveSelection(ctx, f.userID, f.jobID, 1, []uuid.UUID{other.ID}, bank); e != nil {
			t.Fatal(e)
		}
	}
	f.h.letter.chain = coverletter.NewAnalyzer(llm.NewWithModel(model))
	f.h.llm = llmBinding{client: llm.NewWithModel(model)}
	got, status := f.post(t, f.tailored.ID.String())
	if status != 200 {
		t.Fatalf("draft status %d", status)
	}
	if got.Letter == nil {
		t.Fatal("no draft")
	}
	for _, prompt := range model.seen {
		if strings.Contains(prompt, "EXCLUDED") {
			t.Fatal("unselected fact in model input")
		}
	}
	stored, err := f.letters.Get(ctx, f.userID, f.jobID)
	if err != nil || stored == nil {
		t.Fatalf("stored: %v", err)
	}
	if stored.EvidenceRevision != 1 || stored.EvidenceFingerprint != coverletter.EvidenceFingerprint([]experience.Atom{chosen}) {
		t.Fatal("snapshot changed during generation")
	}
	view, status := f.get(t, f.tailored.ID.String())
	if status != 200 || !view.EvidenceChanged {
		t.Fatal("newer selection not reported")
	}
}

func TestAutomaticDraftStoresEvidenceMetadata(t *testing.T) {
	f := newCoverLetterFixture(t, startPostgres(t))
	ctx := context.Background()
	atom, err := f.bank.AddAtom(ctx, f.userID, experience.Atom{Claim: "Confirmed work"}, experience.AuthorCandidate)
	if err != nil {
		t.Fatal(err)
	}
	seedAnalysis(t, f.h, f.userID, f.jobID)
	model := chainFor(atom.ID, "AUDITED")
	f.h.letter.chain = coverletter.NewAnalyzer(llm.NewWithModel(model))
	f.h.llm = llmBinding{client: llm.NewWithModel(model)}
	if _, status := f.post(t, f.tailored.ID.String()); status != 200 {
		t.Fatalf("draft: %d", status)
	}
	stored, err := f.letters.Get(ctx, f.userID, f.jobID)
	if err != nil || stored == nil || stored.EvidenceFingerprint == "" {
		t.Fatalf("automatic snapshot missing: %v %v", stored, err)
	}
}

func TestSavedEmptySelectionBlocksDirectDraftButton(t *testing.T) {
	f := newCoverLetterFixture(t, startPostgres(t))
	if _, err := f.letters.SaveSelection(context.Background(), f.userID, f.jobID, 0, nil, nil); err != nil {
		t.Fatal(err)
	}
	resp := doCV(t, f.app, fiber.MethodGet, "/api/v1/me/cvs/"+f.tailored.ID.String()+"/cover-letter", f.token, nil)
	defer resp.Body.Close()
	var body struct {
		Data struct {
			Blocked bool `json:"evidence_blocked"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if !body.Data.Blocked {
		t.Fatal("empty selection did not disable generation")
	}
}

func TestEvidenceReviewIncludesEmploymentContext(t *testing.T) {
	f := newCoverLetterFixture(t, startPostgres(t))
	ctx := context.Background()
	role, err := f.bank.CreateEmployment(ctx, f.userID, experience.Employment{Kind: experience.KindJob, Company: "Acme", Role: "Engineer"})
	if err != nil {
		t.Fatal(err)
	}
	resp := doCV(t, f.app, fiber.MethodGet, "/api/v1/me/cvs/"+f.tailored.ID.String()+"/cover-letter/evidence", f.token, nil)
	defer resp.Body.Close()
	var body struct {
		Data struct {
			Labels map[string]string `json:"employment_labels"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.Data.Labels[role.ID.String()] != "Acme · Engineer" {
		t.Fatal("employment context missing")
	}
}
