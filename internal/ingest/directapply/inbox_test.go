package directapply

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/strelov1/freehire/internal/ingest/employer"
)

// fakeInbox holds one employer's delivered applications by id.
type fakeInbox struct {
	apps     map[int64]InboxApplication
	cv       map[int64]Attachment
	lastList InboxFilter
}

func newFakeInbox() *fakeInbox {
	return &fakeInbox{
		apps: map[int64]InboxApplication{
			1: {ID: 1, JobSlug: slug, JobTitle: "Platform Engineer", CandidateEmail: "c@example.test", Stage: StageNew},
			2: {ID: 2, JobSlug: slug, JobTitle: "Platform Engineer", CandidateEmail: "d@example.test", Stage: StageReviewing},
		},
		cv: map[int64]Attachment{1: {Filename: "cv.pdf", ContentType: "application/pdf", Content: []byte("%PDF")}},
	}
}

func (f *fakeInbox) List(_ context.Context, id int64, flt InboxFilter) ([]InboxApplication, error) {
	f.lastList = flt
	if id != employerID {
		return nil, nil
	}
	var out []InboxApplication
	for _, a := range f.apps {
		if flt.Stage == "" || a.Stage == flt.Stage {
			out = append(out, a)
		}
	}
	return out, nil
}

func (f *fakeInbox) Counts(_ context.Context, id int64, flt InboxFilter) (map[Stage]int64, int64, error) {
	out := map[Stage]int64{}
	if id != employerID {
		return out, 0, nil
	}
	var total int64
	for _, a := range f.apps {
		out[a.Stage]++
		if flt.Stage == "" || flt.Stage == a.Stage {
			total++
		}
	}
	return out, total, nil
}

func (f *fakeInbox) UpdateWorkspace(_ context.Context, _, _ int64, _ bool, _ *string) error {
	panic("unexpected workspace update")
}

func (f *fakeInbox) Get(_ context.Context, id, appID int64) (InboxApplication, error) {
	a, ok := f.apps[appID]
	if !ok || id != employerID {
		return InboxApplication{}, ErrNotFound
	}
	return a, nil
}

func (f *fakeInbox) CV(_ context.Context, id, appID int64) (Attachment, error) {
	a, ok := f.cv[appID]
	if !ok || id != employerID {
		return Attachment{}, ErrNotFound
	}
	return a, nil
}

func (f *fakeInbox) SetStage(_ context.Context, id, appID int64, st Stage) (StageMove, error) {
	a, ok := f.apps[appID]
	if !ok || id != employerID {
		return StageMove{}, ErrNotFound
	}
	changed := a.Stage != st
	a.Stage = st
	f.apps[appID] = a
	return StageMove{Changed: changed, CandidateID: candidateID, CandidateEmail: a.CandidateEmail, JobID: 11, JobSlug: a.JobSlug, JobTitle: a.JobTitle, Company: "Acme"}, nil
}

type fakeMirror struct {
	stages   []string
	err      error
	onMirror func()
}

func (m *fakeMirror) Mirror(_ context.Context, _, _ int64, stage string) (bool, error) {
	if m.onMirror != nil {
		m.onMirror()
	}
	m.stages = append(m.stages, stage)
	return m.err == nil, m.err
}

type fakeNotifier struct {
	sent []StatusMail
	err  error
}

func (n *fakeNotifier) SendStatusUpdate(_ context.Context, m StatusMail) error {
	if n.err != nil {
		return n.err
	}
	n.sent = append(n.sent, m)
	return nil
}

// fakeMoves records which applications a move ran for, and whether the move's work ran
// inside the run; its scope is the harness's own repository and mirror.
type fakeMoves struct {
	repo   InboxRepository
	mirror StageMirror
	held   int64
	locked []int64
	// committed is set between fn and its after-commit step, as the real commit is.
	committed bool
}

func (m *fakeMoves) RunMove(ctx context.Context, id int64, fn func(context.Context, MoveScope) (func(context.Context), error)) error {
	m.locked = append(m.locked, id)
	m.held = id
	m.committed = false
	defer func() { m.held = 0 }()
	after, err := fn(ctx, MoveScope{Repo: m.repo, Mirror: m.mirror})
	if err != nil {
		return err
	}
	m.committed = true
	if after != nil {
		after(ctx)
	}
	return nil
}

type inboxHarness struct {
	repo      *fakeInbox
	employers fakeEmployers
	mirror    *fakeMirror
	notifier  *fakeNotifier
	locks     *fakeMoves
}

func newInboxHarness() *inboxHarness {
	h := &inboxHarness{repo: newFakeInbox(), mirror: &fakeMirror{}, notifier: &fakeNotifier{}}
	h.locks = &fakeMoves{repo: h.repo, mirror: h.mirror}
	return h
}

