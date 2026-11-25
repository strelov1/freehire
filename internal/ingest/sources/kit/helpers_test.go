package kit

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestCountryFromCode(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want []string
	}{
		{"alpha-2 lowercase", "ae", []string{"ae"}},
		{"alpha-2 uppercase", "AE", []string{"ae"}},
		{"alpha-3 as Ashby sends it", "USA", []string{"us"}},
		{"empty", "", nil},
		{"unresolved code outside the curated set", "ZZZ", nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := CountryFromCode(tc.in); !reflect.DeepEqual(got, tc.want) {
				t.Errorf("CountryFromCode(%q) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}

// A detail request that failed tells the crawl one of two very different things, and the whole
// stale-job sweep hangs off which: 404/410 is the platform stating the posting is gone, and
// anything else is this crawl having failed to see a posting that may well still be there.
func TestDetailUnreadable(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"the platform reports the posting gone", &StatusError{Code: 404}, false},
		{"the platform reports the posting withdrawn", &StatusError{Code: 410}, false},
		{"the origin refuses this address", &StatusError{Code: 403}, true},
		{"rate limited past the client's own retries", &StatusError{Code: 429}, true},
		{"the origin is broken", &StatusError{Code: 502}, true},
		{"an unexpected status says nothing about the posting", &StatusError{Code: 401}, true},
		{"a transport failure", errors.New("dial tcp: i/o timeout"), true},
		{"a status wrapped by an adapter", fmt.Errorf("detail: %w", &StatusError{Code: 404}), false},
		{"a decode failure", fmt.Errorf("sources: decode %s: %w", "https://x/y", errors.New("EOF")), true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := DetailUnreadable(tc.err); got != tc.want {
				t.Errorf("DetailUnreadable(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}

func TestCollapseSpace(t *testing.T) {
	if got := CollapseSpace("  Senior \n\t Engineer  "); got != "Senior Engineer" {
		t.Errorf("CollapseSpace = %q", got)
	}
}

func TestSanitizeHTMLAndErrListingAbsent(t *testing.T) {
	if got := SanitizeHTML(`<p>Role</p><script>x()</script>`); got != "<p>Role</p>" {
		t.Errorf("SanitizeHTML = %q", got)
	}
	if err := ErrListingAbsent("greenhouse", "acme", "jobs"); err == nil || err.Error() != `greenhouse: board acme: the listing response carries no "jobs" collection` {
		t.Errorf("ErrListingAbsent = %v", err)
	}
}

type hydratePosting struct {
	id, company string
}

func hydrateBase(p hydratePosting) Job { return Job{ExternalID: p.id, Company: p.company} }

// The shared walk's contract: a stored posting and a covered one are yielded body-less, the
// budget is spent on the rest in listing order, and a page nothing answered fails the crawl
// while pages left for want of time do not.
func TestHydrateNew(t *testing.T) {
	h := Hydration{Provider: "ex", Noun: "posting", Plural: "postings", Unit: "posting pages", MaxNewPerRun: 2, Workers: 1, Deadline: time.Minute}
	postings := []hydratePosting{{"1", "Seen"}, {"2", "Covered"}, {"3", "New"}, {"4", "New"}, {"5", "Over budget"}}
	var asked []string
	gate := DetailGate{Covered: func(companies []string) map[string]bool {
		asked = companies
		return map[string]bool{"Covered": true}
	}}
	var read []string
	jobs, err := HydrateNew(context.Background(), h, postings, func(id string) bool { return id == "1" }, gate,
		nil, hydrateBase, func(p hydratePosting) any { return p.id },
		func(_ context.Context, p hydratePosting) (Job, bool, error) {
			read = append(read, p.id)
			j := hydrateBase(p)
			j.Description = "body"
			return j, true, nil
		})
	if err != nil {
		t.Fatalf("HydrateNew: %v", err)
	}
	if strings.Join(read, ",") != "3,4" {
		t.Errorf("read = %v, want only the two new postings inside the budget", read)
	}
	if len(asked) != 5 || asked[0] != "Seen" {
		t.Errorf("gate asked about %v, want every listed company in listing order", asked)
	}
	if len(jobs) != 4 || !jobs[0].SeenRefresh || jobs[1].SeenRefresh || jobs[0].Description != "" || jobs[1].Description != "" || jobs[2].Description != "body" || jobs[3].Description != "body" {
		t.Fatalf("jobs = %+v, want [seen, covered, 3, 4]: seen refreshes, covered stays body-less, the two read ones carry a body", jobs)
	}
}

// A zero gate condemns nothing, and a page the detail could not read is dropped, not yielded.
func TestHydrateNewNoGate(t *testing.T) {
	h := Hydration{Provider: "ex", Noun: "posting", Plural: "postings", Unit: "posting pages", MaxNewPerRun: 10, Workers: 1, Deadline: 0}
	jobs, err := HydrateNew(context.Background(), h, []hydratePosting{{"1", ""}}, func(string) bool { return false }, DetailGate{},
		nil, hydrateBase, func(p hydratePosting) any { return p.id },
		func(context.Context, hydratePosting) (Job, bool, error) { return Job{}, false, nil })
	if err != nil || len(jobs) != 0 {
		t.Fatalf("jobs = %v, err = %v; want a page the site reported gone to be dropped quietly", jobs, err)
	}
}

// A crawl that asked for a page and got no answer is broken; one that ran out of time before
// asking is merely deferred and must not fail.
func TestHydrateNewNoneAnsweredVersusOutOfTime(t *testing.T) {
	h := Hydration{Provider: "ex", Noun: "posting", Plural: "postings", Unit: "posting pages", MaxNewPerRun: 10, Workers: 1, Deadline: 0}
	detail := func(context.Context, hydratePosting) (Job, bool, error) { return Job{}, false, errors.New("timeout") }
	_, err := HydrateNew(context.Background(), h, []hydratePosting{{"1", "New"}}, func(string) bool { return false }, DetailGate{},
		nil, hydrateBase, func(p hydratePosting) any { return p.id }, detail)
	if err == nil || err.Error() != "ex: requested 1 posting pages and none answered" {
		t.Fatalf("err = %v, want the none-answered failure with the adapter's own wording", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Hour)
	defer cancel()
	h.Deadline = 2 * time.Hour // the deadline is inside the margin from the first request
	read := false
	jobs, err := HydrateNew(ctx, h, []hydratePosting{{"1", "New"}}, func(string) bool { return false }, DetailGate{},
		nil, hydrateBase, func(p hydratePosting) any { return p.id },
		func(context.Context, hydratePosting) (Job, bool, error) { read = true; return Job{}, false, nil })
	if err != nil || read || len(jobs) != 0 {
		t.Fatalf("jobs = %v, err = %v, read = %v; want the page left for a later run, not requested and not a failure", jobs, err, read)
	}
}

// The budget stops the BUFFER at MaxNewPerRun when the listing's order stands, and the walk keeps
// scanning for seen and covered postings past it; an ordered walk buffers every candidate so the
// sort can pick from all of them.
func TestHydrateNewBudgetBoundsTheBufferButNotTheScan(t *testing.T) {
	h := Hydration{Provider: "ex", Noun: "posting", Plural: "postings", Unit: "posting pages", MaxNewPerRun: 1, Workers: 1}
	var read []string
	postings := []hydratePosting{{"1", "New"}, {"2", "New"}, {"3", "Seen later"}, {"4", "Covered later"}}
	gate := DetailGate{Covered: func([]string) map[string]bool { return map[string]bool{"Covered later": true} }}
	jobs, err := HydrateNew(context.Background(), h, postings, func(id string) bool { return id == "3" }, gate, nil,
		hydrateBase, func(p hydratePosting) any { return p.id },
		func(_ context.Context, p hydratePosting) (Job, bool, error) {
			read = append(read, p.id)
			return hydrateBase(p), true, nil
		})
	if err != nil || strings.Join(read, ",") != "1" {
		t.Fatalf("read = %v, err = %v; want only the first new posting", read, err)
	}
	// Result order is the kept postings in listing order, then the read ones.
	if len(jobs) != 3 || jobs[0].ExternalID != "3" || !jobs[0].SeenRefresh || jobs[1].ExternalID != "4" || jobs[1].SeenRefresh || jobs[2].ExternalID != "1" {
		t.Fatalf("jobs = %v; want [seen 3, covered 4, read 1]: the postings past the budget are still yielded", jobs)
	}
}

// order is the adapter's say in which unseen pages the budget buys first: the listing's order
// stands without one, and the budget is taken from the head of the ordered slice.
func TestHydrateNewOrderSpendsTheBudgetOnTheHead(t *testing.T) {
	h := Hydration{Provider: "ex", Noun: "posting", Plural: "postings", Unit: "posting pages", MaxNewPerRun: 2, Workers: 1}
	var read []string
	oldest := func(unseen []hydratePosting) {
		slices.SortFunc(unseen, func(a, b hydratePosting) int { return strings.Compare(a.id, b.id) })
	}
	_, err := HydrateNew(context.Background(), h, []hydratePosting{{"3", ""}, {"1", ""}, {"2", ""}}, func(string) bool { return false }, DetailGate{}, oldest,
		hydrateBase, func(p hydratePosting) any { return p.id },
		func(_ context.Context, p hydratePosting) (Job, bool, error) {
			read = append(read, p.id)
			return hydrateBase(p), true, nil
		})
	if err != nil || strings.Join(read, ",") != "1,2" {
		t.Fatalf("read = %v, err = %v; want the two oldest after ordering, in that order", read, err)
	}
}
