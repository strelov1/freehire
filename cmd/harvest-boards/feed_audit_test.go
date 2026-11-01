package main

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/strelov1/freehire/internal/ingest/sources"
	"golang.org/x/net/html"
)

func TestCompareFeedAuditReportsLossesOnMatchedJobs(t *testing.T) {
	date := time.Date(2026, 10, 31, 0, 0, 0, 0, time.UTC)
	baseline, err := adapterObservation([]sources.Job{
		{ExternalID: "1", Title: "Engineer", Description: `<p>Build &amp; ship</p>`, PostedAt: &date},
		{ExternalID: "2", Title: "Engineer", Description: "Long detailed responsibilities"},
		{ExternalID: "3", Title: "Removed"},
	})
	if err != nil {
		t.Fatal(err)
	}
	feed := feedObservation{Rows: 4, Jobs: map[string]auditPosting{
		"1": {ID: "1", Title: "Engineer", Body: "Build & ship", Fields: map[string]bool{}},
		"2": {ID: "2", Title: "Engineer", Body: "Summary", Fields: map[string]bool{}},
		"4": {ID: "4", Title: "Extra", Fields: map[string]bool{}},
	}}
	got := compareFeedAudit(feed, baseline)
	for name, check := range map[string]struct {
		got  []string
		want string
	}{
		"missing": {got: got.MissingIDs, want: "3"},
		"extra":   {got: got.ExtraIDs, want: "4"},
		"body":    {got: got.BodyDifferences, want: "2"},
	} {
		if strings.Join(check.got, ",") != check.want {
			t.Fatalf("%s: got %v, want %s", name, check.got, check.want)
		}
	}
	if got.Matched != 2 || got.MissingFields["posted_at"] != 1 {
		t.Fatalf("comparison = %+v", got)
	}
	if got.Feed.Fields["description"] != 2 || got.Adapter.Fields["description"] != 2 {
		t.Fatalf("description counts: feed %v, adapter %v", got.Feed.Fields, got.Adapter.Fields)
	}
}

func TestAdapterObservationRejectsUnusableIdentity(t *testing.T) {
	for _, jobs := range [][]sources.Job{
		{{Title: "No id"}},
		{{ExternalID: "1"}, {ExternalID: "1"}},
	} {
		if _, err := adapterObservation(jobs); err == nil {
			t.Fatal("accepted unverified identities")
		}
	}
}

type auditFailingClient struct{ httpClient }

func (auditFailingClient) GetHTML(context.Context, string) (*html.Node, error) {
	return nil, errors.New("detail failed")
}

type auditSkippingSource struct{}

func (auditSkippingSource) Provider() string { return "test" }
func (auditSkippingSource) Fetch(context.Context, sources.CompanyEntry) ([]sources.Job, error) {
	return []sources.Job{{ExternalID: "1", Title: "Engineer"}}, nil
}

func TestAuditBaselineDetectsSwallowedRequestFailure(t *testing.T) {
	c := newAuditClient(auditFailingClient{})
	// A detail adapter can discard this error and still return other jobs.
	_, _ = c.GetHTML(context.Background(), "https://jobs.acme.com/jobs/2")
	_, err := readAuditBaseline(context.Background(), c, auditSkippingSource{}, sources.CompanyEntry{Board: "acme"})
	if err == nil {
		t.Fatal("accepted a partial baseline after an observed transport failure")
	}
}

func TestAuditBodyPreservesInlineWords(t *testing.T) {
	if got := auditBody(`<p>Type<strong>Script</strong> &amp; Go</p><p>Next role</p>`); got != "TypeScript & Go Next role" {
		t.Fatal(got)
	}
}
