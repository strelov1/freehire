package telegram

import (
	"context"
	"errors"
	"strconv"
	"testing"
	"time"

	"github.com/strelov1/freehire/internal/job/job"
)

type fakeExtractor struct {
	result  Extraction
	err     error
	prompts []Kind
}

func (f *fakeExtractor) Extract(_ context.Context, _ string, kind Kind) (Extraction, error) {
	f.prompts = append(f.prompts, kind)
	if f.err != nil {
		return Extraction{}, f.err
	}
	return f.result, nil
}

type completion struct {
	post PendingPost
	jobs []job.Job
}

type linkCompletion struct {
	post PendingPost
	jobs []job.Job
}

type fakeExtractStore struct {
	pending       []PendingPost
	completed     []completion
	linkCompleted []linkCompletion
	failures      []string
}

// fakeLinkResolver returns canned resolved jobs (and/or an error) for any post's links.
type fakeLinkResolver struct {
	jobs  []ResolvedJob
	err   error
	calls int
}

func (f *fakeLinkResolver) Resolve(_ context.Context, _ []Link) ([]ResolvedJob, error) {
	f.calls++
	return f.jobs, f.err
}

func (s *fakeExtractStore) Claim(_ context.Context, _ int32, batch int32) ([]PendingPost, error) {
	n := int(batch)
	if n > len(s.pending) {
		n = len(s.pending)
	}
	out := s.pending[:n]
	s.pending = s.pending[n:]
	return out, nil
}

func (s *fakeExtractStore) Complete(_ context.Context, post PendingPost, jobs []job.Job) error {
	s.completed = append(s.completed, completion{post: post, jobs: jobs})
	return nil
}

func (s *fakeExtractStore) CompleteLinks(_ context.Context, post PendingPost, jobs []job.Job) error {
	s.linkCompleted = append(s.linkCompleted, linkCompletion{post: post, jobs: jobs})
	return nil
}

func (s *fakeExtractStore) Fail(_ context.Context, post PendingPost, msg string) error {
	s.failures = append(s.failures, post.Channel+": "+msg)
	return nil
}

func pendingPost() PendingPost {
	return PendingPost{
		Channel:  "hrlunapark",
		MsgID:    392,
		Text:     "tl;dr: ML & full-stack engineers, $110k-220k, London ...",
		PostedAt: time.Date(2026, 5, 28, 12, 3, 7, 0, time.UTC),
	}
}

func kinds() map[string]Kind {
	return map[string]Kind{"hrlunapark": KindAuthored}
}

func TestExtractCompletesWithExtractedJobs(t *testing.T) {
	ex := &fakeExtractor{result: Extraction{Jobs: []ExtractedJob{
		{Title: "ML Engineer", Company: "Claimsorted", Description: "AI claims workflows, $120k-220k"},
		{Title: "Full-stack Engineer", Company: "Claimsorted", Description: "Next.js/Node, $110k-200k"},
	}}}
	store := &fakeExtractStore{pending: []PendingPost{pendingPost()}}
	r := ExtractRunner{Extractor: ex, Store: store, Kinds: kinds()}

	stats, err := r.Run(context.Background())
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if stats.Processed != 1 || stats.Jobs != 2 || stats.Failed != 0 {
		t.Errorf("stats = %+v, want Processed=1 Jobs=2 Failed=0", stats)
	}
	if len(store.completed) != 1 || len(store.completed[0].jobs) != 2 {
		t.Fatalf("completed = %+v, want one completion with 2 jobs", store.completed)
	}
	if ex.prompts[0] != KindAuthored {
		t.Errorf("extractor got kind %q, want authored (from config)", ex.prompts[0])
	}
}

func TestExtractZeroJobsIsANormalCompletion(t *testing.T) {
	ex := &fakeExtractor{result: Extraction{}}
	store := &fakeExtractStore{pending: []PendingPost{pendingPost()}}
	r := ExtractRunner{Extractor: ex, Store: store, Kinds: kinds()}

	stats, err := r.Run(context.Background())
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if stats.Processed != 1 || stats.Jobs != 0 || stats.Failed != 0 {
		t.Errorf("stats = %+v, want Processed=1 Jobs=0 Failed=0", stats)
	}
	if len(store.completed) != 1 || len(store.completed[0].jobs) != 0 {
		t.Errorf("want a zero-job completion, got %+v", store.completed)
	}
}

func TestExtractInvalidPayloadIsFailedNotPersisted(t *testing.T) {
	ex := &fakeExtractor{result: Extraction{Jobs: []ExtractedJob{{Title: ""}}}}
	store := &fakeExtractStore{pending: []PendingPost{pendingPost()}}
	r := ExtractRunner{Extractor: ex, Store: store, Kinds: kinds()}

	stats, err := r.Run(context.Background())
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if stats.Failed != 1 || stats.Processed != 0 {
		t.Errorf("stats = %+v, want Failed=1 Processed=0", stats)
	}
	if len(store.completed) != 0 {
		t.Errorf("invalid payload was persisted: %+v", store.completed)
	}
	if len(store.failures) != 1 {
		t.Errorf("failures = %v, want 1", store.failures)
	}
}