func (h *inboxHarness) inbox() *Inbox {
	return NewInbox(h.repo, h.employers, h.notifier, h.locks)
}

func TestInboxList_CountsEveryStageAndTotalsTheFilter(t *testing.T) {
	h := newInboxHarness()
	page, err := h.inbox().List(context.Background(), employerID, InboxFilter{Stage: StageNew})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(page.Applications) != 1 || page.Applications[0].ID != 1 {
		t.Errorf("applications = %+v, want only the new one", page.Applications)
	}
	if page.Counts[StageNew] != 1 || page.Counts[StageReviewing] != 1 {
		t.Errorf("counts = %v, want every stage counted whatever the filter", page.Counts)
	}
	if page.Total != 1 {
		t.Errorf("total = %d, want the filtered stage's count", page.Total)
	}
	if h.repo.lastList.Limit != DefaultInboxLimit {
		t.Errorf("limit = %d, want the default", h.repo.lastList.Limit)
	}

	page, err = h.inbox().List(context.Background(), employerID, InboxFilter{Limit: 1000})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if page.Total != 2 || h.repo.lastList.Limit != MaxInboxLimit {
		t.Errorf("total = %d, limit = %d; want both stages and the cap", page.Total, h.repo.lastList.Limit)
	}
}

func TestInbox_RefusesAnAccountThatIsNotActive(t *testing.T) {
	h := newInboxHarness()
	h.employers = fakeEmployers{err: employer.ErrNotActive}
	if _, err := h.inbox().List(context.Background(), employerID, InboxFilter{}); !errors.Is(err, employer.ErrNotActive) {
		t.Errorf("List err = %v, want ErrNotActive", err)
	}
	if _, err := h.inbox().CV(context.Background(), employerID, 1); !errors.Is(err, employer.ErrNotActive) {
		t.Errorf("CV err = %v, want ErrNotActive", err)
	}
	if _, err := h.inbox().Move(context.Background(), employerID, 1, StageRejected, ""); !errors.Is(err, employer.ErrNotActive) {
		t.Errorf("Move err = %v, want ErrNotActive", err)
	}
	if h.repo.apps[1].Stage != StageNew {
		t.Error("a refused move changed the stage")
	}
}

func TestInboxList_RefusesAnUnknownStage(t *testing.T) {
	if _, err := newInboxHarness().inbox().List(context.Background(), employerID, InboxFilter{Stage: "ghosted"}); !errors.Is(err, ErrInvalid) {
		t.Errorf("err = %v, want ErrInvalid", err)
	}
}

func TestInboxMove_RejectionMirrorsAndEmailsTheCandidate(t *testing.T) {
	h := newInboxHarness()
	res, err := h.inbox().Move(context.Background(), employerID, 1, StageRejected, "  Thank you — we went with another candidate.  ")
	if err != nil {
		t.Fatalf("Move: %v", err)
	}
	if res.Application.Stage != StageRejected || !res.Notified {
		t.Errorf("result = %+v, want rejected and notified", res)
	}
	if len(h.mirror.stages) != 1 || h.mirror.stages[0] != "rejected" {
		t.Errorf("mirrored = %v, want the candidate's record rejected", h.mirror.stages)
	}
	if len(h.notifier.sent) != 1 {
		t.Fatalf("letters = %d, want 1", len(h.notifier.sent))
	}
	m := h.notifier.sent[0]
	if m.To != "c@example.test" || m.ReplyTo != "hr@acme.test" || m.Stage != StageRejected || m.Message != "Thank you — we went with another candidate." || m.CompanyName != "Acme" {
		t.Errorf("letter = %+v", m)
	}
}

func TestInboxMove_ReviewingMirrorsWithoutALetter(t *testing.T) {
	h := newInboxHarness()
	res, err := h.inbox().Move(context.Background(), employerID, 1, StageReviewing, "")
	if err != nil {
		t.Fatalf("Move: %v", err)
	}
	if res.Notified || len(h.notifier.sent) != 0 {
		t.Error("moving to reviewing emailed the candidate")
	}
	if len(h.mirror.stages) != 1 || h.mirror.stages[0] != "screening" {
		t.Errorf("mirrored = %v, want screening", h.mirror.stages)
	}
}

func TestInboxMove_BackToNewUndoesTheCandidatesStageWithoutALetter(t *testing.T) {
	h := newInboxHarness()
	if _, err := h.inbox().Move(context.Background(), employerID, 2, StageNew, ""); err != nil {
		t.Fatalf("Move: %v", err)
	}
	if len(h.mirror.stages) != 1 || h.mirror.stages[0] != "applied" {
		t.Errorf("mirrored %v, want the candidate's record back at applied", h.mirror.stages)
	}
	if len(h.notifier.sent) != 0 {
		t.Errorf("letters = %d, want none", len(h.notifier.sent))
	}
}

