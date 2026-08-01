package linkimport

import (
	"testing"
	"time"

	"github.com/strelov1/freehire/internal/linksource"
	"github.com/strelov1/freehire/internal/sources"
)

// A destination adapter that states the posting's own date supplies it as the source
// posted time. It reaches the aggregate through the draft, so the write maps — and
// fingerprints — the posted_at that is actually stored.
func TestDraftFrom_CarriesResolvedPostedTime(t *testing.T) {
	posted := time.Date(2026, 3, 4, 5, 6, 7, 0, time.UTC)
	d := draftFrom(linksource.Resolved{
		Source: "greenhouse",
		Job: sources.Job{
			ExternalID: "acme/1",
			URL:        "https://boards.greenhouse.io/acme/jobs/1",
			Title:      "Senior Go Developer",
			Company:    "Acme",
			PostedAt:   &posted,
		},
	})

	if d.PostedAt == nil || !d.PostedAt.Equal(posted) {
		t.Errorf("PostedAt = %v, want %v", d.PostedAt, posted)
	}
}

// Most destination pages state no date. The draft then leaves posted_at unset rather
// than stamping the import time, so freshness can tell "unknown" from "posted now".
func TestDraftFrom_UnstatedPostedTimeStaysNil(t *testing.T) {
	d := draftFrom(linksource.Resolved{
		Source: "greenhouse",
		Job: sources.Job{
			ExternalID: "acme/2",
			URL:        "https://boards.greenhouse.io/acme/jobs/2",
			Title:      "Backend Engineer",
			Company:    "Acme",
		},
	})

	if d.PostedAt != nil {
		t.Errorf("PostedAt = %v, want nil", d.PostedAt)
	}
}

// The draft carries the resolved identity and content verbatim; derivation happens
// inside job.New, never here.
func TestDraftFrom_CarriesIdentityAndContent(t *testing.T) {
	d := draftFrom(linksource.Resolved{
		Source: "greenhouse",
		Job: sources.Job{
			ExternalID:  "acme/3",
			URL:         "https://boards.greenhouse.io/acme/jobs/3",
			Title:       "Platform Engineer",
			Company:     "Acme",
			Location:    "Berlin",
			Description: "We use Go.",
			Remote:      true,
			WorkMode:    "remote",
		},
	})

	if d.Source != "greenhouse" || d.ExternalID != "acme/3" {
		t.Errorf("identity = %q/%q", d.Source, d.ExternalID)
	}
	if d.Title != "Platform Engineer" || d.Company != "Acme" || d.Location != "Berlin" {
		t.Errorf("content = %q/%q/%q", d.Title, d.Company, d.Location)
	}
	if d.Description != "We use Go." || d.WorkMode != "remote" {
		t.Errorf("description/workMode = %q/%q", d.Description, d.WorkMode)
	}
	if d.URL != "https://boards.greenhouse.io/acme/jobs/3" || !d.Remote {
		t.Errorf("url/remote = %q/%v", d.URL, d.Remote)
	}
}
