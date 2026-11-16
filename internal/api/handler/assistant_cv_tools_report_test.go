package handler

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/strelov1/freehire/internal/candidate/cv"
	"github.com/strelov1/freehire/internal/candidate/experience"
)

// cv_edit and tailor_report write two different columns through two different tool calls;
// nothing ties them together unless cv_edit is told which requirement its batch just closed.
// requirement/requirement_status is that link — it must land in the SAME call as the edit,
// because that is the one moment the model still has the requirement in mind.
func TestCVEditToolMergesTheClosedRequirementIntoTheReport(t *testing.T) {
	bank := newStubBank()
	atom := bank.add(3, experience.Atom{Claim: "Senior backend engineer", Provenance: experience.ProvenanceStatedInChat})
	a, repo := cvToolsAPIWithBank(t, oneExperienceCV, bank)

	tool := toolByName(t, a.assistantCVTools(testCVID, &testJobID, uuid.New()), "cv_edit")
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

	tool := toolByName(t, a.assistantCVTools(testCVID, &testJobID, uuid.New()), "cv_edit")
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

	tool := toolByName(t, a.assistantCVTools(testCVID, &testJobID, uuid.New()), "cv_edit")
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

	tool := toolByName(t, a.assistantCVTools(testCVID, &testJobID, uuid.New()), "cv_edit")
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

	tool := toolByName(t, a.assistantCVTools(testCVID, &testJobID, uuid.New()), "cv_edit")
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
	tool := toolByName(t, a.assistantCVTools(testCVID, &testJobID, uuid.New()), "cv_edit")
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

	tool := toolByName(t, a.assistantCVTools(testCVID, &testJobID, uuid.New()), "cv_edit")
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
