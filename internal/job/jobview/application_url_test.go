package jobview_test

import (
	"encoding/json"
	"testing"

	"github.com/strelov1/freehire/internal/job/job"
	"github.com/strelov1/freehire/internal/job/jobview"
	"github.com/strelov1/freehire/internal/platform/db"
)

func TestApplicationDestinationPreservesSourcePosting(t *testing.T) {
	const source = "https://www.linkedin.com/jobs/view/4472812273/"
	const target = "https://gistworld.mua.hrdepartment.com/hrsmart/ats/Posting/view/3735?src=LinkedIn"
	for _, destination := range []string{"", target} {
		t.Run(destination, func(t *testing.T) {
			// Read the persisted shape independently of the Go field mapping.
			payload, _ := json.Marshal(map[string]string{"source": "linkedin", "url": source, "application_url": destination, "title": "Senior Java Software Engineer", "public_slug": "senior-java-1"})
			var row db.Job
			if err := json.Unmarshal(payload, &row); err != nil {
				t.Fatal(err)
			}
			domain, extras, err := job.FromRow(row)
			if err != nil {
				t.Fatal(err)
			}
			got, err := jobview.FromDomain(domain, extras)
			if err != nil {
				t.Fatal(err)
			}
			want := source
			if destination != "" {
				want = target
			}
			if got.URL != want || got.Source != "linkedin" || domain.Fields().URL != source {
				t.Fatalf("wire URL=%q source=%q stored URL=%q; want %q, linkedin, %q", got.URL, got.Source, domain.Fields().URL, want, source)
			}
			if got.ApplyURL != "https://freehire.me/api/v1/go/senior-java-1?src=web" || jobview.RowDestination(row) != want {
				t.Fatalf("apply_url=%q destination=%q; want the redirect and %q", got.ApplyURL, jobview.RowDestination(row), want)
			}
			if destination != "" && got.SourceURL != source+"?utm_source=freehire.me" {
				t.Fatalf("original posting = %q", got.SourceURL)
			}
		})
	}
}

func TestDirectApplyHasNoApplyURL(t *testing.T) {
	row := db.Job{Source: job.EmployerSource, ExternalID: job.DirectApplyPrefix + "1", URL: "https://freehire.me/jobs/x", PublicSlug: "x"}
	domain, extras, err := job.FromRow(row)
	if err != nil {
		t.Fatal(err)
	}
	got, err := jobview.FromDomain(domain, extras)
	if err != nil {
		t.Fatal(err)
	}
	if got.URL != "" || got.ApplyURL != "" || jobview.RowDestination(row) != "" {
		t.Fatalf("url=%q apply_url=%q: a freehire-only vacancy has nowhere to redirect", got.URL, got.ApplyURL)
	}
}

func TestPrivateJobKeepsDirectLink(t *testing.T) {
	row := db.Job{Source: "greenhouse", ExternalID: "1", URL: "https://acme.test/1", PublicSlug: "p", IsPrivate: true}
	domain, extras, err := job.FromRow(row)
	if err != nil {
		t.Fatal(err)
	}
	got, err := jobview.FromDomain(domain, extras)
	if err != nil {
		t.Fatal(err)
	}
	if got.ApplyURL != "" || got.URL != "https://acme.test/1" {
		t.Fatalf("url=%q apply_url=%q: the redirect refuses private jobs, so the owner keeps the direct link", got.URL, got.ApplyURL)
	}
}
