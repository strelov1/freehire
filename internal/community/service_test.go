package community

import (
	"context"
	"errors"
	"testing"
	"time"
)

// fakeRepo is an in-memory Repository for exercising the service without a DB.
type fakeRepo struct {
	personas    map[int64]Persona
	handles     map[string]int64 // handle -> owner, to simulate UNIQUE(handle)
	threads     map[int64]Thread
	replies     map[int64][]Reply
	nextThread  int64
	nextReply   int64
	threadTimes []time.Time // creation times, for rate counting
	replyTimes  []time.Time

	failHandleOnce bool // simulate one handle collision then succeed
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{
		personas: map[int64]Persona{}, handles: map[string]int64{},
		threads: map[int64]Thread{}, replies: map[int64][]Reply{},
	}
}

func (f *fakeRepo) GetPersona(_ context.Context, userID int64) (Persona, error) {
	if p, ok := f.personas[userID]; ok {
		return p, nil
	}
	return Persona{}, ErrPersonaNotFound
}

func (f *fakeRepo) InsertPersona(_ context.Context, userID int64, handle string) (Persona, error) {
	if p, ok := f.personas[userID]; ok { // concurrent same-user mint resolves to existing
		return p, nil
	}
	if f.failHandleOnce {
		f.failHandleOnce = false
		return Persona{}, ErrHandleTaken
	}
	if _, taken := f.handles[handle]; taken {
		return Persona{}, ErrHandleTaken
	}
	p := Persona{UserID: userID, Handle: handle}
	f.personas[userID] = p
	f.handles[handle] = userID
	return p, nil
}

func (f *fakeRepo) InsertThread(_ context.Context, st, ref, title, body string, author int64) (Thread, error) {
	f.nextThread++
	t := Thread{ID: f.nextThread, SubjectType: st, SubjectRef: ref, Title: title, Body: body, Status: StatusOpen}
	f.threads[t.ID] = t
	f.threadTimes = append(f.threadTimes, time.Now())
	return t, nil
}

func (f *fakeRepo) GetThread(_ context.Context, id int64) (Thread, error) {
	if t, ok := f.threads[id]; ok {
		return t, nil
	}
	return Thread{}, ErrThreadNotFound
}

func (f *fakeRepo) ListOpenThreads(_ context.Context, st, ref string, _ Cursor, _ int32) ([]Thread, error) {
	var out []Thread
	for _, t := range f.threads {
		if t.SubjectType == st && t.SubjectRef == ref && t.Status == StatusOpen {
			out = append(out, t)
		}
	}
	return out, nil
}

func (f *fakeRepo) CountOpenThreads(_ context.Context, st, ref string) (int64, error) {
	var n int64
	for _, t := range f.threads {
		if t.SubjectType == st && t.SubjectRef == ref && t.Status == StatusOpen {
			n++
		}
	}
	return n, nil
}

func (f *fakeRepo) CloseThread(_ context.Context, id int64) error {
	t := f.threads[id]
	t.Status = StatusClosed
	f.threads[id] = t
	return nil
}

func (f *fakeRepo) InsertReply(_ context.Context, threadID, parentReplyID, author int64, body string) (Reply, error) {
	f.nextReply++
	r := Reply{ID: f.nextReply, ThreadID: threadID, ParentID: parentReplyID, Body: body}
	f.replies[threadID] = append(f.replies[threadID], r)
	f.replyTimes = append(f.replyTimes, time.Now())
	return r, nil
}

func (f *fakeRepo) IncrementReplyCount(_ context.Context, threadID int64) error {
	t := f.threads[threadID]
	t.ReplyCount++
	f.threads[threadID] = t
	return nil
}

func (f *fakeRepo) ListReplies(_ context.Context, threadID int64, _ Cursor, _ int32) ([]Reply, error) {
	return f.replies[threadID], nil
}

func (f *fakeRepo) CountRecentThreads(_ context.Context, _ int64, since time.Time) (int64, error) {
	var n int64
	for _, ts := range f.threadTimes {
		if ts.After(since) {
			n++
		}
	}
	return n, nil
}

func (f *fakeRepo) CountRecentReplies(_ context.Context, _ int64, since time.Time) (int64, error) {
	var n int64
	for _, ts := range f.replyTimes {
		if ts.After(since) {
			n++
		}
	}
	return n, nil
}

// fakeSubjects answers existence from a fixed set of "type/slug" keys.
type fakeSubjects struct{ known map[string]bool }

func (s fakeSubjects) SubjectExists(_ context.Context, st, slug string) (bool, error) {
	return s.known[st+"/"+slug], nil
}

func newService(repo *fakeRepo, known ...string) *Service {
	set := map[string]bool{}
	for _, k := range known {
		set[k] = true
	}
	return New(repo, fakeSubjects{known: set}, Config{})
}

func TestCreateThreadHappy(t *testing.T) {
	repo := newFakeRepo()
	svc := newService(repo, "company/acme")
	th, err := svc.CreateThread(context.Background(), CreateThreadInput{
		UserID: 1, SubjectType: SubjectCompany, SubjectSlug: "acme", Title: "Do they ghost?", Body: "asking",
	})
	if err != nil {
		t.Fatalf("CreateThread: %v", err)
	}
	if th.AuthorHandle == "" {
		t.Fatal("expected a persona handle on the created thread")
	}
	if th.SubjectRef != "acme" || th.Title != "Do they ghost?" {
		t.Fatalf("unexpected thread: %+v", th)
	}
}

