package contribution

import (
	"bytes"
	"context"
	"errors"
	"log"
	"os"
	"strings"
	"testing"
)

// fakeRepo is an in-memory Repository for the service branch tests. Each behaviour is a
// tunable field so a test sets only what it exercises.
type fakeRepo struct {
	boardTracked  bool
	recordErr     error
	recorded      RecordInput
	recordCalls   int
	reviewErr     error
	reviewedURL   string
	reviewCalls   int
	listByUserRet []Contribution
	companyName   string
	companySlug   string
	jobIDBoard    string // BoardByGreenhouseJobID returns this (ok when non-empty)
	ashbyIDBoard  string // BoardByAshbyJobID returns this (ok when non-empty)
}

func (f *fakeRepo) BoardTracked(_ context.Context, _, _ string) (bool, error) {
	return f.boardTracked, nil
}

func (f *fakeRepo) CompanyForBoard(_ context.Context, _, _ string) (string, string, bool, error) {
	return f.companyName, f.companySlug, f.companyName != "" || f.companySlug != "", nil
}

func (f *fakeRepo) BoardByGreenhouseJobID(_ context.Context, _ string) (string, bool, error) {
	return f.jobIDBoard, f.jobIDBoard != "", nil
}

func (f *fakeRepo) BoardByAshbyJobID(_ context.Context, _ string) (string, bool, error) {
	return f.ashbyIDBoard, f.ashbyIDBoard != "", nil
}

func (f *fakeRepo) Record(_ context.Context, in RecordInput) (Contribution, error) {
	f.recordCalls++
	f.recorded = in
	if f.recordErr != nil {
		return Contribution{}, f.recordErr
	}
	return Contribution{
		ID: 1, SubmittedBy: in.SubmittedBy, URL: in.URL,
		Source: in.Source, Board: in.Board, Status: "pending",
	}, nil
}

func (f *fakeRepo) RecordReview(_ context.Context, submittedBy int64, url string) (Contribution, error) {
	f.reviewCalls++
	f.reviewedURL = url
	if f.reviewErr != nil {
		return Contribution{}, f.reviewErr
	}
	return Contribution{ID: 2, SubmittedBy: submittedBy, URL: url, Status: StatusReview}, nil
}

func (f *fakeRepo) ListByUser(_ context.Context, _ int64) ([]Contribution, error) {
	return f.listByUserRet, nil
}

// fakeResolver stands in for the network fallback.
type fakeResolver struct {
	source, board, canonical string
	ok                       bool
	calls                    int
}

func (r *fakeResolver) Resolve(_ context.Context, _ string) (string, string, string, bool) {
	r.calls++
	return r.source, r.board, r.canonical, r.ok
}

// newService wires the service with a network-free recognizer only (nil resolver).
func newService(repo Repository) *Service {
	return New(repo, nil)
}

func TestSubmitRecordsUnknownHostForReview(t *testing.T) {
	// An unknown host yields no board, but the link is a valid URL — record it for manual
	// review (no board record, no credit) rather than rejecting it.
	repo := &fakeRepo{}
	got, source, board, err := newService(repo).Submit(context.Background(), 7, "https://example.com/careers/123")
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}
	if got.Status != StatusReview || source != "" || board != "" {
		t.Errorf("got = (status %q, %q, %q), want (review, empty, empty)", got.Status, source, board)
	}
	if repo.reviewCalls != 1 || repo.recordCalls != 0 {
		t.Errorf("reviewCalls=%d recordCalls=%d, want 1 and 0", repo.reviewCalls, repo.recordCalls)
	}
	if repo.reviewedURL != "https://example.com/careers/123" {
		t.Errorf("reviewed URL = %q, want the pasted link", repo.reviewedURL)
	}
}

func TestSubmitRecordsSingleTenantSourceForReview(t *testing.T) {
	// geekjob is a single-tenant aggregator — no per-company board, but a valid URL, so it
	// enters the review queue for a maintainer to judge.
	repo := &fakeRepo{}
	got, _, _, err := newService(repo).Submit(context.Background(), 7, "https://geekjob.ru/vacancy/6a1ebb85")
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}
	if got.Status != StatusReview || repo.reviewCalls != 1 {
		t.Errorf("got status %q, reviewCalls %d, want review and 1", got.Status, repo.reviewCalls)
	}
}

func TestSubmitRejectsNonURL(t *testing.T) {
	repo := &fakeRepo{}
	_, _, _, err := newService(repo).Submit(context.Background(), 7, "not a url")
	if !errors.Is(err, ErrUnsupportedATS) {
		t.Fatalf("err = %v, want ErrUnsupportedATS", err)
	}
	if repo.reviewCalls != 0 || repo.recordCalls != 0 {
		t.Errorf("wrote something (review=%d record=%d), want nothing for non-URL garbage", repo.reviewCalls, repo.recordCalls)
	}
}

