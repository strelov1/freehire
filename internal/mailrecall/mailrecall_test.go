package mailrecall

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/strelov1/freehire/internal/llm"
)

// fakeStore records what the service asked of persistence, so a test can assert on the
// calls that were NOT made as easily as on the ones that were.
type fakeStore struct {
	messages []Message
	listErr  error

	since       time.Time
	until       time.Time
	limit       int32
	suggested   []suggestCall
	suggerr     error
	suggestRows *int64
}

type suggestCall struct {
	emailID    int64
	jobID      int64
	confidence float32
}

func (s *fakeStore) ListForRecall(_ context.Context, _ int64, since, until time.Time, limit int32) ([]Message, error) {
	s.since, s.until, s.limit = since, until, limit
	return s.messages, s.listErr
}

func (s *fakeStore) Suggest(_ context.Context, emailID, _, jobID int64, confidence float32) (int64, error) {
	if s.suggerr != nil {
		return 0, s.suggerr
	}
	s.suggested = append(s.suggested, suggestCall{emailID: emailID, jobID: jobID, confidence: confidence})
	if s.suggestRows != nil {
		return *s.suggestRows, nil
	}
	return 1, nil
}

// fakeGen stands in for the model. It records the prompt so the tests can assert what the
// model was actually shown — the truncation and the HTML-only rule are only real if the
// text that leaves this process carries them.
type fakeGen struct {
	calls    int
	prompt   string
	verdicts []verdict
	raw      string
	err      error
}

func (g *fakeGen) GenerateJSON(_ context.Context, _, user string, _ ...llm.GenOption) (string, error) {
	g.calls++
	g.prompt = user
	if g.err != nil {
		return "", g.err
	}
	if g.raw != "" {
		return g.raw, nil
	}
	out, err := json.Marshal(answer{Verdicts: g.verdicts})
	if err != nil {
		return "", err
	}
	return string(out), nil
}

var appliedAt = time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC)

// svc builds the service around fakes. New takes a *llm.Client so callers cannot forget
// the per-user credential seam; the private field is what a same-package test uses, the
// way mailclassify's tests do.
func svc(store Store, g gen) *Service { return &Service{store: store, gen: g} }

func testApplication() Application {
	return Application{JobID: 77, Company: "Derq", Role: "Backend Engineer", AppliedAt: appliedAt}
}

func msg(id int64, subject string) Message {
	return Message{
		ID: id, FromAddr: "no-reply@ashbyhq.com", FromName: "Ashby",
		Subject: subject, BodyText: "We received your application.",
		ReceivedAt: appliedAt.Add(time.Hour),
	}
}

// The rule the whole package exists under. Store is the only way out to persistence, so
// its method set is where a linking capability would have to appear — and this test makes
// adding one a deliberate act rather than an accident. It is calmatch.Tier.Links() in
// another shape: the next reader must answer the question rather than trip over it.
func TestMailRecallCannotLink(t *testing.T) {
	iface := reflect.TypeOf((*Store)(nil)).Elem()
	allowed := map[string]bool{"ListForRecall": true, "Suggest": true}
	for i := range iface.NumMethod() {
		name := iface.Method(i).Name
		if !allowed[name] {
			t.Errorf("Store gained the method %q. This package proposes and never links: "+
				"a linking method here would let a model's pick reach application_events. "+
				"If the capability is genuinely wanted, change the design first.", name)
		}
	}
	if iface.NumMethod() != len(allowed) {
		t.Errorf("Store has %d methods, want %d", iface.NumMethod(), len(allowed))
	}
}

// A model that names a message it was never shown gets nothing. Bodies are
// attacker-controlled, and this is the answer to one talking the model into reaching
// outside the net.
func TestRecallDiscardsAVerdictOutsideTheBatch(t *testing.T) {
	store := &fakeStore{messages: []Message{msg(1, "Thanks for applying to Derq")}}
	gen := &fakeGen{verdicts: []verdict{
		{EmailID: 1, Belongs: true, Confidence: 0.9},
		{EmailID: 999, Belongs: true, Confidence: 0.99},
	}}

	got, err := svc(store, gen).Recall(context.Background(), 5, testApplication())
	if err != nil {
		t.Fatalf("recall: %v", err)
	}
	if len(got.Proposed) != 1 || got.Proposed[0].Message.ID != 1 {
		t.Fatalf("proposed %+v, want only the message that was actually offered", got.Proposed)
	}
	for _, c := range store.suggested {
		if c.emailID == 999 {
			t.Fatal("a message outside the batch was written to the database")
		}
	}
}

