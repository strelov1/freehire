package handler

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"

	"github.com/strelov1/freehire/internal/candidate/cv"
	"github.com/strelov1/freehire/internal/candidate/fitanalysis"
	"github.com/strelov1/freehire/internal/candidate/matchanalysis"
	"github.com/strelov1/freehire/internal/platform/db"
)

// jobMatchToolAPI wires job_match over the same fakes cv_job_match_test.go and
// assistant_cv_context_test.go already use, so this test needs no renderer, no database,
// and no model.
func jobMatchToolAPI(t *testing.T, doc string, job db.Job, analysis string) (*assistantHandlers, *cvRepo) {
	t.Helper()
	repo := &cvRepo{id: testCVID, userID: 3, jobID: 9, data: []byte(doc)}
	h := &cvHandlers{
		cvStore:        cv.NewStore(repo),
		cvRenderer:     &fakeCVRenderer{pdf: []byte(scorableCV)},
		extractPDFText: textFromPDF,
		jobReader:      jobStub{job: job},
		fit:            fitanalysis.New(analysisCache{analysis: analysis}, nil, matchanalysis.NewAnalyzer(nil)),
	}
	return &assistantHandlers{cv: h, fit: h.fit, jobs: jobStub{job: job}}, repo
}

func TestJobMatchToolScoresTheCurrentCV(t *testing.T) {
	a, _ := jobMatchToolAPI(t, oneExperienceCV,
		db.Job{Title: "Senior Backend Engineer", Skills: []string{"go", "kafka", "terraform"}},
		twoRequirementAnalysis)

	tool := toolByName(t, a.assistantCVTools(testCVID, 9, uuid.New()), "job_match")
	out, err := tool.Run(context.Background(), 3, json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("job_match: %v", err)
	}
	payload, _ := json.Marshal(out)
	var got struct {
		Available bool `json:"available"`
		Score     struct {
			Overall       int      `json:"overall"`
			MissingSkills []string `json:"missing_skills"`
		} `json:"score"`
	}
	if err := json.Unmarshal(payload, &got); err != nil {
		t.Fatalf("unmarshal: %v (payload=%s)", err, payload)
	}
	if !got.Available {
		t.Fatalf("available = false, want true (payload=%s)", payload)
	}
	// The fake renderer's text layer (scorableCV) names Go and Kafka but not Terraform.
	if len(got.Score.MissingSkills) != 1 || got.Score.MissingSkills[0] != "terraform" {
		t.Errorf("missing skills = %v, want [terraform]", got.Score.MissingSkills)
	}
	if got.Score.Overall <= 0 {
		t.Errorf("overall = %d, want a positive score", got.Score.Overall)
	}
}

// A CV re-scores after an edit changes what its rendered text says — this is the whole
// point of the tool over the cached fit analysis, which must NOT be recomputed mid-turn.
func TestJobMatchToolReflectsAnEditedDocument(t *testing.T) {
	a, repo := jobMatchToolAPI(t, oneExperienceCV,
		db.Job{Title: "Senior Backend Engineer", Skills: []string{"go", "kafka", "terraform"}},
		twoRequirementAnalysis)
	tools := a.assistantCVTools(testCVID, 9, uuid.New())

	before, err := toolByName(t, tools, "job_match").Run(context.Background(), 3, json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("job_match (before): %v", err)
	}

	// Swap in a CV whose rendered text now names Terraform too — the renderer is a fake
	// keyed on nothing but its own field, so this stands in for the workspace's own
	// render-after-save without touching cvedit at all.
	a.cv.cvRenderer = &fakeCVRenderer{pdf: []byte(scorableCV + "\nTerraform")}
	_ = repo // repo's stored document is irrelevant here — the renderer decides the text.

	after, err := toolByName(t, tools, "job_match").Run(context.Background(), 3, json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("job_match (after): %v", err)
	}

	beforePayload, _ := json.Marshal(before)
	afterPayload, _ := json.Marshal(after)
	if string(beforePayload) == string(afterPayload) {
		t.Errorf("score did not change after the rendered text changed: %s", beforePayload)
	}
}

// TestJobMatchReportsAnUnwiredDeployment is the companion to cv_context's own guard: a tool
// that dereferences a collaborator nobody wired must report it, because the call runs inside
// the SSE writer's goroutine where Registry.Call's error path cannot reach a panic and Fiber's
// recover is not listening — so it takes the process down, not one request.
func TestJobMatchReportsAnUnwiredDeployment(t *testing.T) {
	bare := &assistantHandlers{}

	_, err := toolByName(t, bare.assistantCVTools(testCVID, 9, uuid.New()), "job_match").
		Run(context.Background(), 3, json.RawMessage(`{}`))
	if err == nil {
		t.Fatal("job_match answered from a handler wired to nothing")
	}
}
