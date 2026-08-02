package mailrecall

import (
	"context"
	"encoding/json"
	"errors"
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

	since     time.Time
	limit     int32
	suggested []suggestCall
	suggerr   error
}

type suggestCall struct {
	emailID    int64
	jobID      int64
	confidence float32
}

func (s *fakeStore) ListForRecall(_ context.Context, _ int64, since time.Time, limit int32) ([]Message, error) {
	s.since, s.limit = since, limit
	return s.messages, s.listErr
}

func (s *fakeStore) Suggest(_ context.Context, emailID, _, jobID int64, confidence float32) (int64, error) {
	if s.suggerr != nil {
		return 0, s.suggerr
	}
	s.suggested = append(s.suggested, suggestCall{emailID: emailID, jobID: jobID, confidence: confidence})
	return 1, nil
}

// fakeGen stands in for the model. It records the prompt so the tests can assert what the
// model was actually shown — the truncation and the HTML-only rule are only real if the
// text that leaves this process carries them.
type fakeGen struct {
	calls    int
	prompt   string
	verdicts []verdict
	err      error
}

func (g *fakeGen) GenerateJSON(_ context.Context, _, user string, _ ...llm.GenOption) (string, error) {
	g.calls++
	g.prompt = user
	if g.err != nil {
		return "", g.err
	}
	out, err := json.Marshal(answer{Verdicts: g.verdicts})
	if err != nil {
		return "", err
	}
	return string(out), nil
}

var appliedAt = time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC)

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

	got, err := New(store, gen).Recall(context.Background(), 5, testApplication())
	if err != nil {
		t.Fatalf("recall: %v", err)
	}
	if len(got.Proposed) != 1 || got.Proposed[0] != 1 {
		t.Fatalf("proposed %v, want only the message that was actually offered", got.Proposed)
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

	got, err := New(store, gen).Recall(context.Background(), 5, testApplication())
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

	if _, err := New(store, gen).Recall(context.Background(), 5, testApplication()); err != nil {
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

	if _, err := New(store, gen).Recall(context.Background(), 5, testApplication()); err != nil {
		t.Fatalf("recall: %v", err)
	}
	if strings.Contains(gen.prompt, strings.Repeat("a", maxBodyRunes+1)) {
		t.Errorf("the prompt carried an unbroken body run longer than %d runes", maxBodyRunes)
	}
}

// The window opens before the recorded date, because the date is when the application was
// entered and the acknowledgement that proves it may predate that entry.
func TestRecallOpensTheWindowBeforeTheRecordedDate(t *testing.T) {
	store := &fakeStore{}
	if _, err := New(store, &fakeGen{}).Recall(context.Background(), 5, testApplication()); err != nil {
		t.Fatalf("recall: %v", err)
	}
	want := appliedAt.Add(-windowLead)
	if !store.since.Equal(want) {
		t.Errorf("the net opened at %s, want %s", store.since, want)
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
		{EmailID: 2, Belongs: true, Confidence: minConfidence - 0.01},
		{EmailID: 3, Belongs: false, Confidence: 0.99},
	}}

	got, err := New(store, gen).Recall(context.Background(), 5, testApplication())
	if err != nil {
		t.Fatalf("recall: %v", err)
	}
	if len(got.Proposed) != 1 || got.Proposed[0] != 1 {
		t.Fatalf("proposed %v, want only the confident belonging message", got.Proposed)
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

	got, err := New(store, gen).Recall(context.Background(), 5, testApplication())
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

	if _, err := New(store, gen).Recall(context.Background(), 5, testApplication()); err == nil {
		t.Fatal("a failed model call reported success")
	}
	if len(store.suggested) != 0 {
		t.Errorf("%d writes happened on the failure path", len(store.suggested))
	}
}