// A button pressed on an application with no mail must not pay for nothing.
func TestRecallWithAnEmptyNetMakesNoModelCall(t *testing.T) {
	store := &fakeStore{}
	gen := &fakeGen{}

	got, err := svc(store, gen).Recall(context.Background(), 5, testApplication())
	if err != nil {
		t.Fatalf("recall: %v", err)
	}
	if gen.calls != 0 {
		t.Errorf("the model was called %d times on an empty net", gen.calls)
	}
	if got.Scanned != 0 || len(got.Proposed) != 0 {
		t.Errorf("got %+v, want nothing scanned and nothing proposed", got)
	}
	if len(store.suggested) != 0 {
		t.Errorf("%d writes on an empty net", len(store.suggested))
	}
}

// The trap the net's whole shape is built around: body_text is empty for HTML-only
// senders, so a service reading it alone shows the model an empty message and gets a
// verdict from the subject line.
func TestRecallShowsTheModelTheHTMLBodyWhenThereIsNoTextPart(t *testing.T) {
	m := msg(1, "Interview")
	m.BodyText = ""
	m.BodyHTML = "<p>We would like to book a call about the Backend role.</p>"
	store := &fakeStore{messages: []Message{m}}
	gen := &fakeGen{verdicts: []verdict{{EmailID: 1, Belongs: true, Confidence: 0.9}}}

	if _, err := svc(store, gen).Recall(context.Background(), 5, testApplication()); err != nil {
		t.Fatalf("recall: %v", err)
	}
	if !strings.Contains(gen.prompt, "book a call about the Backend role") {
		t.Errorf("the model was shown:\n%s\nwant the HTML part rendered down to text", gen.prompt)
	}
}

// One message reads 4000 runes in mailclassify; forty read 800 each here. The cap is the
// difference between one call and one bill.
func TestRecallTruncatesEachBody(t *testing.T) {
	m := msg(1, "Long")
	m.BodyText = strings.Repeat("a", maxBodyRunes*2)
	store := &fakeStore{messages: []Message{m}}
	gen := &fakeGen{verdicts: []verdict{{EmailID: 1, Belongs: true, Confidence: 0.9}}}

	if _, err := svc(store, gen).Recall(context.Background(), 5, testApplication()); err != nil {
		t.Fatalf("recall: %v", err)
	}
	if strings.Contains(gen.prompt, strings.Repeat("a", maxBodyRunes+1)) {
		t.Errorf("the prompt carried an unbroken body run longer than %d runes", maxBodyRunes)
	}
}

// The window opens before the recorded date and closes after it, and both ends earn their
// place. It opens early because the date is when the application was ENTERED, so the
// acknowledgement proving it may predate the entry. It closes at all because the cap is
// what one press costs: left open, a three-month-old application in a busy mailbox spends
// its forty candidates on recent unrelated mail and never shows the model the
// acknowledgement — the button then answers "nothing found" on exactly the applications
// people press it for.
func TestRecallBoundsTheWindowAtBothEnds(t *testing.T) {
	store := &fakeStore{}
	if _, err := svc(store, &fakeGen{}).Recall(context.Background(), 5, testApplication()); err != nil {
		t.Fatalf("recall: %v", err)
	}
	if want := appliedAt.Add(-windowLead); !store.since.Equal(want) {
		t.Errorf("the net opened at %s, want %s", store.since, want)
	}
	if want := appliedAt.Add(windowTrail); !store.until.Equal(want) {
		t.Errorf("the net closed at %s, want %s", store.until, want)
	}
	if store.limit != maxCandidates {
		t.Errorf("the net asked for %d candidates, want the cap of %d", store.limit, maxCandidates)
	}
}

