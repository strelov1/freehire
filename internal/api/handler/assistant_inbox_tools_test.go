package handler

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/application/inbox"
	"github.com/strelov1/freehire/internal/application/mailclassify"
	"github.com/strelov1/freehire/internal/platform/db"
)

// mailStore is an in-memory inbox.Queries. The tools are exercised through the
// real service, because half of what is being asserted (the vocabulary, the link
// state, what is written) is the service's and must not be re-implemented here.
type mailStore struct {
	list  []db.ListEmailsRow
	total int64
	state []db.CountEmailsByStateRow

	lastList db.ListEmailsParams
	triaged  int
}

func (m *mailStore) ListEmails(_ context.Context, arg db.ListEmailsParams) ([]db.ListEmailsRow, error) {
	m.lastList = arg
	return m.list, nil
}
func (m *mailStore) CountEmails(context.Context, db.CountEmailsParams) (db.CountEmailsRow, error) {
	return db.CountEmailsRow{Total: m.total}, nil
}
func (m *mailStore) CountEmailsByState(context.Context, int64) ([]db.CountEmailsByStateRow, error) {
	return m.state, nil
}
func (m *mailStore) GetEmail(context.Context, db.GetEmailParams) (db.GetEmailRow, error) {
	return db.GetEmailRow{ID: 812, Subject: "Interview with Acme"}, nil
}

// No invitation is the honest default for a mailbox nothing seeded: most applications
// never carry one, and a blank row would read as "there is an invitation, it is empty".
func (m *mailStore) GetInterviewInvitation(context.Context, db.GetInterviewInvitationParams) (db.GetInterviewInvitationRow, error) {
	return db.GetInterviewInvitationRow{}, pgx.ErrNoRows
}
func (m *mailStore) GetJobIDBySlug(context.Context, string) (int64, error) {
	return 42, nil
}
func (m *mailStore) AgentTriageEmail(context.Context, db.AgentTriageEmailParams) (int64, error) {
	m.triaged++
	return 1, nil
}
func (m *mailStore) LinkEmailToJob(context.Context, db.LinkEmailToJobParams) (int64, error) {
	return 1, nil
}
func (m *mailStore) UnlinkEmail(context.Context, db.UnlinkEmailParams) (int64, error) { return 1, nil }
func (m *mailStore) ConfirmEmailLink(context.Context, db.ConfirmEmailLinkParams) (int64, error) {
	return 1, nil
}
func (m *mailStore) RejectEmailLink(context.Context, db.RejectEmailLinkParams) (int64, error) {
	return 1, nil
}
func (m *mailStore) GetUserJobStage(context.Context, db.GetUserJobStageParams) (string, error) {
	return "", nil
}
func (m *mailStore) AdvanceUserJobStage(context.Context, db.AdvanceUserJobStageParams) error {
	return nil
}

// mailAPI wires the assistant handlers over an in-memory mail store.
func mailAPI(store *mailStore) *assistantHandlers {
	return &assistantHandlers{mail: &inboxHandlers{inbox: inbox.New(store, nil)}}
}

// runMailTool calls one mail tool and returns its result.
func runMailTool(t *testing.T, a *assistantHandlers, name, args string) (any, error) {
	t.Helper()
	return toolByName(t, a.assistantInboxTools(), name).Run(context.Background(), 3, json.RawMessage(args))
}

// A tool result is persisted in the transcript and replayed into the model's
// context on every later turn, so a page of bodies is charged again to every
// question that follows. The HTTP surface lets a harness take fifty; the model
// takes ten however many it asks for.
func TestInboxSearchCapsBodyPagesBelowTheHarnessCeiling(t *testing.T) {
	store := &mailStore{}
	a := mailAPI(store)

	if _, err := runMailTool(t, a, "inbox_search", `{"include_body":true,"limit":50}`); err != nil {
		t.Fatalf("inbox_search: %v", err)
	}
	if store.lastList.Lim != assistantInboxBodyMax {
		t.Errorf("asked the store for %d messages with bodies, want the cap of %d",
			store.lastList.Lim, assistantInboxBodyMax)
	}
	if assistantInboxBodyMax >= harnessPageMax {
		t.Errorf("the model's body cap (%d) is not below the harness's (%d); the asymmetry is the point",
			assistantInboxBodyMax, harnessPageMax)
	}
}

