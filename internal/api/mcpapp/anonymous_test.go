package mcpapp

import (
	"context"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestAnonymousCapsListTools(t *testing.T) {
	for name, tc := range map[string]struct {
		userID    int64
		wantLimit int
		wantNote  bool
	}{
		"anonymous": {0, anonymousMaxResults, true},
		"signed in": {42, 50, false},
	} {
		t.Run(name, func(t *testing.T) {
			var searchLimit, searchOffset, similarLimit int
			r := readerThatFinds(t)
			r.searchJobs = func(_ context.Context, in SearchInput) (SearchJobsResult, error) {
				searchLimit, searchOffset = in.Page()
				return SearchJobsResult{Offset: searchOffset, Total: 100}, nil
			}
			r.similarJobs = func(_ context.Context, _ string, limit int) (SimilarJobsResult, error) {
				similarLimit = limit
				return SimilarJobsResult{}, nil
			}
			ctx := context.Background()
			ct, st := mcp.NewInMemoryTransports()
			ss, err := newMixedServer(nil, tc.userID, testOrigin, r, false).Connect(ctx, st, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = ss.Close() }()
			session, err := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "0"}, nil).Connect(ctx, ct, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = session.Close() }()

			var found SearchJobsResult
			callJSON(t, session, toolSearchJobs, map[string]any{"query": "go", "limit": 50, "offset": 10}, &found)
			if searchOffset != 10 || found.Offset != 10 || found.Total != 100 {
				t.Errorf("paging changed: offset=%d answered=%d total=%d", searchOffset, found.Offset, found.Total)
			}
			if searchLimit != tc.wantLimit || (found.Notice != "") != tc.wantNote {
				t.Errorf("search_jobs asked %d notice=%q, want %d note=%v", searchLimit, found.Notice, tc.wantLimit, tc.wantNote)
			}

			var similar SimilarJobsResult
			callJSON(t, session, toolSimilarJobs, map[string]any{"slug": "x", "limit": 20}, &similar)
			wantSimilar := min(tc.wantLimit, maxSimilar)
			if similarLimit != wantSimilar || (similar.Notice != "") != tc.wantNote {
				t.Errorf("similar_jobs asked %d notice=%q, want %d note=%v", similarLimit, similar.Notice, wantSimilar, tc.wantNote)
			}
		})
	}
}

func TestAnonymousWithinCapGetsNoNotice(t *testing.T) {
	r := readerThatFinds(t)
	r.searchJobs = func(context.Context, SearchInput) (SearchJobsResult, error) { return SearchJobsResult{}, nil }
	got, err := forAnonymous(r).SearchJobs(context.Background(), SearchInput{Query: "go"})
	if err != nil || got.Notice != "" {
		t.Errorf("default page: notice=%q err=%v, want none", got.Notice, err)
	}
}