func TestExtractLLMErrorIsFailed(t *testing.T) {
	ex := &fakeExtractor{err: errors.New("llm down")}
	store := &fakeExtractStore{pending: []PendingPost{pendingPost()}}
	r := ExtractRunner{Extractor: ex, Store: store, Kinds: kinds()}

	stats, err := r.Run(context.Background())
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if stats.Failed != 1 {
		t.Errorf("stats = %+v, want Failed=1", stats)
	}
}

func linkPost() PendingPost {
	p := pendingPost()
	p.Links = []Link{{Text: "Product manager в СберЗдоровье", URL: "https://u.habr.com/PnBO7"}}
	return p
}

func TestExtractRoutesLinkPostToResolverAndSkipsLLM(t *testing.T) {
	resolver := &fakeLinkResolver{jobs: []ResolvedJob{
		{Source: "habr_career", ExternalID: "1000166712", Title: "Product manager", Company: "СберЗдоровье", Description: "<p>...</p>"},
	}}
	ex := &fakeExtractor{result: Extraction{Jobs: []ExtractedJob{{Title: "should not be used"}}}}
	store := &fakeExtractStore{pending: []PendingPost{linkPost()}}
	r := ExtractRunner{Extractor: ex, Store: store, Kinds: kinds(), Links: resolver}

	stats, err := r.Run(context.Background())
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if stats.Processed != 1 || stats.Jobs != 1 || stats.Failed != 0 {
		t.Errorf("stats = %+v, want Processed=1 Jobs=1 Failed=0", stats)
	}
	if len(store.linkCompleted) != 1 || len(store.linkCompleted[0].jobs) != 1 {
		t.Fatalf("linkCompleted = %+v, want one completion with 1 job", store.linkCompleted)
	}
	if got := store.linkCompleted[0].jobs[0].Fields().Source; got != "habr_career" {
		t.Errorf("resolved job source = %q, want habr_career", got)
	}
	if len(ex.prompts) != 0 {
		t.Errorf("LLM extractor was called %d times, want 0 (link post bypasses the LLM)", len(ex.prompts))
	}
	if len(store.completed) != 0 {
		t.Errorf("LLM completion path used: %+v", store.completed)
	}
}

func TestExtractFallsBackToLLMWhenNoLinkMatches(t *testing.T) {
	resolver := &fakeLinkResolver{jobs: nil} // links present but none matched a destination
	ex := &fakeExtractor{result: Extraction{Jobs: []ExtractedJob{{Title: "ML Engineer", Company: "Acme", Description: "x"}}}}
	store := &fakeExtractStore{pending: []PendingPost{linkPost()}}
	r := ExtractRunner{Extractor: ex, Store: store, Kinds: kinds(), Links: resolver}

	stats, err := r.Run(context.Background())
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if stats.Processed != 1 || stats.Jobs != 1 {
		t.Errorf("stats = %+v, want Processed=1 Jobs=1 via the LLM fallback", stats)
	}
	if len(ex.prompts) != 1 {
		t.Errorf("LLM extractor calls = %d, want 1 (fallback)", len(ex.prompts))
	}
	if len(store.completed) != 1 || len(store.linkCompleted) != 0 {
		t.Errorf("want one LLM completion and no link completion; got completed=%+v link=%+v", store.completed, store.linkCompleted)
	}
}

func TestExtractFailsPostWhenAllLinksFail(t *testing.T) {
	resolver := &fakeLinkResolver{err: errors.New("habr 503")}
	ex := &fakeExtractor{result: Extraction{Jobs: []ExtractedJob{{Title: "should not run"}}}}
	store := &fakeExtractStore{pending: []PendingPost{linkPost()}}
	r := ExtractRunner{Extractor: ex, Store: store, Kinds: kinds(), Links: resolver}

	stats, err := r.Run(context.Background())
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if stats.Failed != 1 || stats.Processed != 0 {
		t.Errorf("stats = %+v, want Failed=1 Processed=0", stats)
	}
	if len(ex.prompts) != 0 {
		t.Errorf("LLM was called on a failed-link post: %d times", len(ex.prompts))
	}
	if len(store.failures) != 1 {
		t.Errorf("failures = %v, want 1 (retry via lease)", store.failures)
	}
}

func TestExtractUnknownChannelKindDefaultsToBoard(t *testing.T) {
	ex := &fakeExtractor{result: Extraction{}}
	store := &fakeExtractStore{pending: []PendingPost{{Channel: "unlisted", MsgID: 1, Text: "x", PostedAt: time.Now()}}}
	r := ExtractRunner{Extractor: ex, Store: store, Kinds: kinds()}

	if _, err := r.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(ex.prompts) != 1 || ex.prompts[0] != KindBoard {
		t.Errorf("kind = %v, want board fallback for a channel no longer configured", ex.prompts)
	}
}