func TestCreateThreadRejections(t *testing.T) {
	repo := newFakeRepo()
	svc := newService(repo, "company/acme")
	base := CreateThreadInput{UserID: 1, SubjectType: SubjectCompany, SubjectSlug: "acme", Title: "t", Body: "b"}

	cases := []struct {
		name string
		mut  func(CreateThreadInput) CreateThreadInput
		want error
	}{
		{"bad type", func(in CreateThreadInput) CreateThreadInput { in.SubjectType = "user"; return in }, ErrUnsupportedSubject},
		{"unknown subject", func(in CreateThreadInput) CreateThreadInput { in.SubjectSlug = "ghost"; return in }, ErrSubjectNotFound},
		{"empty title", func(in CreateThreadInput) CreateThreadInput { in.Title = "  "; return in }, ErrEmptyTitle},
		{"empty body", func(in CreateThreadInput) CreateThreadInput { in.Body = ""; return in }, ErrEmptyBody},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := svc.CreateThread(context.Background(), c.mut(base))
			if !errors.Is(err, c.want) {
				t.Fatalf("want %v, got %v", c.want, err)
			}
		})
	}
}

func TestPersonaStableAcrossPosts(t *testing.T) {
	repo := newFakeRepo()
	svc := newService(repo, "company/acme")
	in := CreateThreadInput{UserID: 7, SubjectType: SubjectCompany, SubjectSlug: "acme", Title: "t", Body: "b"}
	a, _ := svc.CreateThread(context.Background(), in)
	b, _ := svc.CreateThread(context.Background(), in)
	if a.AuthorHandle != b.AuthorHandle {
		t.Fatalf("same user got two handles: %q vs %q", a.AuthorHandle, b.AuthorHandle)
	}

	other, _ := svc.CreateThread(context.Background(), CreateThreadInput{
		UserID: 8, SubjectType: SubjectCompany, SubjectSlug: "acme", Title: "t", Body: "b"})
	if other.AuthorHandle == a.AuthorHandle {
		t.Fatal("different users must get different handles")
	}
}

func TestPersonaRetriesOnHandleCollision(t *testing.T) {
	repo := newFakeRepo()
	repo.failHandleOnce = true
	svc := newService(repo, "company/acme")
	th, err := svc.CreateThread(context.Background(), CreateThreadInput{
		UserID: 1, SubjectType: SubjectCompany, SubjectSlug: "acme", Title: "t", Body: "b"})
	if err != nil {
		t.Fatalf("expected retry to succeed, got %v", err)
	}
	if th.AuthorHandle == "" {
		t.Fatal("expected a handle after collision retry")
	}
}

func TestThreadRateLimit(t *testing.T) {
	repo := newFakeRepo()
	svc := New(repo, fakeSubjects{known: map[string]bool{"company/acme": true}}, Config{ThreadCap: 2})
	in := CreateThreadInput{UserID: 1, SubjectType: SubjectCompany, SubjectSlug: "acme", Title: "t", Body: "b"}
	for i := 0; i < 2; i++ {
		if _, err := svc.CreateThread(context.Background(), in); err != nil {
			t.Fatalf("post %d: %v", i, err)
		}
	}
	if _, err := svc.CreateThread(context.Background(), in); !errors.Is(err, ErrRateLimited) {
		t.Fatalf("want ErrRateLimited, got %v", err)
	}
}

func TestReplyFlow(t *testing.T) {
	repo := newFakeRepo()
	svc := newService(repo, "company/acme")
	th, _ := svc.CreateThread(context.Background(), CreateThreadInput{
		UserID: 1, SubjectType: SubjectCompany, SubjectSlug: "acme", Title: "t", Body: "b"})

	r, err := svc.Reply(context.Background(), th.ID, 0, 2, "same here")
	if err != nil {
		t.Fatalf("Reply: %v", err)
	}
	if r.AuthorHandle == "" {
		t.Fatal("reply missing persona handle")
	}
	if got := repo.threads[th.ID].ReplyCount; got != 1 {
		t.Fatalf("reply_count = %d, want 1", got)
	}
}

func TestReplyToMissingThread(t *testing.T) {
	repo := newFakeRepo()
	svc := newService(repo)
	if _, err := svc.Reply(context.Background(), 999, 0, 1, "hi"); !errors.Is(err, ErrThreadNotFound) {
		t.Fatalf("want ErrThreadNotFound, got %v", err)
	}
}

func TestReplyToClosedThread(t *testing.T) {
	repo := newFakeRepo()
	svc := newService(repo, "company/acme")
	th, _ := svc.CreateThread(context.Background(), CreateThreadInput{
		UserID: 1, SubjectType: SubjectCompany, SubjectSlug: "acme", Title: "t", Body: "b"})
	if err := svc.Close(context.Background(), th.ID); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if _, err := svc.Reply(context.Background(), th.ID, 0, 2, "hi"); !errors.Is(err, ErrThreadClosed) {
		t.Fatalf("want ErrThreadClosed, got %v", err)
	}
}