func TestSubmitDeduplicatesReviewLink(t *testing.T) {
	// A url already in the review queue surfaces the unique violation as the same
	// "already contributed" outcome as a duplicate board.
	repo := &fakeRepo{reviewErr: ErrBoardAlreadyContributed}
	_, _, _, err := newService(repo).Submit(context.Background(), 7, "https://example.com/careers/123")
	if !errors.Is(err, ErrBoardAlreadyContributed) {
		t.Fatalf("err = %v, want ErrBoardAlreadyContributed", err)
	}
}

func TestSubmitRejectsWhenBoardAlreadyTracked(t *testing.T) {
	repo := &fakeRepo{boardTracked: true}
	_, source, board, err := newService(repo).Submit(context.Background(), 7, "https://jobs.ashbyhq.com/blitzy/a741b4e8-8799-4539-b1c2-78d69ff625e7")
	if !errors.Is(err, ErrBoardAlreadyTracked) {
		t.Fatalf("err = %v, want ErrBoardAlreadyTracked", err)
	}
	// Submit returns the resolved identity even on the tracked path, so the caller can look up
	// the company without re-recognizing.
	if source != "ashby" || board != "blitzy" {
		t.Errorf("returned (%q,%q), want (ashby, blitzy)", source, board)
	}
	if repo.recordCalls != 0 {
		t.Errorf("recorded %d times, want 0", repo.recordCalls)
	}
}

func TestSubmitRejectsDuplicateBoard(t *testing.T) {
	repo := &fakeRepo{recordErr: ErrBoardAlreadyContributed}
	_, _, _, err := newService(repo).Submit(context.Background(), 7, "https://jobs.ashbyhq.com/blitzy")
	if !errors.Is(err, ErrBoardAlreadyContributed) {
		t.Fatalf("err = %v, want ErrBoardAlreadyContributed", err)
	}
}

func TestSubmitRecordsBoardFromVacancyURL(t *testing.T) {
	repo := &fakeRepo{}
	got, source, board, err := newService(repo).Submit(context.Background(), 7, "https://jobs.ashbyhq.com/blitzy/a741b4e8-8799-4539-b1c2-78d69ff625e7?utm=x")
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}
	if source != "ashby" || board != "blitzy" || repo.recorded.Source != "ashby" || repo.recorded.Board != "blitzy" {
		t.Errorf("recorded = (%q,%q), want (ashby, blitzy)", repo.recorded.Source, repo.recorded.Board)
	}
	if repo.recorded.URL != "https://jobs.ashbyhq.com/blitzy/a741b4e8-8799-4539-b1c2-78d69ff625e7" {
		t.Errorf("stored URL = %q, want canonicalized", repo.recorded.URL)
	}
	if got.SubmittedBy != 7 {
		t.Errorf("SubmittedBy = %d, want 7", got.SubmittedBy)
	}
}

func TestSubmitUsesResolverForUnknownHost(t *testing.T) {
	// A vanity careers page (unknown host): recognizeBoard fails, the resolver detects the
	// embedded board.
	repo := &fakeRepo{}
	res := &fakeResolver{source: "greenhouse", board: "talkspace", canonical: "https://www.talkspace.com/careers/job", ok: true}
	_, source, board, err := New(repo, res).Submit(context.Background(), 7, "https://www.talkspace.com/careers/job?gh_jid=123")
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}
	if res.calls != 1 {
		t.Errorf("resolver called %d times, want 1", res.calls)
	}
	if source != "greenhouse" || board != "talkspace" || repo.recorded.Board != "talkspace" {
		t.Errorf("recorded = (%q,%q), want (greenhouse, talkspace)", repo.recorded.Source, repo.recorded.Board)
	}
	if repo.recorded.URL != "https://www.talkspace.com/careers/job" {
		t.Errorf("stored URL = %q, want the resolver's canonical", repo.recorded.URL)
	}
}

func TestSubmitSkipsResolverForRecognizedHost(t *testing.T) {
	// A known ATS host resolves network-free; the resolver must not be called.
	repo := &fakeRepo{}
	res := &fakeResolver{ok: true, source: "x", board: "y"}
	_, _, _, err := New(repo, res).Submit(context.Background(), 7, "https://jobs.ashbyhq.com/blitzy")
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}
	if res.calls != 0 {
		t.Errorf("resolver called %d times for a known host, want 0", res.calls)
	}
	if repo.recorded.Source != "ashby" {
		t.Errorf("recorded source = %q, want ashby (network-free)", repo.recorded.Source)
	}
}

