package sources

import (
	"context"
	"testing"
)

func TestSparkProvider(t *testing.T) {
	if got := NewSpark(nil).Provider(); got != "spark" {
		t.Errorf("Provider() = %q, want %q", got, "spark")
	}
}

// sparkListBody builds a one-page listing with the given items already serialized.
func sparkListBody(items ...string) string {
	body := `{"pageCount":1,"page":1,"pageSize":100,"items":[`
	for i, it := range items {
		if i > 0 {
			body += ","
		}
		body += it
	}
	return body + `]}`
}

func TestSparkFetchMapsListing(t *testing.T) {
	// An Active job whose fields carry the description as the lone HTML field and the work
	// mode as the enum-valued field; recovered by content, not by the numeric field id.
	active := `{"id":203,"name":"  Business Development Assistant  ","vacancyStatusId":20,` +
		`"openDate":"2026-07-14T00:00:00+00:00","fields":{` +
		`"50":"Yerevan","60":"Permanent","120":"<p>Build &amp; grow.</p>","130":"Onsite"}}`
	// A Closed job (status 30) must be skipped.
	closed := `{"id":9,"name":"Old Role","vacancyStatusId":30,"openDate":"2026-01-01T00:00:00+00:00","fields":{}}`

	fake := (&routedHTTP{}).route("/api/jobOpenings", sparkListBody(active, closed))

	jobs, err := NewSpark(fake).Fetch(context.Background(), CompanyEntry{
		Company: "VOLO", Provider: "spark", Board: "volo",
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("len(jobs) = %d, want 1 (Closed job must be skipped)", len(jobs))
	}
	j := jobs[0]
	if j.ExternalID != "203" {
		t.Errorf("ExternalID = %q, want %q", j.ExternalID, "203")
	}
	if j.Title != "Business Development Assistant" {
		t.Errorf("Title = %q, want trimmed name", j.Title)
	}
	if want := "https://volo.spark.work/career/job/203/Business-Development-Assistant"; j.URL != want {
		t.Errorf("URL = %q, want %q", j.URL, want)
	}
	if j.WorkMode != "onsite" {
		t.Errorf("WorkMode = %q, want %q (recovered from the enum-valued field)", j.WorkMode, "onsite")
	}
	if j.Remote {
		t.Error("Remote = true, want false for an onsite role")
	}
	// The description retains sanitized structural HTML (bluemonday allowlist keeps <p>),
	// rather than being flattened to plain text.
	if want := "<p>Build &amp; grow.</p>"; j.Description != want {
		t.Errorf("Description = %q, want %q (the HTML field, sanitized)", j.Description, want)
	}
	if j.PostedAt == nil {
		t.Error("PostedAt = nil, want the openDate")
	}
}
