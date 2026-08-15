package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/strelov1/freehire/internal/assistant"
	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/experience"
	"github.com/strelov1/freehire/internal/inbox"
	"github.com/strelov1/freehire/internal/matchanalysis"
)

// A prompt that names a tool the preset does not register produces an agent describing a
// capability it cannot reach. That is not hypothetical here: the interviewer was told to
// resolve achievement ids with a tool that retrieves by meaning, and duly reported that the
// candidate's own achievements did not exist.
func TestProfilePresetRegistersEveryBankToolItsPromptNames(t *testing.T) {
	names := presetAPI().registry(
		assistant.Session{UserID: 3, Preset: assistant.PresetProfile}, uuid.New()).Names()

	prompt := assistant.SystemPrompt(assistant.PresetProfile, "en")
	// Only the bank's own tools: the prompt backticks plenty of field names too
	// (`metrics`, `said`, `soft_duplicate_clusters`), and those are not tools.
	named := regexp.MustCompile("`(experience_[a-z_]+)`").FindAllStringSubmatch(prompt, -1)
	if len(named) == 0 {
		t.Fatal("the profile prompt names no bank tool at all; the interviewer's whole job is the bank")
	}
	for _, match := range named {
		if !slices.Contains(names, match[1]) {
			t.Errorf("the prompt tells the agent to call %q, which this preset does not register: %v", match[1], names)
		}
	}
	// The read is the one the prompt cannot do its job without.
	if !slices.Contains(names, "experience_get") {
		t.Errorf("the profile preset does not register experience_get: %v", names)
	}
}

// The rehearsal's tool set is chosen by exclusion as much as by inclusion. It edits no
// CV, so the CV tools would be a call that changes the wrong document; it reads no
// mailbox, because the one thing it needs from there is placed in its context by the
// server; and no browser is attached, so the page tool could only fail.
func TestInterviewPresetCarriesItsContextAndNothingItCannotUse(t *testing.T) {
	jobID := int64(9)
	names := presetAPI().registry(
		assistant.Session{UserID: 3, Preset: assistant.PresetInterview, JobID: &jobID}, uuid.New()).Names()

	if !slices.Contains(names, "interview_context") {
		t.Errorf("the rehearsal preset does not register interview_context, which its prompt tells the model to call first: %v", names)
	}
	for _, forbidden := range []string{"cv_edit", "cv_get", "cv_context", "tailor_report", "read_current_page"} {
		if slices.Contains(names, forbidden) {
			t.Errorf("the rehearsal preset offers %q, which belongs to another session's job", forbidden)
		}
	}
	for _, name := range names {
		if strings.HasPrefix(name, "inbox_") {
			t.Errorf("the rehearsal preset offers %q; its invitation is placed by the server, not fetched by the model", name)
		}
	}
	// The bank is the point of the rehearsal: an answer worth keeping has to be
	// recordable in the same turn it was given.
	for _, want := range []string{"experience_add", "experience_search", "get_profile"} {
		if !slices.Contains(names, want) {
			t.Errorf("the rehearsal preset is missing %q, which its prompt names: %v", want, names)
		}
	}
}

// stageStub answers the application's stage, or reports that the caller has no
// application against this vacancy at all.
type stageStub struct {
	stage string
	err   error
}

func (s stageStub) GetUserJobStage(context.Context, db.GetUserJobStageParams) (string, error) {
	return s.stage, s.err
}

func (s stageStub) GetJobBySlug(context.Context, string) (db.Job, error) {
	return db.Job{ID: 9}, s.err
}

// invitationStub stands in for the mail service's invitation lookup.
type invitationStub struct {
	msg inbox.Message
	err error
}

func (s invitationStub) InterviewInvitation(context.Context, int64, int64) (inbox.Message, error) {
	return s.msg, s.err
}

// rehearsalAPI wires interview_context over stubbed analysis, bank, job, stage and mail.
func rehearsalAPI(t *testing.T, analysis string, atoms []experience.Atom, stage stageStub, invite invitationStub) *assistantHandlers {
	t.Helper()
	return &assistantHandlers{
		cv: &cvHandlers{
			matchAnalysisCache: analysisCache{analysis: analysis},
			jobReader: jobStub{job: db.Job{
				Title: "Senior Backend Engineer", Company: "Acme",
				PublicSlug: "senior-backend-acme", Description: "We need Kafka in production.",
			}},
		},
		experience: &bankStub{atoms: atoms},
		stages:     stage,
		invitation: invite,
	}
}