// Without bodies a row still has to carry enough to answer: who wrote, about what,
// when, under which label, and whether it is attached to an application.
func TestInboxSearchRowsAnswerWithoutBodies(t *testing.T) {
	store := &mailStore{total: 1, list: []db.ListEmailsRow{{
		ID: 812, Source: "gmail", FromName: "Workable", FromAddr: "no-reply@workablemail.com",
		Subject: "Interview with Acme", Snippet: "We would like to talk",
		BodyText:     "We would like to talk about the Go role",
		ReceivedAt:   pgtype.Timestamptz{Time: time.Unix(1_700_000_000, 0), Valid: true},
		StatusSignal: pgtype.Text{String: "interview_invitation", Valid: true},
		JobID:        pgtype.Int8{Int64: 42, Valid: true},
		LinkedSlug:   pgtype.Text{String: "go-dev-acme", Valid: true},
	}}}

	got, err := runMailTool(t, mailAPI(store), "inbox_search", `{"label":"interview_invitation"}`)
	if err != nil {
		t.Fatalf("inbox_search: %v", err)
	}
	rendered := renderTool(t, got)
	for _, want := range []string{"Workable", "Interview with Acme", "interview_invitation", "go-dev-acme", "linked"} {
		if !strings.Contains(rendered, want) {
			t.Errorf("a body-less row omits %q: %s", want, rendered)
		}
	}
	if strings.Contains(rendered, "about the Go role") {
		t.Errorf("a body-less row leaked the body: %s", rendered)
	}
	if store.lastList.WithBody {
		t.Error("inbox_search asked the store for bodies nobody requested")
	}
}

// The orientation call must stay cheap enough to keep forever, so it carries
// counts and no content at all.
func TestInboxOverviewNamesNoMessage(t *testing.T) {
	store := &mailStore{state: []db.CountEmailsByStateRow{
		{Label: "interview_invitation", N: 3, Unread: 3, Linked: 3},
	}}

	got, err := runMailTool(t, mailAPI(store), "inbox_overview", `{}`)
	if err != nil {
		t.Fatalf("inbox_overview: %v", err)
	}
	rendered := renderTool(t, got)
	for _, leak := range []string{"subject", "from", "body", "snippet"} {
		if strings.Contains(strings.ToLower(rendered), leak) {
			t.Errorf("inbox_overview carries %q; it must report shape, not content: %s", leak, rendered)
		}
	}
	// Every label, including the ones at zero — "none" and "no such label" are
	// different answers, and the model can only tell them apart if both are named.
	for _, label := range mailclassify.SignalValues {
		if !strings.Contains(rendered, label) {
			t.Errorf("inbox_overview omits the label %q: %s", label, rendered)
		}
	}
}

// mailclassify.Sanitize would coerce this to `other`; that is right for the worker
// reading an attacker's mail and wrong here, where the label is a judgement the
// user asked for. The error is the model's only route to self-correction inside
// the turn, so it has to carry the vocabulary.
func TestInboxTriageRefusesAnUnknownLabelWithTheVocabulary(t *testing.T) {
	store := &mailStore{}

	_, err := runMailTool(t, mailAPI(store), "inbox_triage", `{"id":812,"label":"ghosted"}`)

	if err == nil {
		t.Fatal("inbox_triage accepted a label outside the vocabulary")
	}
	if store.triaged != 0 {
		t.Error("inbox_triage wrote a classification for a label it refused")
	}
	for _, want := range []string{"ghosted", "interview_invitation", "rejection"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the error %q does not name %q", err, want)
		}
	}
}

// Omitting the slug classifies without touching the link. Clearing is unlink.
func TestInboxTriageWithoutASlugIsAccepted(t *testing.T) {
	store := &mailStore{}

	if _, err := runMailTool(t, mailAPI(store), "inbox_triage", `{"id":812,"label":"rejection"}`); err != nil {
		t.Fatalf("inbox_triage without a slug: %v", err)
	}
	if store.triaged != 1 {
		t.Error("inbox_triage did not reach the store")
	}
}

func TestInboxResolveSuggestionRejectsAnUnknownDecision(t *testing.T) {
	_, err := runMailTool(t, mailAPI(&mailStore{}), "inbox_resolve_suggestion", `{"id":812,"decision":"maybe"}`)

	if err == nil {
		t.Fatal("inbox_resolve_suggestion accepted a decision that is neither")
	}
	for _, want := range []string{"maybe", "confirm", "reject"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the error %q does not name %q", err, want)
		}
	}
}