// Belongs-but-unsure is not a proposal. The bar sits below maillink's auto-link threshold
// because nothing here links, and above nothing because a proposal overwrites another
// application's pending one.
func TestRecallKeepsOnlyConfidentVerdicts(t *testing.T) {
	store := &fakeStore{messages: []Message{msg(1, "Sure"), msg(2, "Unsure"), msg(3, "No")}}
	gen := &fakeGen{verdicts: []verdict{
		{EmailID: 1, Belongs: true, Confidence: 0.95},
		{EmailID: 2, Belongs: true, Confidence: 0.69},
		{EmailID: 3, Belongs: false, Confidence: 0.99},
	}}

	got, err := svc(store, gen).Recall(context.Background(), 5, testApplication())
	if err != nil {
		t.Fatalf("recall: %v", err)
	}
	if len(got.Proposed) != 1 || got.Proposed[0].Message.ID != 1 {
		t.Fatalf("proposed %+v, want only the confident belonging message", got.Proposed)
	}
	if got.Proposed[0].Confidence != 0.95 {
		t.Errorf("carried confidence %v, want the model's 0.95", got.Proposed[0].Confidence)
	}
	if got.Scanned != 3 {
		t.Errorf("scanned %d, want the 3 the net caught", got.Scanned)
	}
	if len(store.suggested) != 1 || store.suggested[0].jobID != 77 {
		t.Errorf("wrote %+v, want one suggestion naming job 77", store.suggested)
	}
}

// The count is what lets the card say the meetings are coming without reading a calendar.
func TestRecallCountsProposedInvitations(t *testing.T) {
	withUID := msg(1, "Interview")
	withUID.ICalUID = "derq@ashbyhq.com"
	plain := msg(2, "Thanks")
	uncounted := msg(3, "Rejected")
	uncounted.ICalUID = "other@ashbyhq.com"

	store := &fakeStore{messages: []Message{withUID, plain, uncounted}}
	gen := &fakeGen{verdicts: []verdict{
		{EmailID: 1, Belongs: true, Confidence: 0.9},
		{EmailID: 2, Belongs: true, Confidence: 0.9},
		{EmailID: 3, Belongs: false, Confidence: 0.9},
	}}

	got, err := svc(store, gen).Recall(context.Background(), 5, testApplication())
	if err != nil {
		t.Fatalf("recall: %v", err)
	}
	if got.Invitations != 1 {
		t.Errorf("counted %d invitations, want 1 — only PROPOSED mail counts", got.Invitations)
	}
}

// The person pressed a button and is waiting. An empty success is indistinguishable from
// a mailbox with nothing in it.
func TestRecallPropagatesAModelFailureAndWritesNothing(t *testing.T) {
	store := &fakeStore{messages: []Message{msg(1, "Thanks")}}
	gen := &fakeGen{err: errors.New("gateway down")}

	if _, err := svc(store, gen).Recall(context.Background(), 5, testApplication()); err == nil {
		t.Fatal("a failed model call reported success")
	}
	if len(store.suggested) != 0 {
		t.Errorf("%d writes happened on the failure path", len(store.suggested))
	}
}

// The second half of the no-link guard. The reflection test above watches the Store
// interface, which leaves four ways to break the rule with it still green: reimplement
// DBStore.Suggest over a linking query, add a linking method to DBStore alone, give
// Service a second dependency that bypasses Store, or reach the ledger directly. A scan of
// the package's own source closes the first three, because all of them have to name one of
// these symbols somewhere in this directory.
func TestMailRecallNamesNoLinkingSymbol(t *testing.T) {
	forbidden := []string{
		"LinkEmailToJob", "ConfirmEmailLink", "SetEmailClassification", "AgentTriageEmail",
		"application_events", "ReconcileMailEvent", "MarkJobApplied", "AdvanceStage",
		// And no calendar. The spec says the sweep reads none — the meetings arrive on the
		// next cal-sync, which re-reads its own window — and a calendar holds medical
		// appointments and a current employer's meetings. "We do not read it" is a claim
		// worth a guard rather than a sentence.
		"calsync", "ListEvents", "CalendarReader", "UpsertInterview",
	}
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read package dir: %v", err)
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") || strings.HasSuffix(e.Name(), "_test.go") {
			continue
		}
		src, err := os.ReadFile(e.Name())
		if err != nil {
			t.Fatalf("read %s: %v", e.Name(), err)
		}
		for _, symbol := range forbidden {
			if strings.Contains(string(src), symbol) {
				t.Errorf("%s names %q. This package proposes and never links — a suggestion "+
					"is resolved by the caller through the inbox, and reaching a linking path "+
					"from here would put a model's pick into application_events.", e.Name(), symbol)
			}
		}
	}
}