// noInvitation is the common case: most applications carry no invitation we can see.
var noInvitation = invitationStub{err: pgx.ErrNoRows}

// The rehearsal's questions come from the vacancy's requirements, and whether the
// candidate can answer one is what the bank already holds for it. Handing the agent
// both in the opening call is what keeps it from spending the turn searching.
func TestInterviewContextCarriesRequirementsWithBankEvidence(t *testing.T) {
	atoms := []experience.Atom{{
		Claim: "Ran a Kafka cluster through a regional outage", Skills: []string{"kafka"},
	}}
	a := rehearsalAPI(t, twoRequirementAnalysis, atoms, stageStub{stage: "interview"}, noInvitation)

	out, err := toolByName(t, a.assistantInterviewTools(9), "interview_context").
		Run(context.Background(), 3, json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("interview_context: %v", err)
	}
	payload := string(mustJSON(t, out))
	for _, want := range []string{"Kafka in production", "Team leadership", "regional outage", "interview"} {
		if !strings.Contains(payload, want) {
			t.Errorf("context omits %q:\n%s", want, payload)
		}
	}
	// The verdict and score set the tone: a rehearsal for a marginal fit is a different
	// conversation from one for a strong one.
	for _, want := range []string{"strong fit", "81"} {
		if !strings.Contains(payload, want) {
			t.Errorf("context omits the analysis' %q, which the rehearsal reads for tone:\n%s", want, payload)
		}
	}
}

// A posting can list thirty requirements; an interview presses on a handful. Every one
// carried here is paid for again on every later turn of the session, so the context keeps
// the ones an interviewer actually works from — the required ones first.
func TestInterviewContextBoundsTheRequirementList(t *testing.T) {
	var reqs []string
	for i := range 20 {
		priority := "preferred"
		if i >= 15 {
			priority = "required"
		}
		reqs = append(reqs, fmt.Sprintf(`{"text":"requirement %d","priority":%q,"status":"missing-gap"}`, i, priority))
	}
	analysis := `{"requirement_match":[` + strings.Join(reqs, ",") + `],"verdict":"fair","overall_score":50}`
	a := rehearsalAPI(t, analysis, nil, stageStub{stage: "interview"}, noInvitation)

	out, err := toolByName(t, a.assistantInterviewTools(9), "interview_context").
		Run(context.Background(), 3, json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("interview_context: %v", err)
	}
	got := out.(interviewContext)
	if len(got.Requirements) != interviewRequirementCap {
		t.Fatalf("carried %d requirements, want the cap of %d", len(got.Requirements), interviewRequirementCap)
	}
	// The five required ones come first: a posting's required list is what the
	// interviewer works from, and a preferred nice-to-have rarely becomes a question.
	for i := range 5 {
		if got.Requirements[i].Priority != matchanalysis.PriorityRequired {
			t.Errorf("requirement %d is %q; the required ones must survive the cap first",
				i, got.Requirements[i].Priority)
		}
	}
}

// An invitation runs to signatures and legal boilerplate. Its useful part — the format,
// the length, who is on the call — is near the top, and the whole of it would be replayed
// into the model's context on every later turn.
func TestInterviewContextBoundsTheInvitationBody(t *testing.T) {
	invite := invitationStub{msg: inbox.Message{
		Subject:  "Interview",
		BodyText: strings.Repeat("boilerplate ", 2000),
	}}
	a := rehearsalAPI(t, twoRequirementAnalysis, nil, stageStub{stage: "interview"}, invite)

	out, err := toolByName(t, a.assistantInterviewTools(9), "interview_context").
		Run(context.Background(), 3, json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("interview_context: %v", err)
	}
	got := out.(interviewContext)
	if got.Invitation == nil {
		t.Fatal("the invitation went missing")
	}
	if len([]rune(got.Invitation.Body)) > interviewInvitationLimit {
		t.Errorf("invitation body is %d runes, want it bounded at %d",
			len([]rune(got.Invitation.Body)), interviewInvitationLimit)
	}
}

