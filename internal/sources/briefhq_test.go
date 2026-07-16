package sources

import (
	"context"
	"testing"
)

// briefhqCareersHTML is a briefhq-shaped static careers page: two vacancies each inlined as their
// own schema.org JobPosting ld+json block, plus the Organization and BreadcrumbList blocks the
// real page also carries — which LDJobPostings must ignore. The first posting is on-site with a
// full jobLocation address; the second is fully remote (TELECOMMUTE, no address).
const briefhqCareersHTML = `<html><head>
<script type="application/ld+json">{"@type":"Organization","name":"Brief"}</script>
<script type="application/ld+json">{"@type":"BreadcrumbList","itemListElement":[]}</script>
<script type="application/ld+json">
{"@context":"https://schema.org","@type":"JobPosting",
"title":"Product Builder (Forward Deployed)",
"url":"https://briefhq.ai/careers/#product-builder-forward-deployed",
"description":"<p>Ship product with customers.</p>",
"datePosted":"2026-07-06",
"employmentType":"FULL_TIME",
"jobLocationType":"ON_SITE",
"hiringOrganization":{"@type":"Organization","name":"Brief"},
"jobLocation":{"@type":"Place","address":{"@type":"PostalAddress",
"addressLocality":"San Francisco","addressRegion":"CA","addressCountry":"US"}},
"identifier":{"@type":"PropertyValue","name":"Brief","value":"product-builder-forward-deployed"}}
</script>
<script type="application/ld+json">
{"@context":"https://schema.org","@type":"JobPosting",
"title":"Senior Product Builder (Engineering Focus)",
"url":"https://briefhq.ai/careers/#senior-product-builder-engineering",
"description":"<p>Own the codebase.</p>",
"datePosted":"2026-07-06",
"employmentType":"FULL_TIME",
"jobLocationType":"TELECOMMUTE",
"hiringOrganization":{"@type":"Organization","name":"Brief"},
"identifier":{"@type":"PropertyValue","name":"Brief","value":"senior-product-builder-engineering"}}
</script>
</head><body></body></html>`

func TestBriefHQProvider(t *testing.T) {
	if got := NewBriefHQ(nil).Provider(); got != "briefhq" {
		t.Errorf("Provider() = %q, want %q", got, "briefhq")
	}
}

func TestBriefHQFetchMapsInlinePostings(t *testing.T) {
	fake := &fakeHTTP{body: briefhqCareersHTML}

	jobs, err := NewBriefHQ(fake).Fetch(context.Background(), CompanyEntry{
		Company: "Brief", Provider: "briefhq", Board: "briefhq.ai/careers/",
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if fake.gotURL != "https://briefhq.ai/careers/" {
		t.Errorf("fetched %q, want the careers page built from the board", fake.gotURL)
	}
	if len(jobs) != 2 {
		t.Fatalf("got %d jobs, want 2 (Organization/BreadcrumbList blocks must be ignored)", len(jobs))
	}

	onsite := jobs[0]
	if onsite.ExternalID != "product-builder-forward-deployed" {
		t.Errorf("ExternalID = %q, want identifier.value", onsite.ExternalID)
	}
	if onsite.URL != "https://briefhq.ai/careers/#product-builder-forward-deployed" {
		t.Errorf("URL = %q, want the posting deeplink", onsite.URL)
	}
	if onsite.Title != "Product Builder (Forward Deployed)" {
		t.Errorf("Title = %q", onsite.Title)
	}
	if onsite.Company != "Brief" {
		t.Errorf("Company = %q, want hiringOrganization name", onsite.Company)
	}
	if onsite.Location != "San Francisco, CA, US" {
		t.Errorf("Location = %q, want the jobLocation address", onsite.Location)
	}
	if onsite.Remote || onsite.WorkMode != "" {
		t.Errorf("on-site posting Remote=%v WorkMode=%q, want false/empty", onsite.Remote, onsite.WorkMode)
	}
	if onsite.EmploymentType != "full_time" {
		t.Errorf("EmploymentType = %q, want full_time", onsite.EmploymentType)
	}
	if onsite.Description != "<p>Ship product with customers.</p>" {
		t.Errorf("Description = %q", onsite.Description)
	}
	if onsite.PostedAt == nil || onsite.PostedAt.Format("2006-01-02") != "2026-07-06" {
		t.Errorf("PostedAt = %v, want 2026-07-06", onsite.PostedAt)
	}

	remote := jobs[1]
	if remote.ExternalID != "senior-product-builder-engineering" {
		t.Errorf("ExternalID = %q, want identifier.value", remote.ExternalID)
	}
	if !remote.Remote || remote.WorkMode != "remote" {
		t.Errorf("TELECOMMUTE posting Remote=%v WorkMode=%q, want true/remote", remote.Remote, remote.WorkMode)
	}
	if remote.Location != "" {
		t.Errorf("Location = %q, want empty (no jobLocation address)", remote.Location)
	}
}