// The prompt spells the JSON shape out in prose, and the struct tags spell it again. They
// are two copies of one fact, which is what prompt_test.go exists for in mailclassify: a
// key described but not decoded is an answer thrown away.
func TestThePromptNamesTheKeysTheAnswerDecodes(t *testing.T) {
	for _, key := range []string{"verdicts", "email_id", "belongs", "confidence"} {
		if !strings.Contains(systemPrompt, `"`+key+`"`) {
			t.Errorf("the prompt never names %q, which the answer decodes", key)
		}
	}
}

// A tracking row that was never applied to is not an application. The check lives in the
// service because a rule enforced in a handler is a rule the in-process caller never meets.
func TestRecallRefusesATrackedJobThatWasNeverAppliedTo(t *testing.T) {
	store := &fakeStore{messages: []Message{msg(1, "Thanks")}}
	gen := &fakeGen{}

	app := testApplication()
	app.AppliedAt = time.Time{}
	if _, err := svc(store, gen).Recall(context.Background(), 5, app); !errors.Is(err, ErrNotAnApplication) {
		t.Fatalf("got %v, want ErrNotAnApplication", err)
	}
	if gen.calls != 0 {
		t.Error("the model was called for a job that was never applied to")
	}
}

// The statement's guard fires when a candidate is claimed between the net and the write.
// Reporting it anyway would draw a suggestion the database does not hold.
func TestRecallDoesNotReportAProposalTheGuardRefused(t *testing.T) {
	store := &fakeStore{messages: []Message{msg(1, "Thanks")}, suggestRows: new(int64)}
	gen := &fakeGen{verdicts: []verdict{{EmailID: 1, Belongs: true, Confidence: 0.9}}}

	got, err := svc(store, gen).Recall(context.Background(), 5, testApplication())
	if err != nil {
		t.Fatalf("recall: %v", err)
	}
	if len(got.Proposed) != 0 {
		t.Errorf("proposed %+v, want nothing — the write changed no rows", got.Proposed)
	}
	if got.Scanned != 1 {
		t.Errorf("scanned %d, want 1 — the message WAS examined", got.Scanned)
	}
}

// Every failure the caller can meet is a failure, not an empty answer.
func TestRecallSurfacesEveryFailurePath(t *testing.T) {
	// Whether the failure is the MODEL's decides how the caller renders it: a gateway
	// hiccup is a 502, while a dead pool or a cancelled request must stay ours so it
	// reaches an error tracker instead of being filed as a routine bad gateway.
	for name, tc := range map[string]struct {
		build   func() (*fakeStore, *fakeGen)
		isModel bool
	}{
		"the net cannot be read": {func() (*fakeStore, *fakeGen) {
			return &fakeStore{listErr: errors.New("db down")}, &fakeGen{}
		}, false},
		"the model is unreachable": {func() (*fakeStore, *fakeGen) {
			return &fakeStore{messages: []Message{msg(1, "Thanks")}}, &fakeGen{err: errors.New("gateway down")}
		}, true},
		"the answer cannot be read": {func() (*fakeStore, *fakeGen) {
			return &fakeStore{messages: []Message{msg(1, "Thanks")}}, &fakeGen{raw: "not json"}
		}, true},
		"the suggestion cannot be written": {func() (*fakeStore, *fakeGen) {
			return &fakeStore{messages: []Message{msg(1, "Thanks")}, suggerr: errors.New("db down")},
				&fakeGen{verdicts: []verdict{{EmailID: 1, Belongs: true, Confidence: 0.9}}}
		}, false},
	} {
		t.Run(name, func(t *testing.T) {
			store, gen := tc.build()
			got, err := svc(store, gen).Recall(context.Background(), 5, testApplication())
			if err == nil {
				t.Fatalf("%s reported success with %+v", name, got)
			}
			if len(got.Proposed) != 0 {
				t.Errorf("a failed run still proposed %+v", got.Proposed)
			}
			if isModel := errors.Is(err, ErrModel); isModel != tc.isModel {
				t.Errorf("errors.Is(err, ErrModel) = %v, want %v — the caller renders the two differently",
					isModel, tc.isModel)
			}
		})
	}
}
