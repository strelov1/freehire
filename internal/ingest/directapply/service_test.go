package directapply

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/strelov1/freehire/internal/candidate/answerbank"
	"github.com/strelov1/freehire/internal/dict/answertopic"
	"github.com/strelov1/freehire/internal/ingest/applyform"
	"github.com/strelov1/freehire/internal/ingest/employer"
)

const (
	candidateID = int64(7)
	employerID  = int64(3)
	slug        = "platform-engineer-acme"
)

var ownCV = uuid.MustParse("6f1c0c3e-8a51-4c1e-9d55-6a4c1f2b0a01")

// fakeRepo holds one vacancy and the applications recorded against it, standing in for
// the UNIQUE (job_id, user_id) row and its sent_at.
type fakeRepo struct {
	vacancy   Vacancy
	vacErr    error
	form      applyform.Form
	candidate Candidate
	limit     int64 // applications the cap still allows
	records   map[int64]Record
	sent      map[int64]bool
	sending   map[int64]bool
	markErr   error
	nextID    int64
}

func newFakeRepo(t *testing.T) *fakeRepo {
	return &fakeRepo{
		vacancy:   Vacancy{ID: 11, Slug: slug, Title: "Platform Engineer", Company: "Acme", EmployerID: employerID, Open: true},
		form:      testForm(t),
		candidate: Candidate{Email: "c@example.test", Verified: true},
		limit:     30,
		records:   map[int64]Record{},
		sent:      map[int64]bool{},
		sending:   map[int64]bool{},
	}
}

func (r *fakeRepo) Vacancy(_ context.Context, s string) (Vacancy, error) {
	if r.vacErr != nil {
		return Vacancy{}, r.vacErr
	}
	if s != r.vacancy.Slug {
		return Vacancy{}, ErrNotFound
	}
	return r.vacancy, nil
}
func (r *fakeRepo) Form(context.Context, int64) (applyform.Form, error) { return r.form, nil }
func (r *fakeRepo) Candidate(context.Context, int64) (Candidate, error) { return r.candidate, nil }

func (r *fakeRepo) Record(_ context.Context, rec Record, _ int64) (int64, bool, error) {
	for id, existing := range r.records {
		if existing.JobID == rec.JobID && existing.UserID == rec.UserID {
			if r.sent[id] || r.sending[id] {
				return 0, false, nil
			}
			r.records[id] = rec
			r.sending[id] = true
			return id, true, nil
		}
	}
	if r.limit <= 0 {
		return 0, false, nil
	}
	r.limit--
	r.nextID++
	r.records[r.nextID] = rec
	r.sending[r.nextID] = true
	return r.nextID, true, nil
}

func (r *fakeRepo) State(_ context.Context, jobID, userID int64) (State, error) {
	for id, rec := range r.records {
		if rec.JobID == jobID && rec.UserID == userID {
			switch {
			case r.sent[id]:
				return StateSent, nil
			case r.sending[id]:
				return StateSending, nil
			}
			return StateUnsent, nil
		}
	}
	return StateNone, nil
}

func (r *fakeRepo) MarkSent(_ context.Context, id int64) error {
	if r.markErr != nil {
		return r.markErr
	}
	r.sent[id] = true
	delete(r.sending, id)
	return nil
}

func (r *fakeRepo) Release(_ context.Context, id int64) error {
	delete(r.sending, id)
	return nil
}

type fakeEmployers struct{ err error }

func (e fakeEmployers) ActiveAccount(_ context.Context, userID int64) (employer.Account, error) {
	if e.err != nil {
		return employer.Account{}, e.err
	}
	return employer.Account{UserID: userID, CompanyName: "Acme", WorkEmail: "hr@acme.test"}, nil
}

type fakeCVs struct{ renders *int }

func (f fakeCVs) PDF(_ context.Context, userID int64, id uuid.UUID) ([]byte, error) {
	if f.renders != nil {
		*f.renders++
	}
	if userID != candidateID || id != ownCV {
		return nil, ErrNotFound
	}
	return []byte("%PDF"), nil
}

type fakeMailer struct {
	sent []Mail
	err  error
}

func (m *fakeMailer) SendApplication(_ context.Context, mail Mail) error {
	if m.err != nil {
		return m.err
	}
	m.sent = append(m.sent, mail)
	return nil
}

type fakeTracker struct{ applied []string }

func (t *fakeTracker) MarkApplied(_ context.Context, _ int64, s, _ string) error {
	t.applied = append(t.applied, s)
	return nil
}

type fakeBank struct {
	saved    map[string]string
	sendable map[string]string // topic -> answer
}

func (b *fakeBank) Sendable(context.Context, int64) (map[string]string, error) {
	return b.sendable, nil
}

