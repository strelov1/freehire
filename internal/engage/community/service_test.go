package community

import (
	"context"
	"errors"
	"sort"
	"testing"
	"time"
)

// subjectRow is what the feed query's two LEFT JOINs would have resolved for one
// subject: the display title and the employer, which are separate columns and are
// separately absent.
type subjectRow struct{ title, company string }

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
	// subjectNames stands in for the jobs/companies joins in the feed query, keyed
	// "<subject_type>:<subject_ref>". A missing key is a subject that no longer exists.
	// Title and company are held APART because the query resolves them from different
	// columns (jobs.title vs jobs.company) and they genuinely diverge — a posting whose
	// employer column is empty has a title and no company. A fake that fills both from
	// one string cannot express the case its consumer exists to handle.
	subjectNames map[string]subjectRow

	failHandleOnce bool // simulate one handle collision then succeed
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{
		personas: map[int64]Persona{}, handles: map[string]int64{},
		threads: map[int64]Thread{}, replies: map[int64][]Reply{},
		subjectNames: map[string]subjectRow{},
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

// ListRecentOpenThreads mirrors the SQL: open threads across every subject, newest
// first, with the subject's name resolved on read. subjectNames stands in for the
// jobs/companies joins — a ref absent from it is a subject that no longer exists, and
// the row must still come back with an empty name rather than being filtered out.
func (f *fakeRepo) ListRecentOpenThreads(_ context.Context, _ Cursor, limit int32) ([]ThreadWithSubject, error) {
	var out []ThreadWithSubject
	for _, t := range f.threads {
		if t.Status != StatusOpen {
			continue
		}
		row := f.subjectNames[t.SubjectType+":"+t.SubjectRef]
		out = append(out, ThreadWithSubject{
			Thread:         t,
			SubjectTitle:   row.title,
			SubjectCompany: row.company,
		})
	}
	// The map iteration above is unordered; the real query orders by (created_at DESC,
	// id DESC) and the ids here are monotonic, so id DESC is the same order.
	sort.Slice(out, func(i, j int) bool { return out[i].ID > out[j].ID })
	if limit > 0 && len(out) > int(limit) {
		out = out[:limit]
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
	// Mirrors InsertThreadReply's SQL guard: a non-zero parentReplyID must name a
	// reply that belongs to this same threadID, not just any reply anywhere.
	if parentReplyID != 0 {
		found := false
		for _, r := range f.replies[threadID] {
			if r.ID == parentReplyID {
				found = true
				break
			}
		}
		if !found {
			return Reply{}, ErrInvalidParentReply
		}
	}
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

// A reply cannot be nested under a parent from a DIFFERENT thread — the review
// finding: parentReplyID was passed straight to InsertReply with no check that it
// belongs to threadID, so a caller could nest a reply under an unrelated thread's
// reply.
func TestReplyRejectsParentFromAnotherThread(t *testing.T) {
	repo := newFakeRepo()
	svc := newService(repo, "company/acme", "company/other")
	threadA, _ := svc.CreateThread(context.Background(), CreateThreadInput{
		UserID: 1, SubjectType: SubjectCompany, SubjectSlug: "acme", Title: "t", Body: "b"})
	threadB, _ := svc.CreateThread(context.Background(), CreateThreadInput{
		UserID: 1, SubjectType: SubjectCompany, SubjectSlug: "other", Title: "t2", Body: "b2"})

	parent, err := svc.Reply(context.Background(), threadA.ID, 0, 2, "top level in A")
	if err != nil {
		t.Fatalf("Reply: %v", err)
	}

	if _, err := svc.Reply(context.Background(), threadB.ID, parent.ID, 3, "nested under A's reply"); !errors.Is(err, ErrInvalidParentReply) {
		t.Fatalf("Reply across threads = %v, want ErrInvalidParentReply", err)
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

func TestListRecentThreadsEmpty(t *testing.T) {
	svc := newService(newFakeRepo())
	got, err := svc.ListRecentThreads(context.Background(), Cursor{})
	if err != nil {
		t.Fatalf("ListRecentThreads: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("want no threads, got %d", len(got))
	}
}

// The feed spans subject types — that is its whole point — and names each subject.
func TestListRecentThreadsSpansSubjectTypes(t *testing.T) {
	repo := newFakeRepo()
	repo.subjectNames["company:acme"] = subjectRow{title: "Acme Inc.", company: "Acme Inc."}
	repo.subjectNames["job:senior-go-acme-abc123"] = subjectRow{title: "Senior Go Engineer", company: "Acme Inc."}
	svc := newService(repo, "company/acme", "job/senior-go-acme-abc123")
	ctx := context.Background()

	for _, in := range []CreateThreadInput{
		{UserID: 1, SubjectType: SubjectCompany, SubjectSlug: "acme", Title: "Do they ghost?", Body: "asking"},
		{UserID: 2, SubjectType: SubjectJob, SubjectSlug: "senior-go-acme-abc123", Title: "Dead link", Body: "gone"},
	} {
		if _, err := svc.CreateThread(ctx, in); err != nil {
			t.Fatalf("CreateThread(%s): %v", in.SubjectType, err)
		}
	}

	got, err := svc.ListRecentThreads(ctx, Cursor{})
	if err != nil {
		t.Fatalf("ListRecentThreads: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("want 2 threads, got %d", len(got))
	}
	// Newest first: the job thread was opened second.
	if got[0].SubjectType != SubjectJob || got[1].SubjectType != SubjectCompany {
		t.Fatalf("want job then company, got %s then %s", got[0].SubjectType, got[1].SubjectType)
	}
	if got[0].SubjectTitle != "Senior Go Engineer" {
		t.Fatalf("job thread subject title = %q", got[0].SubjectTitle)
	}
	if got[1].SubjectTitle != "Acme Inc." {
		t.Fatalf("company thread subject title = %q", got[1].SubjectTitle)
	}
}

// A subject can be hard-deleted (cmd/prune) while its threads survive, since no FK
// binds them. The thread must stay in the feed with an unresolved name — dropping it
// would make discussion disappear because the posting it was about did.
func TestListRecentThreadsKeepsThreadWithMissingSubject(t *testing.T) {
	repo := newFakeRepo()
	svc := newService(repo, "job/gone-forever-xyz789")
	ctx := context.Background()
	if _, err := svc.CreateThread(ctx, CreateThreadInput{
		UserID: 1, SubjectType: SubjectJob, SubjectSlug: "gone-forever-xyz789", Title: "Expired", Body: "gone",
	}); err != nil {
		t.Fatalf("CreateThread: %v", err)
	}
	// No entry in subjectNames — the joins found nothing.
	got, err := svc.ListRecentThreads(ctx, Cursor{})
	if err != nil {
		t.Fatalf("ListRecentThreads: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("want the thread kept, got %d rows", len(got))
	}
	if got[0].SubjectTitle != "" || got[0].SubjectCompany != "" {
		t.Fatalf("want empty subject names, got %q / %q", got[0].SubjectTitle, got[0].SubjectCompany)
	}
	if got[0].SubjectRef != "gone-forever-xyz789" {
		t.Fatalf("want the slug preserved for the client fallback, got %q", got[0].SubjectRef)
	}
}

// A closed thread is hidden from every default listing, the feed included.
func TestListRecentThreadsExcludesClosed(t *testing.T) {
	repo := newFakeRepo()
	repo.subjectNames["company:acme"] = subjectRow{title: "Acme Inc.", company: "Acme Inc."}
	svc := newService(repo, "company/acme")
	ctx := context.Background()
	th, err := svc.CreateThread(ctx, CreateThreadInput{
		UserID: 1, SubjectType: SubjectCompany, SubjectSlug: "acme", Title: "spam", Body: "spam",
	})
	if err != nil {
		t.Fatalf("CreateThread: %v", err)
	}
	if err := svc.Close(ctx, th.ID); err != nil {
		t.Fatalf("Close: %v", err)
	}
	got, err := svc.ListRecentThreads(ctx, Cursor{})
	if err != nil {
		t.Fatalf("ListRecentThreads: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("want closed thread excluded, got %d rows", len(got))
	}
}

// A posting whose employer column is empty still RESOLVED — the vacancy is there, only
// its company name is missing. The feed must report the title it found and the absent
// company separately, because the client decides "is the subject gone?" from the title:
// keying that on the company would call a live posting pruned.
func TestListRecentThreadsSeparatesTitleFromCompany(t *testing.T) {
	repo := newFakeRepo()
	repo.subjectNames["job:platform-eng-k8s-e9"] = subjectRow{title: "Platform Engineer", company: ""}
	svc := newService(repo, "job/platform-eng-k8s-e9")
	ctx := context.Background()
	if _, err := svc.CreateThread(ctx, CreateThreadInput{
		UserID: 1, SubjectType: SubjectJob, SubjectSlug: "platform-eng-k8s-e9",
		Title: "Who is hiring for this?", Body: "No company name at all.",
	}); err != nil {
		t.Fatalf("CreateThread: %v", err)
	}
	got, err := svc.ListRecentThreads(ctx, Cursor{})
	if err != nil {
		t.Fatalf("ListRecentThreads: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("want 1 thread, got %d", len(got))
	}
	if got[0].SubjectTitle != "Platform Engineer" {
		t.Fatalf("want the title the join found, got %q", got[0].SubjectTitle)
	}
	if got[0].SubjectCompany != "" {
		t.Fatalf("want the absent company reported absent, got %q", got[0].SubjectCompany)
	}
}