func TestInboxMove_ARepeatedMoveNotifiesNobodyTwice(t *testing.T) {
	h := newInboxHarness()
	for range 2 {
		if _, err := h.inbox().Move(context.Background(), employerID, 1, StageInterview, ""); err != nil {
			t.Fatalf("Move: %v", err)
		}
	}
	if len(h.notifier.sent) != 1 || len(h.mirror.stages) != 1 {
		t.Errorf("letters = %d, mirrors = %d; want one each", len(h.notifier.sent), len(h.mirror.stages))
	}
}

func TestInboxMove_FollowUpFailuresDoNotFailTheMove(t *testing.T) {
	h := newInboxHarness()
	h.mirror.err = errors.New("db down")
	h.notifier.err = errors.New("ses down")
	res, err := h.inbox().Move(context.Background(), employerID, 1, StageOffer, "")
	if err != nil {
		t.Fatalf("Move: %v", err)
	}
	if res.Application.Stage != StageOffer {
		t.Errorf("stage = %q, want the move saved", res.Application.Stage)
	}
	if res.Notified {
		t.Error("a failed letter was reported as sent")
	}
}

func TestInboxMove_Refusals(t *testing.T) {
	cases := []struct {
		name    string
		id      int64
		stage   Stage
		message string
		want    error
	}{
		{"unknown stage", 1, "ghosted", "", ErrInvalid},
		{"a message with a stage that sends no letter", 1, StageReviewing, "hello", ErrInvalid},
		{"an overlong message", 1, StageRejected, strings.Repeat("x", maxMessage+1), ErrInvalid},
		{"another employer's application", 99, StageRejected, "", ErrNotFound},
	}
	for _, c := range cases {
		h := newInboxHarness()
		if _, err := h.inbox().Move(context.Background(), employerID, c.id, c.stage, c.message); !errors.Is(err, c.want) {
			t.Errorf("%s: err = %v, want %v", c.name, err, c.want)
		}
		if len(h.notifier.sent) != 0 || len(h.mirror.stages) != 0 {
			t.Errorf("%s: a refused move had follow-ups", c.name)
		}
	}
}

func TestInboxCV_ServesTheKeptFile(t *testing.T) {
	h := newInboxHarness()
	cv, err := h.inbox().CV(context.Background(), employerID, 1)
	if err != nil || string(cv.Content) != "%PDF" {
		t.Fatalf("CV = %+v, %v", cv, err)
	}
	if _, err := h.inbox().CV(context.Background(), employerID, 2); !errors.Is(err, ErrNotFound) {
		t.Errorf("CV of an application with no kept file: err = %v, want ErrNotFound", err)
	}
}

func TestInboxMove_RunsUnderTheApplicationsLock(t *testing.T) {
	h := newInboxHarness()
	var heldDuringMirror int64
	h.mirror.onMirror = func() { heldDuringMirror = h.locks.held }
	if _, err := h.inbox().Move(context.Background(), employerID, 1, StageInterview, ""); err != nil {
		t.Fatalf("Move: %v", err)
	}
	if len(h.locks.locked) != 1 || h.locks.locked[0] != 1 || heldDuringMirror != 1 {
		t.Errorf("locked %v, held during mirror %d; want the move's follow-ups inside application 1's lock", h.locks.locked, heldDuringMirror)
	}
}

// notifierAfterCommit records whether the move had committed when the letter went out.
type notifierAfterCommit struct {
	moves     *fakeMoves
	committed []bool
}

func (n *notifierAfterCommit) SendStatusUpdate(context.Context, StatusMail) error {
	n.committed = append(n.committed, n.moves.committed)
	return nil
}

func TestInboxMove_TheLetterFollowsTheCommit(t *testing.T) {
	h := newInboxHarness()
	n := &notifierAfterCommit{moves: h.locks}
	in := NewInbox(h.repo, h.employers, n, h.locks)
	res, err := in.Move(context.Background(), employerID, 1, StageRejected, "")
	if err != nil {
		t.Fatalf("Move: %v", err)
	}
	if len(n.committed) != 1 || !n.committed[0] || !res.Notified {
		t.Errorf("letter sent with committed=%v, notified=%v; want one letter after the commit", n.committed, res.Notified)
	}
}

func TestInbox_RejectsInvalidWorkspaceText(t *testing.T) {
	inbox := newInboxHarness().inbox()
	ctx := context.Background()
	if _, err := inbox.List(ctx, employerID, InboxFilter{Search: "Go\x00SQL"}); !errors.Is(err, ErrInvalid) {
		t.Errorf("search with NUL: got %v, want ErrInvalid", err)
	}
	notes := strings.Repeat("é", 4001)
	if _, err := inbox.UpdateWorkspace(ctx, employerID, 1, false, &notes); !errors.Is(err, ErrInvalid) {
		t.Errorf("oversized notes: got %v, want ErrInvalid", err)
	}
}