func (b *fakeBank) Save(_ context.Context, _ int64, q, a string, by answerbank.Provenance) error {
	if by != answerbank.AuthorCandidate {
		return errors.New("only the candidate's own answers may be banked")
	}
	b.saved[q] = a
	return nil
}

type harness struct {
	repo    *fakeRepo
	mailer  *fakeMailer
	tracker *fakeTracker
	bank    *fakeBank
	emp     fakeEmployers
	renders int
}

func newHarness(t *testing.T) *harness {
	return &harness{repo: newFakeRepo(t), mailer: &fakeMailer{}, tracker: &fakeTracker{}, bank: &fakeBank{saved: map[string]string{}}}
}

func (h *harness) service() *Service {
	return New(h.repo, h.emp, fakeCVs{renders: &h.renders}, h.mailer, h.tracker, h.bank, 30)
}

func validApplication() Application {
	return Application{CVID: ownCV, Answers: map[string]string{"q1": "true", "q2": "Go"}, Note: " Hi "}
}

func TestApply_DeliversToTheEmployerAndUpdatesTheCandidatesRecords(t *testing.T) {
	h := newHarness(t)
	if err := h.service().Apply(context.Background(), candidateID, slug, validApplication()); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if len(h.mailer.sent) != 1 {
		t.Fatalf("mails = %d, want 1", len(h.mailer.sent))
	}
	m := h.mailer.sent[0]
	if m.To != "hr@acme.test" || m.CandidateEmail != "c@example.test" || m.CompanyName != "Acme" || string(m.CV) != "%PDF" || m.Note != "Hi" {
		t.Errorf("mail = %+v", m)
	}
	if len(m.Answers) != 2 {
		t.Errorf("answers = %+v", m.Answers)
	}
	if len(h.repo.sent) != 1 {
		t.Error("the delivered application is not stamped sent")
	}
	if len(h.tracker.applied) != 1 || h.tracker.applied[0] != slug {
		t.Errorf("tracking = %v, want the vacancy marked applied", h.tracker.applied)
	}
	if h.bank.saved["EU work permit?"] != "Yes" || h.bank.saved["Stack"] != "Go" {
		t.Errorf("banked = %v, want the boolean in words and the choice as given", h.bank.saved)
	}
}

func TestApply_Refusals(t *testing.T) {
	cases := map[string]struct {
		setup func(h *harness)
		app   func() Application
		slug  string
		want  error
	}{
		"not an on-site vacancy": {slug: "something-else", want: ErrNotFound},
		"closed vacancy":         {setup: func(h *harness) { h.repo.vacancy.Open = false }, want: ErrClosed},
		"revoked employer":       {setup: func(h *harness) { h.emp.err = employer.ErrNotActive }, want: ErrClosed},
		"unverified candidate":   {setup: func(h *harness) { h.repo.candidate.Verified = false }, want: ErrUnverified},
		"missing required answer": {
			app:  func() Application { a := validApplication(); delete(a.Answers, "q1"); return a },
			want: ErrInvalid,
		},
		"someone else's CV": {
			app:  func() Application { a := validApplication(); a.CVID = uuid.New(); return a },
			want: ErrNotFound,
		},
		"daily cap":                {setup: func(h *harness) { h.repo.limit = 0 }, want: ErrDailyLimit},
		"deleted employer account": {setup: func(h *harness) { h.emp.err = employer.ErrNotFound }, want: ErrClosed},
		"the employer's own vacancy": {
			setup: func(h *harness) { h.repo.vacancy.EmployerID = candidateID },
			want:  ErrNotFound,
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			h := newHarness(t)
			if tc.setup != nil {
				tc.setup(h)
			}
			app := validApplication()
			if tc.app != nil {
				app = tc.app()
			}
			s := slug
			if tc.slug != "" {
				s = tc.slug
			}
			if err := h.service().Apply(context.Background(), candidateID, s, app); !errors.Is(err, tc.want) {
				t.Fatalf("err = %v, want %v", err, tc.want)
			}
			if len(h.mailer.sent) != 0 || len(h.tracker.applied) != 0 || len(h.bank.saved) != 0 {
				t.Error("a refused application must send nothing and touch no record")
			}
			if tc.want != ErrDailyLimit && len(h.repo.records) != 0 {
				t.Error("a refused application must not be recorded")
			}
		})
	}
}

