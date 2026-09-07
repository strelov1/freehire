package firecrawl

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

// vendor stands in for the scrape API. Everything here is exercised against it: no key, no
// network, no credits. A test that quietly spends money on every CI run would be a worse
// failure than no test at all.
type vendor struct {
	*httptest.Server
	calls atomic.Int64
}

// newVendor answers each request from reply, which the test supplies per case.
func newVendor(t *testing.T, reply func(w http.ResponseWriter, r *http.Request)) *vendor {
	t.Helper()
	v := &vendor{}
	v.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		v.calls.Add(1)
		reply(w, r)
	}))
	t.Cleanup(v.Close)
	return v
}

// scrapeOK is the vendor's success envelope: it answers 200 and reports the TARGET's own
// status inside.
func scrapeOK(targetStatus int, body string) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": true,
			"data": map[string]any{
				"rawHtml":  body,
				"metadata": map[string]any{"statusCode": targetStatus},
			},
		})
	}
}

func newTestClient(t *testing.T, v *vendor, budget int64) *Client {
	t.Helper()
	c, err := New(Config{APIKey: "test-key", BaseURL: v.URL, MaxPagesPerRun: budget})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return c
}

func TestFetchReturnsThePageBytes(t *testing.T) {
	v := newVendor(t, scrapeOK(http.StatusOK, "<html><body>hello</body></html>"))
	c := newTestClient(t, v, 10)

	status, body, err := c.Fetch(context.Background(), "https://example.test/page")
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if status != http.StatusOK {
		t.Errorf("status = %d, want 200", status)
	}
	if string(body) != "<html><body>hello</body></html>" {
		t.Errorf("body = %q", body)
	}
}

// The vendor answers 200 even when the TARGET refused. Reporting the vendor's status would
// turn every refusal into a success carrying a challenge page as if it were content.
func TestFetchReportsTheTargetStatusNotTheVendors(t *testing.T) {
	v := newVendor(t, scrapeOK(http.StatusNotFound, "not here"))
	c := newTestClient(t, v, 10)

	status, _, err := c.Fetch(context.Background(), "https://example.test/gone")
	if err != nil {
		t.Fatalf("Fetch returned an error for a target 404: %v", err)
	}
	if status != http.StatusNotFound {
		t.Errorf("status = %d, want the target's 404", status)
	}
}