// A prompt injection carried in a message body has no outbound channel to reach:
// nothing in this group sends anything.
func TestNoMailToolSendsMail(t *testing.T) {
	for _, tool := range mailAPI(&mailStore{}).assistantInboxTools() {
		name := strings.ToLower(tool.Name)
		if strings.Contains(name, "send") || strings.Contains(name, "reply") || strings.Contains(name, "draft") {
			t.Errorf("the mail tool group offers %q; an injection in a body must have nowhere to go", tool.Name)
		}
	}
}

// Sweeping the backlog must not cost the user their unread count, so no tool opens
// a single message.
func TestNoMailToolOpensOneMessage(t *testing.T) {
	for _, tool := range mailAPI(&mailStore{}).assistantInboxTools() {
		if tool.Name == "inbox_read" || tool.Name == "inbox_get" || tool.Name == "get_email" {
			t.Errorf("the mail tool group offers %q, which would mark mail read", tool.Name)
		}
	}
}

// A session whose mail surface was never wired up must report that, not panic.
func TestMailToolsReportAnUnwiredSurface(t *testing.T) {
	a := &assistantHandlers{}

	if _, err := runMailTool(t, a, "inbox_overview", `{}`); err == nil {
		t.Error("inbox_overview succeeded with no mail service behind it")
	}
}

// assistantInboxToolsAreRegisteredUnderTheirDocumentedNames pins the names the
// prompt and the specs refer to.
func TestMailToolNames(t *testing.T) {
	var got []string
	for _, tool := range mailAPI(&mailStore{}).assistantInboxTools() {
		got = append(got, tool.Name)
	}
	if len(got) != len(assistantMailTools) {
		t.Fatalf("the group has %d tools %v, want %d", len(got), got, len(assistantMailTools))
	}
	for i, want := range assistantMailTools {
		if got[i] != want {
			t.Errorf("tool %d = %q, want %q", i, got[i], want)
		}
	}
}

// renderTool marshals a tool result the way it reaches the model.
func renderTool(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal tool result: %v", err)
	}
	return string(b)
}

// The page cap bounds how MANY bodies come back; nothing bounded how LARGE each
// one is. Real ATS mail is HTML-only and renders to tens of kilobytes of text, so
// ten of them overflow the registry's result cap — the model then gets a
// "truncated" envelope holding one message instead of the ten it asked for, and
// has to spend another round narrowing. The classifier already truncates each body
// before judging it; the tool has to as well, and for the harder reason: this text
// is replayed into the context on every later turn.
func TestInboxSearchTruncatesEachBody(t *testing.T) {
	huge := strings.Repeat("a very long recruiter template. ", 4000)
	rows := make([]db.ListEmailsRow, assistantInboxBodyMax)
	for i := range rows {
		rows[i] = db.ListEmailsRow{ID: int64(i + 1), BodyText: huge}
	}
	store := &mailStore{list: rows, total: int64(len(rows))}

	got, err := runMailTool(t, mailAPI(store), "inbox_search", `{"include_body":true}`)
	if err != nil {
		t.Fatalf("inbox_search: %v", err)
	}
	rendered := renderTool(t, got)
	if len(rendered) >= assistantResultCap {
		t.Errorf("a full page of bodies rendered %d bytes, at or over the registry cap of %d — the model would get a truncation envelope instead of its page",
			len(rendered), assistantResultCap)
	}
}

// A caller that names no limit must get a page, not an empty one. LIMIT 0 is a
// legal query returning nothing, so a zero has to mean "unspecified" here rather
// than reaching the store.
func TestSearchWithNoLimitStillReturnsAPage(t *testing.T) {
	store := &mailStore{}

	if _, err := inbox.New(store, nil).Search(context.Background(), 3, inbox.Query{}); err != nil {
		t.Fatalf("Search: %v", err)
	}
	if store.lastList.Lim <= 0 {
		t.Errorf("Search asked the store for LIMIT %d; a zero must not reach it", store.lastList.Lim)
	}
}

func (m *mailStore) RetractSupersededEmailEvent(context.Context, db.RetractSupersededEmailEventParams) (int64, error) {
	return 0, nil
}

func (m *mailStore) RecordEmailApplicationEvent(context.Context, db.RecordEmailApplicationEventParams) error {
	return nil
}