func TestApply_AFailedDeliveryStaysUnsentAndARetryReplacesIt(t *testing.T) {
	h := newHarness(t)
	h.mailer.err = errors.New("ses throttled")

	if err := h.service().Apply(context.Background(), candidateID, slug, validApplication()); !errors.Is(err, ErrDelivery) {
		t.Fatalf("err = %v, want ErrDelivery", err)
	}
	if len(h.repo.sent) != 0 || len(h.tracker.applied) != 0 {
		t.Fatal("a failed delivery must be neither stamped sent nor tracked as applied")
	}
	if len(h.repo.sending) != 0 {
		t.Fatal("a failed delivery must release its claim so the retry is immediate")
	}

	h.mailer.err = nil
	retry := validApplication()
	retry.Note = "second try"
	if err := h.service().Apply(context.Background(), candidateID, slug, retry); err != nil {
		t.Fatalf("retry: %v", err)
	}
	if len(h.repo.records) != 1 {
		t.Errorf("records = %d, want the one application, not two", len(h.repo.records))
	}
	if h.mailer.sent[0].Note != "second try" {
		t.Errorf("delivered note = %q, want the retry's content", h.mailer.sent[0].Note)
	}
}

func TestApply_ASentApplicationIsNotSentAgain(t *testing.T) {
	h := newHarness(t)
	s := h.service()
	if err := s.Apply(context.Background(), candidateID, slug, validApplication()); err != nil {
		t.Fatalf("first: %v", err)
	}
	if err := s.Apply(context.Background(), candidateID, slug, validApplication()); !errors.Is(err, ErrAlreadyApplied) {
		t.Fatalf("second: err = %v, want ErrAlreadyApplied", err)
	}
	if len(h.mailer.sent) != 1 {
		t.Errorf("mails = %d, want the first only", len(h.mailer.sent))
	}
	if h.renders != 1 {
		t.Errorf("CV renders = %d, want 1 — a refused second application must not render the PDF", h.renders)
	}
}

// A second request while the first is still delivering must not send it again: the
// employer would receive the same application twice.
func TestApply_AnApplicationInFlightIsNotSentTwice(t *testing.T) {
	h := newHarness(t)
	h.repo.records[1] = Record{JobID: h.repo.vacancy.ID, UserID: candidateID}
	h.repo.sending[1] = true
	h.repo.nextID = 1

	if err := h.service().Apply(context.Background(), candidateID, slug, validApplication()); !errors.Is(err, ErrInProgress) {
		t.Fatalf("err = %v, want ErrInProgress", err)
	}
	if len(h.mailer.sent) != 0 {
		t.Error("an application in flight was sent a second time")
	}
}

// Delivered but not stamped: the employer HAS it, so the candidate is told it went.
func TestApply_ADeliveredApplicationSucceedsEvenIfItsStampFails(t *testing.T) {
	h := newHarness(t)
	h.repo.markErr = errors.New("db blip")
	if err := h.service().Apply(context.Background(), candidateID, slug, validApplication()); err != nil {
		t.Fatalf("err = %v, want success — the employer received it", err)
	}
	if len(h.tracker.applied) != 1 {
		t.Error("a delivered application must still reach the candidate's tracking")
	}
}

func TestForm_ShowsTheQuestionsAndPrefillsFromTheBank(t *testing.T) {
	h := newHarness(t)
	topic := func(q string) string {
		k, ok := answertopic.Of(q)
		if !ok {
			t.Fatalf("no topic for %q", q)
		}
		return k
	}
	h.bank.sendable = map[string]string{
		topic("EU work permit?"): "Yes",
		topic("Stack"):           "Java", // not one of the options: offered nowhere
		topic("Anything else?"):  "Remote only",
	}

	view, err := h.service().Form(context.Background(), candidateID, slug)
	if err != nil {
		t.Fatalf("Form: %v", err)
	}
	if view.CompanyName != "Acme" || !view.Open || view.Applied || len(view.Questions) != 3 {
		t.Fatalf("view = %+v", view)
	}
	want := map[string]string{"q1": "true", "q3": "Remote only"}
	if len(view.Prefill) != len(want) {
		t.Fatalf("prefill = %v, want %v", view.Prefill, want)
	}
	for id, v := range want {
		if view.Prefill[id] != v {
			t.Errorf("prefill[%s] = %q, want %q", id, view.Prefill[id], v)
		}
	}
}

func TestForm_AnonymousSeesTheQuestionsOnly(t *testing.T) {
	h := newHarness(t)
	h.bank.sendable = map[string]string{"anything": "x"}
	view, err := h.service().Form(context.Background(), 0, slug)
	if err != nil {
		t.Fatalf("Form: %v", err)
	}
	if len(view.Questions) != 3 || len(view.Prefill) != 0 || view.Applied {
		t.Errorf("view = %+v, want questions without anyone's answers", view)
	}
}

func TestForm_ReportsASentApplicationAndAClosedVacancy(t *testing.T) {
	h := newHarness(t)
	h.repo.records[1] = Record{JobID: h.repo.vacancy.ID, UserID: candidateID}
	h.repo.sent[1] = true
	h.emp.err = employer.ErrNotActive

	view, err := h.service().Form(context.Background(), candidateID, slug)
	if err != nil {
		t.Fatalf("Form: %v", err)
	}
	if !view.Applied || view.Open {
		t.Errorf("view = %+v, want applied and not open", view)
	}
}