// A vendor failure says nothing about the target and must not be mistaken for one — a caller
// that read it as a refusal could conclude a posting is gone when only the API was down.
func TestFetchDistinguishesAVendorFailure(t *testing.T) {
	v := newVendor(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"success":false,"error":"internal"}`))
	})
	c := newTestClient(t, v, 10)

	if _, _, err := c.Fetch(context.Background(), "https://example.test/x"); err == nil {
		t.Fatal("want an error when the vendor itself fails")
	}
}

// The budget is the whole point of this client existing rather than a bare HTTP call. Past it
// the request is NOT made — a budget that still spends the page it refuses would be no budget.
func TestBudgetStopsSpendingAndIssuesNoRequest(t *testing.T) {
	v := newVendor(t, scrapeOK(http.StatusOK, "ok"))
	c := newTestClient(t, v, 2)
	ctx := context.Background()

	for i := 0; i < 2; i++ {
		if _, _, err := c.Fetch(ctx, "https://example.test/p"); err != nil {
			t.Fatalf("Fetch %d within budget: %v", i, err)
		}
	}
	before := v.calls.Load()

	_, _, err := c.Fetch(ctx, "https://example.test/p")
	if err == nil {
		t.Fatal("want an error once the budget is spent")
	}
	if !errors.Is(err, ErrBudgetSpent) {
		t.Errorf("error is %v, want ErrBudgetSpent so a caller can tell it from a fetch failure", err)
	}
	if after := v.calls.Load(); after != before {
		t.Errorf("the refused fetch still issued %d request(s) — that spends the page it refuses", after-before)
	}
}

// The budget counts across every provider in a run, because what is being protected is the
// account and no single adapter can know what the others already spent.
func TestBudgetIsSharedAcrossCallers(t *testing.T) {
	v := newVendor(t, scrapeOK(http.StatusOK, "ok"))
	c := newTestClient(t, v, 3)
	ctx := context.Background()

	for i := 0; i < 3; i++ {
		if _, _, err := c.Fetch(ctx, "https://example.test/a"); err != nil {
			t.Fatalf("Fetch %d: %v", i, err)
		}
	}
	if _, _, err := c.Fetch(ctx, "https://other.test/b"); !errors.Is(err, ErrBudgetSpent) {
		t.Errorf("a different URL got a fresh budget: %v", err)
	}
}

func TestNewRefusesWithoutAKey(t *testing.T) {
	if _, err := New(Config{MaxPagesPerRun: 10}); err == nil {
		t.Error("want an error with no API key: a client that cannot authenticate can only fail per request")
	}
}

func TestNewRefusesANonPositiveBudget(t *testing.T) {
	for _, budget := range []int64{0, -1} {
		if _, err := New(Config{APIKey: "k", MaxPagesPerRun: budget}); err == nil {
			t.Errorf("want an error for budget %d: an unbounded metered client is the failure this exists to prevent", budget)
		}
	}
}

// The vendor publishes a per-minute request limit and answers 429 past it. Treating that as a
// fatal error is what starved the first live run against bayt: eight boards took their listing
// pages, the minute's allowance ran out, and every detail fetch after that failed — so the
// crawl reported a clean zero while the pages themselves were perfectly readable. A documented
// limit is something to obey, not something to break on.
func TestFetchRetriesTheVendorsRateLimit(t *testing.T) {
	var calls atomic.Int64
	v := newVendor(t, func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"success":false,"error":"Rate limit exceeded"}`))
			return
		}
		scrapeOK(http.StatusOK, "after the wait")(w, r)
	})
	c, err := New(Config{APIKey: "k", BaseURL: v.URL, MaxPagesPerRun: 5, RetryWait: time.Millisecond})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	status, body, err := c.Fetch(context.Background(), "https://example.test/p")
	if err != nil {
		t.Fatalf("Fetch gave up on a rate limit instead of waiting: %v", err)
	}
	if status != http.StatusOK || string(body) != "after the wait" {
		t.Errorf("status=%d body=%q, want the retried result", status, body)
	}
}

// A retry must not be charged twice. The budget protects the account, and a page fetched once
// after one wait is one page.
func TestARetriedFetchSpendsOnePage(t *testing.T) {
	var calls atomic.Int64
	v := newVendor(t, func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"success":false,"error":"Rate limit exceeded"}`))
			return
		}
		scrapeOK(http.StatusOK, "ok")(w, r)
	})
	c, err := New(Config{APIKey: "k", BaseURL: v.URL, MaxPagesPerRun: 1, RetryWait: time.Millisecond})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, _, err := c.Fetch(context.Background(), "https://example.test/p"); err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if got := c.Spent(); got != 1 {
		t.Errorf("Spent = %d after one retried fetch, want 1", got)
	}
}

// A rate limit that never lifts must still end, and must say what it was rather than looking
// like a page that is simply not there.
func TestFetchGivesUpOnAPersistentRateLimit(t *testing.T) {
	v := newVendor(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"success":false,"error":"Rate limit exceeded"}`))
	})
	c, err := New(Config{APIKey: "k", BaseURL: v.URL, MaxPagesPerRun: 5, RetryWait: time.Millisecond})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	_, _, err = c.Fetch(context.Background(), "https://example.test/p")
	if err == nil {
		t.Fatal("want an error when the rate limit never lifts")
	}
	if !errors.Is(err, ErrRateLimited) {
		t.Errorf("error is %v, want ErrRateLimited so a caller can tell it from a missing page", err)
	}
}
