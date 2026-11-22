package main

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/strelov1/freehire/internal/ingest/sources"
)

func TestLoadQueriesRequiresIntegerScalarsForLimits(t *testing.T) {
	for _, field := range []string{"pages", "jobage"} {
		for _, value := range []string{"1.5", "0.5", "-0.5", "1.0", "1e0", `"2"`, `""`, "true", "null", "[]", "{count: 2}", "9223372036854775808", "99999999999999999999999999999"} {
			t.Run(field+"="+value, func(t *testing.T) {
				body := fmt.Sprintf("queries:\n  - keywords: software engineer\n    location: Germany\n    %s: %s\n", field, value)
				queries, err := loadQueries(writeTemp(t, body))
				if err == nil || !strings.Contains(err.Error(), field) || queries != nil {
					t.Fatalf("malformed %s=%s produced queries=%+v err=%v", field, value, queries, err)
				}
			})
		}
	}
}

func TestRegistrationPreviewRejectsFractionalLimits(t *testing.T) {
	path := writeTemp(t, "queries:\n  - keywords: software engineer\n    location: Germany\n    pages: 1.5\n    jobage: 0.5\n")
	if exit := runArgs([]string{"--register-searches", path}); exit != 1 {
		t.Fatalf("fractional search preview exit=%d, want 1", exit)
	}
}

func TestLoadQueriesRetainsOmittedAndExplicitZeroDefaults(t *testing.T) {
	for _, limits := range []string{"", "    pages: 0\n    jobage: 0\n"} {
		queries, err := loadQueries(writeTemp(t, "queries:\n  - keywords: software engineer\n    location: Germany\n"+limits))
		if err != nil || len(queries) != 1 || queries[0].Pages != defaultPages || queries[0].JobAge != defaultJobAge {
			t.Fatalf("legacy defaults for %q = %+v, %v", limits, queries, err)
		}
	}
}

func TestLoadQueriesAcceptsTheCheckedInHarvestWorklist(t *testing.T) {
	queries, err := loadQueries(filepath.Join("..", "..", "harvest", "linkedin-queries.yml"))
	if err != nil || len(queries) != 1654 {
		t.Fatalf("checked-in harvest: searches=%d, %v", len(queries), err)
	}
	// A firehose lane rotation takes ~13h, so a shorter window skips postings between visits;
	// the primary pass is one page so detail reads go to the Easy Apply pass.
	firehose := 0
	for _, q := range queries {
		if q.Keywords != sources.LinkedInAnyKeyword {
			continue
		}
		firehose++
		if q.JobAgeHours != 24 || q.Pages != 1 {
			t.Errorf("firehose %q: jobage_hours=%d pages=%d, want 24 and 1", q.Location, q.JobAgeHours, q.Pages)
		}
	}
	if firehose != 125 {
		t.Errorf("checked-in harvest: firehose searches=%d, want 125", firehose)
	}
}
