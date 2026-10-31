package qualityaudit

import (
	"math/rand/v2"
	"net/url"
	"strings"
	"testing"
)

func mustURL(t *testing.T, raw string) *url.URL {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	return u
}

func TestReadStatus(t *testing.T) {
	const posting = "https://jobs.example.com/acme/jobs/12345"
	cases := []struct {
		name    string
		status  int
		final   string
		outcome Outcome
		reason  Reason
	}{
		{"not found", 404, posting, Dead, ""},
		{"gone", 410, posting, Dead, ""},
		{"gone after a redirect", 404, "https://jobs.example.com/missing", Dead, ""},
		{"live", 200, posting, Live, ""},
		{"live with a trailing slash", 200, posting + "/", Live, ""},
		{"live with a query added", 200, posting + "?src=x", Live, ""},
		{"host case folded", 200, "https://JOBS.example.com/acme/jobs/12345", Live, ""},
		{"redirected to the board", 200, "https://jobs.example.com/acme?error=true", Unmeasured, ReasonMoved},
		{"redirected to another host", 200, "https://other.example.com/acme/jobs/12345", Unmeasured, ReasonMoved},
		{"forbidden", 403, posting, Unmeasured, ReasonRefused},
		{"rate limited", 429, posting, Unmeasured, ReasonRefused},
		{"server error", 503, posting, Unmeasured, ReasonNoAnswer},
		{"transport failure", 0, posting, Unmeasured, ReasonNoAnswer},
		{"redirect not followed", 301, posting, Unmeasured, ReasonMoved},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			outcome, reason := readStatus(mustURL(t, posting), c.status, mustURL(t, c.final))
			if outcome != c.outcome || reason != c.reason {
				t.Errorf("readStatus(%d, %s) = %v %q, want %v %q", c.status, c.final, outcome, reason, c.outcome, c.reason)
			}
		})
	}
}

func TestCanaryURL(t *testing.T) {
	cases := []struct {
		name       string
		raw        string
		externalID string
		ok         bool
		keep       string // a part of the URL the canary must keep verbatim
	}{
		{"numeric path id", "https://boards.greenhouse.io/acme/jobs/7654321", "acme:7654321", true, "https://boards.greenhouse.io/acme/jobs/"},
		{"uuid", "https://jobs.lever.co/acme/0b5c7a2e-1f3d-4c8e-9a1b-2c3d4e5f6a7b", "acme:0b5c7a2e-1f3d-4c8e-9a1b-2c3d4e5f6a7b", true, "https://jobs.lever.co/acme/"},
		{"id before a title slug", "https://jobs.smartrecruiters.com/Acme/744000012345678-senior-engineer", "acme:744000012345678", true, "-senior-engineer"},
		{"id in the query", "https://acme.example.com/careers?gh_jid=7654321", "acme:7654321", true, "https://acme.example.com/careers?gh_jid="},
		{"id without a board", "https://example.com/vacancy/98765", "98765", true, "https://example.com/vacancy/"},
		{"id too short to be distinctive", "https://example.com/jobs/123", "acme:123", false, ""},
		{"id absent from the URL", "https://example.com/apply/redirect", "acme:7654321", false, ""},
		{"id only in the host", "https://7654321.example.com/", "acme:7654321", false, ""},
		{"unparseable URL", "://nope", "acme:7654321", false, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rng := rand.New(rand.NewPCG(1, 2))
			got, ok := canaryURL(c.raw, c.externalID, rng)
			if ok != c.ok {
				t.Fatalf("canaryURL(%q, %q) ok = %v, want %v (got %q)", c.raw, c.externalID, ok, c.ok, got)
			}
			if !ok {
				return
			}
			if got == c.raw {
				t.Fatalf("canary %q equals the posting URL", got)
			}
			if len(got) != len(c.raw) {
				t.Errorf("canary %q changed the id's shape (length %d, want %d)", got, len(got), len(c.raw))
			}
			if !strings.Contains(got, c.keep) {
				t.Errorf("canary %q lost %q", got, c.keep)
			}
		})
	}
}

func TestCanaryKeepsTheTokenShape(t *testing.T) {
	const id = "0b5c7a2e-1f3d-4c8e-9a1b-2c3d4e5f6a7b"
	for seed := uint64(0); seed < 50; seed++ {
		got, ok := canaryURL("https://jobs.lever.co/acme/"+id, "acme:"+id, rand.New(rand.NewPCG(seed, seed)))
		if !ok {
			t.Fatal("no canary")
		}
		token := strings.TrimPrefix(got, "https://jobs.lever.co/acme/")
		for i := range len(id) {
			want, have := class(id[i]), class(token[i])
			if want != have {
				t.Fatalf("seed %d: %q position %d is %q, want the same class as %q", seed, token, i, token[i], id[i])
			}
		}
	}
	// A numeric id never gains a leading zero, which some sites strip and resolve anyway.
	for seed := uint64(0); seed < 200; seed++ {
		got, _ := canaryURL("https://example.com/jobs/7654321", "7654321", rand.New(rand.NewPCG(seed, 0)))
		if strings.HasPrefix(strings.TrimPrefix(got, "https://example.com/jobs/"), "0") {
			t.Fatalf("seed %d: canary %q has a leading zero", seed, got)
		}
	}
}

func class(b byte) string {
	switch {
	case b >= '0' && b <= '9':
		return "digit"
	case b >= 'a' && b <= 'f':
		return "hex"
	case b >= 'A' && b <= 'F':
		return "HEX"
	case b >= 'a' && b <= 'z':
		return "lower"
	case b >= 'A' && b <= 'Z':
		return "upper"
	default:
		return string(b)
	}
}
