package boardresolve

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/strelov1/freehire/internal/ingest/sources"
)

type appOneFixture struct {
	fakeFetcher
	t      *testing.T
	detail string
}

func (f appOneFixture) GetJSON(_ context.Context, url string, dst any) error {
	f.t.Helper()
	if url != "https://apply.appone.com/api/apply/v2/jobposting/6aff3ddbe355afbbc661d2e3" {
		f.t.Fatalf("unexpected posting endpoint %s", url)
	}
	return json.Unmarshal([]byte(f.detail), dst)
}

func TestResolveAppOnePostingPortal(t *testing.T) {
	const raw = "https://apply.appone.com/job/6aff3ddbe355afbbc661d2e3"
	for name, tc := range map[string]struct{ detail, raw, want string }{
		"verified portal":    {`{"jobPostId":"6aff3ddbe355afbbc661d2e3","jobPortalUrl":"https://jobs.appone.com/ashley-hall"}`, raw, "ashley-hall"},
		"wrong posting":      {`{"jobPostId":"another","jobPortalUrl":"https://jobs.appone.com/ashley-hall"}`, raw, ""},
		"unrelated provider": {`{"jobPostId":"6aff3ddbe355afbbc661d2e3","jobPortalUrl":"https://jobs.lever.co/acme"}`, raw, ""},
		"lookalike host":     {"", "https://apply.appone.com.example.org/job/6aff3ddbe355afbbc661d2e3", ""},
	} {
		t.Run(name, func(t *testing.T) {
			fixture := appOneFixture{t: t, detail: tc.detail}
			r := &Resolver{http: fixture, appOnePortal: sources.NewAppOnePortalResolver(fixture)}
			source, board, _, ok := r.Resolve(context.Background(), tc.raw)
			if ok != (tc.want != "") || board != tc.want || (ok && source != "appone") {
				t.Fatalf("got %s %s %v, want %s", source, board, ok, tc.want)
			}
		})
	}
}