func TestSubmitResolvesGreenhouseJobIDForServerSideVanity(t *testing.T) {
	// A company careers page that exposes only the Greenhouse job id (gh_jid or a trailing
	// numeric path segment) — no host or embed. The job-id lookup finds the tracked board.
	repo := &fakeRepo{jobIDBoard: "sumup", boardTracked: true}
	cases := []string{
		"https://www.sumup.com/careers/positions/x/8578073002/?city=Brazil",
		"https://www.talkspace.com/careers/job?gh_jid=6118228004",
	}
	for _, raw := range cases {
		_, source, board, err := New(repo, nil).Submit(context.Background(), 7, raw)
		if !errors.Is(err, ErrBoardAlreadyTracked) {
			t.Errorf("Submit(%q) err = %v, want ErrBoardAlreadyTracked", raw, err)
		}
		if source != "greenhouse" || board != "sumup" {
			t.Errorf("Submit(%q) = (%q,%q), want (greenhouse, sumup)", raw, source, board)
		}
	}
	// No id in the URL → not resolved to a board, so it enters the review queue.
	if got, _, _, err := New(repo, nil).Submit(context.Background(), 7, "https://example.com/careers/about"); err != nil || got.Status != StatusReview {
		t.Errorf("got = (status %q, err %v), want (review, nil) for a URL with no job id", got.Status, err)
	}
}

func TestSubmitResolvesAshbyJobIDForEmbeddedVanity(t *testing.T) {
	// A company careers page that embeds Ashby on its own domain via the ashby_jid widget param
	// (company.com/careers?ashby_jid=<uuid>). The slug is never in the URL/markup, but external_id
	// is "<board>:<uuid>", so the id lookup finds the tracked board.
	repo := &fakeRepo{ashbyIDBoard: "valon", boardTracked: true}
	_, source, board, err := New(repo, nil).Submit(context.Background(), 7,
		"https://www.valon.ai/about?ashby_jid=6052f210-29f1-4ef4-93cc-48029969eaf7&utm_source=x#careers")
	if !errors.Is(err, ErrBoardAlreadyTracked) {
		t.Fatalf("err = %v, want ErrBoardAlreadyTracked", err)
	}
	if source != "ashby" || board != "valon" {
		t.Errorf("= (%q,%q), want (ashby, valon)", source, board)
	}
	// A non-UUID ashby_jid (or none) → not resolved to a board, so it enters the review queue.
	if got, _, _, err := New(repo, nil).Submit(context.Background(), 7, "https://www.valon.ai/about?ashby_jid=not-a-uuid"); err != nil || got.Status != StatusReview {
		t.Errorf("got = (status %q, err %v), want (review, nil) for a non-UUID ashby_jid", got.Status, err)
	}
}

func TestCompanyForBoard(t *testing.T) {
	repo := &fakeRepo{companyName: "Acme Corp", companySlug: "acme"}
	name, slug, ok := newService(repo).CompanyForBoard(context.Background(), "greenhouse", "acme")
	if !ok || name != "Acme Corp" || slug != "acme" {
		t.Errorf("CompanyForBoard = (%q,%q,%v), want (Acme Corp, acme, true)", name, slug, ok)
	}
	// No company on the board → not ok.
	if _, _, ok := newService(&fakeRepo{}).CompanyForBoard(context.Background(), "greenhouse", "empty"); ok {
		t.Error("CompanyForBoard(no company) ok = true, want false")
	}
}

func TestSubmitLogsUnrecognizedURLsOnly(t *testing.T) {
	var buf bytes.Buffer
	log.SetOutput(&buf)
	defer log.SetOutput(os.Stderr)

	svc := New(&fakeRepo{}, nil)
	// A valid http(s) URL that resolves to nothing is logged for review.
	_, _, _, _ = svc.Submit(context.Background(), 42, "https://acme.io/careers/some-role")
	if !strings.Contains(buf.String(), "unrecognized link (user=42): https://acme.io/careers/some-role") {
		t.Errorf("log = %q, want the unrecognized http link logged", buf.String())
	}
	// Garbage (not a URL) is not logged — the feed stays signal.
	buf.Reset()
	_, _, _, _ = svc.Submit(context.Background(), 42, "not a url")
	if buf.Len() != 0 {
		t.Errorf("log = %q, want nothing for non-URL garbage", buf.String())
	}
}

func TestListMineReturnsRepoRows(t *testing.T) {
	repo := &fakeRepo{listByUserRet: []Contribution{{ID: 3}, {ID: 2}}}
	got, err := newService(repo).ListMine(context.Background(), 7)
	if err != nil {
		t.Fatalf("ListMine: %v", err)
	}
	if len(got) != 2 || got[0].ID != 3 {
		t.Errorf("ListMine = %+v, want the repo rows in order", got)
	}
}