// TestExtractCountsDroppedJobsAsSkippedNotWritten covers the extraction path's blind spot:
// Validate drops a malformed vacancy in place and keeps the rest, and the run used to report
// only what survived — so a post whose extraction was half garbage looked identical to a clean
// one naming a single role.
func TestExtractCountsDroppedJobsAsSkippedNotWritten(t *testing.T) {
	ex := &fakeExtractor{result: Extraction{Jobs: []ExtractedJob{
		{Title: "ML Engineer", Company: "Claimsorted", Description: "AI claims workflows"},
		// No description: Validate drops this one and keeps the first.
		{Title: "Full-stack Engineer", Company: "Claimsorted"},
	}}}
	store := &fakeExtractStore{pending: []PendingPost{pendingPost()}}
	r := ExtractRunner{Extractor: ex, Store: store, Kinds: kinds()}

	stats, err := r.Run(context.Background())
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if stats.Jobs != 1 {
		t.Errorf("Jobs = %d, want 1 — only the vacancy actually written", stats.Jobs)
	}
	if stats.Skipped != 1 {
		t.Errorf("Skipped = %d, want 1 — the dropped vacancy has to be visible somewhere", stats.Skipped)
	}
	if stats.Processed != 1 || stats.Failed != 0 {
		t.Errorf("stats = %+v; a partly-malformed extraction is not a failed POST", stats)
	}
	if len(store.completed) != 1 || len(store.completed[0].jobs) != 1 {
		t.Fatalf("the store must receive only the good job, got %+v", store.completed)
	}
	if got := store.completed[0].jobs[0].Fields().Title; got != "ML Engineer" {
		t.Errorf("written job title = %q, want the well-formed one", got)
	}
}

// TestExtractRefusedLinkJobsAreSkippedNotWritten is the lie this change was really for. The
// link path has NO Validate — a ResolvedJob goes straight to the writer — so a resolver that
// returned a titleless vacancy had it dropped silently by the adapter while the runner counted
// it as written. jobs= was then simply wrong, not merely optimistic.
func TestExtractRefusedLinkJobsAreSkippedNotWritten(t *testing.T) {
	links := &fakeLinkResolver{jobs: []ResolvedJob{
		{Source: "habr_career", ExternalID: "1", URL: "https://career.habr.com/vacancies/1",
			Title: "Backend Engineer", Company: "Acme", Description: "Go and Postgres."},
		// No title: job.New refuses it, and nothing upstream would have caught it.
		{Source: "habr_career", ExternalID: "2", URL: "https://career.habr.com/vacancies/2",
			Company: "Acme", Description: "A link the parser could not read."},
	}}
	store := &fakeExtractStore{pending: []PendingPost{linkPost()}}
	r := ExtractRunner{Extractor: &fakeExtractor{}, Store: store, Kinds: kinds(), Links: links}

	stats, err := r.Run(context.Background())
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if stats.Jobs != 1 || stats.Skipped != 1 {
		t.Errorf("stats = %+v, want Jobs=1 Skipped=1", stats)
	}
	if len(store.linkCompleted) != 1 || len(store.linkCompleted[0].jobs) != 1 {
		t.Fatalf("the store must receive only the buildable link job, got %+v", store.linkCompleted)
	}
}

// The Telegram post's timestamp is the posting's source posted time: supplied rather than
// derived, and it must reach the built aggregate intact so the derived columns fingerprint the
// posted_at that is actually stored.
func TestDraftJobsCarriesThePostTimestamp(t *testing.T) {
	post := pendingPost()
	post.PostedAt = time.Date(2026, 3, 4, 5, 6, 7, 0, time.UTC)

	built, skipped := ExtractRunner{}.draftJobs(post, []ExtractedJob{
		{Title: "Senior Go Developer", Company: "Acme", Location: "Berlin",
			Description: "We use Golang and PostgreSQL."},
	})
	if skipped != 0 || len(built) != 1 {
		t.Fatalf("built %d jobs, skipped %d; want 1/0", len(built), skipped)
	}
	got := built[0].Fields()
	if got.PostedAt == nil || !got.PostedAt.Equal(post.PostedAt) {
		t.Errorf("PostedAt = %v, want %v", got.PostedAt, post.PostedAt)
	}
	if got.ExternalID != post.Channel+"/"+strconv.FormatInt(post.MsgID, 10)+"/0" {
		t.Errorf("ExternalID = %q; identity is channel/msg/index", got.ExternalID)
	}
}

// A resolved job that states its own posted time keeps it: the destination page knows better
// than the Telegram post that linked to it.
func TestDraftLinkJobsPrefersTheResolvedPostedTime(t *testing.T) {
	own := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	built, skipped := ExtractRunner{}.draftLinkJobs(pendingPost(), []ResolvedJob{
		{Source: "habr_career", ExternalID: "7", URL: "https://career.habr.com/vacancies/7",
			Title: "Backend Engineer", Company: "Acme", Description: "Go.", PostedAt: &own},
	})
	if skipped != 0 || len(built) != 1 {
		t.Fatalf("built %d jobs, skipped %d; want 1/0", len(built), skipped)
	}
	if got := built[0].Fields().PostedAt; got == nil || !got.Equal(own) {
		t.Errorf("PostedAt = %v, want the resolved job's own %v", got, own)
	}
}