// A rehearsal is worth running without a fit analysis — the vacancy and the bank carry
// most of the value. Reporting "run the fit analysis first" would refuse a session the
// candidate opened from an application they are interviewing for tomorrow.
func TestInterviewContextWorksWithoutACachedAnalysis(t *testing.T) {
	a := rehearsalAPI(t, "", nil, stageStub{stage: "interview"}, noInvitation)

	out, err := toolByName(t, a.assistantInterviewTools(9), "interview_context").
		Run(context.Background(), 3, json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("interview_context refused a vacancy with no analysis: %v", err)
	}
	payload := string(mustJSON(t, out))
	if !strings.Contains(payload, "Senior Backend Engineer") {
		t.Errorf("context lost the vacancy when the analysis was missing:\n%s", payload)
	}
}

// The invitation is the one thing in this context an employer wrote. It has to reach
// the model marked as such — this preset carries no mail tool, so the mail section's
// warning never reaches it.
func TestInterviewContextMarksTheInvitationUntrusted(t *testing.T) {
	invite := invitationStub{msg: inbox.Message{
		Subject: "Tech round with the lead", FromName: "Acme Talent",
		BodyText: "45 minutes on distributed systems.",
	}}
	a := rehearsalAPI(t, twoRequirementAnalysis, nil, stageStub{stage: "interview"}, invite)

	out, err := toolByName(t, a.assistantInterviewTools(9), "interview_context").
		Run(context.Background(), 3, json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("interview_context: %v", err)
	}
	payload := string(mustJSON(t, out))
	if !strings.Contains(payload, "distributed systems") {
		t.Errorf("context dropped the invitation's body:\n%s", payload)
	}
	if !strings.Contains(strings.ToLower(payload), "untrusted") {
		t.Errorf("the invitation is not marked untrusted:\n%s", payload)
	}
}

// No invitation is an ordinary state, not a failure — plenty of interviews are arranged
// somewhere we cannot see.
func TestInterviewContextOmitsAMissingInvitation(t *testing.T) {
	a := rehearsalAPI(t, twoRequirementAnalysis, nil, stageStub{stage: "screening"}, noInvitation)

	out, err := toolByName(t, a.assistantInterviewTools(9), "interview_context").
		Run(context.Background(), 3, json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("interview_context failed when the mailbox held no invitation: %v", err)
	}
	if strings.Contains(string(mustJSON(t, out)), "invitation") {
		t.Errorf("context carries an invitation field with nothing in it:\n%s", mustJSON(t, out))
	}
}

// An ATS relay often mails with no display name at all. "From: " with nothing after it
// tells the model less than an address does — the same reason the body falls back from
// text/plain to rendered HTML.
func TestInterviewContextNamesTheSenderWhenTheresNoDisplayName(t *testing.T) {
	invite := invitationStub{msg: inbox.Message{
		Subject: "Interview invitation", FromAddr: "no-reply@ashbyhq.test", FromName: "",
		BodyText: "Tuesday at 10.",
	}}
	a := rehearsalAPI(t, twoRequirementAnalysis, nil, stageStub{stage: "interview"}, invite)

	out, err := toolByName(t, a.assistantInterviewTools(9), "interview_context").
		Run(context.Background(), 3, json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("interview_context: %v", err)
	}
	got := out.(interviewContext)
	if got.Invitation == nil || got.Invitation.From != "no-reply@ashbyhq.test" {
		t.Errorf("invitation from = %q, want the address when the sender left no name", got.Invitation.From)
	}
}

// A tool that reaches a collaborator nobody wired must report it, not panic: the call
// runs inside the SSE writer's goroutine, where Registry.Call's error path cannot reach
// a panic and Fiber's recover is not listening.
func TestInterviewContextReportsAnUnwiredDeployment(t *testing.T) {
	bare := &assistantHandlers{}

	_, err := toolByName(t, bare.assistantInterviewTools(9), "interview_context").
		Run(context.Background(), 3, json.RawMessage(`{}`))
	if err == nil {
		t.Fatal("interview_context answered from a handler wired to nothing")
	}
}

// A rehearsal for an application the caller does not have is not a rehearsal. The stage
// read is the ownership check: user_jobs holds one row per (user, vacancy).
func TestInterviewContextRefusesAVacancyTheCallerHasNotApplied(t *testing.T) {
	a := rehearsalAPI(t, twoRequirementAnalysis, nil, stageStub{err: pgx.ErrNoRows}, noInvitation)

	_, err := toolByName(t, a.assistantInterviewTools(9), "interview_context").
		Run(context.Background(), 3, json.RawMessage(`{}`))
	if err == nil {
		t.Fatal("interview_context served a vacancy the caller never applied to")
	}
}
