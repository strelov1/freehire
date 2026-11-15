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
			payload, _ := json.Marshal(map[string]string{"source": "linkedin", "url": source, "application_url": destination, "title": "Senior Java Software Engineer"})
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
			want := source + "?utm_source=freehire.me"
			if destination != "" {
				want = target + "&utm_source=freehire.me"
			}
			if got.URL != want || got.Source != "linkedin" || domain.Fields().URL != source {
				t.Fatalf("wire URL=%q source=%q stored URL=%q; want %q, linkedin, %q", got.URL, got.Source, domain.Fields().URL, want, source)
			}
			if destination != "" && got.SourceURL != source+"?utm_source=freehire.me" {
				t.Fatalf("original posting = %q", got.SourceURL)
			}
		})
	}
}
