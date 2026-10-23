package sources

import (
	"context"
	"net/http"
	"os"
	"slices"
	"strings"
	"testing"
	"time"
)

// tenmlFixture reads a trimmed copy of a live 10ml.com page (captured 2026-10-23): the listing
// keeps its role anchors plus the language-switcher links to the bare /careers, and each detail
// page keeps only its ld+json blocks — the Organization graph and breadcrumbs as well as the
// JobPosting, so the decoder has to pick the right block.
func tenmlFixture(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile("testdata/tenml/" + name)
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return string(b)
}

func TestTenMLProvider(t *testing.T) {
	src := NewTenML(nil)
	if got := src.Provider(); got != "10ml" {
		t.Errorf("Provider() = %q, want %q", got, "10ml")
	}
	if _, ok := src.(boardless); !ok {
		t.Error("10ml must be boardless: it is a single employer's own careers page")
	}
	if _, ok := src.(aggregator); ok {
		t.Error("10ml must not be an aggregator: every posting is 10ML's own")
	}
}

func TestTenMLSlug(t *testing.T) {
	cases := map[string]string{
		"/careers/senior-full-stack-engineer-ecommerce": "senior-full-stack-engineer-ecommerce",
		"https://10ml.com/careers/data-analytics-lead":  "data-analytics-lead",
		"/careers/head-of-seo-ai-search/":               "head-of-seo-ai-search",
		"/careers/data-analytics-lead?utm_source=x":     "data-analytics-lead",
		"/careers":         "",
		"/careers/":        "",
		"/careers/a/apply": "",
		"/pages/about":     "",
		"/collections/careers/senior-full-stack-engineer-ecommerce-sample": "",
	}
	for href, want := range cases {
		if got := tenmlSlug(href); got != want {
			t.Errorf("tenmlSlug(%q) = %q, want %q", href, got, want)
		}
	}
}

func TestTenMLFetch(t *testing.T) {
	const (
		engineer = "senior-full-stack-engineer-ecommerce"
		decanter = "perfume-decanting-fulfilment-specialist"
		gone     = "head-of-growth-europe"
	)
	// Detail routes precede the listing route, whose match is a prefix of every detail URL.
	// The listing's other nine roles fall through to it, so they read a page with no
	// JobPosting — the unreadable path, not a closure.
	fake := (&routedHTTP{}).
		routeErr("/careers/"+gone, &StatusError{Method: http.MethodGet, Code: http.StatusNotFound}).
		route("/careers/"+engineer, tenmlFixture(t, engineer+".html")).
		route("/careers/"+decanter, tenmlFixture(t, decanter+".html")).
		route("10ml.com/careers", tenmlFixture(t, "listing.html"))

	jobs, err := NewTenML(fake).Fetch(context.Background(), CompanyEntry{Company: "10ML", Provider: "10ml"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	// 12 linked roles: the 404 is dropped, the two fixtures map, the other nine are markers.
	if len(jobs) != 11 {
		t.Fatalf("got %d jobs, want 11", len(jobs))
	}
	byID := map[string]Job{}
	unreadable := 0
	for _, j := range jobs {
		byID[j.ExternalID] = j
		if j.Unreadable {
			unreadable++
			if j.Company != "10ML" || j.URL == "" {
				t.Errorf("unreadable marker %q lacks identity: %+v", j.ExternalID, j)
			}
		}
	}
	if unreadable != 9 {
		t.Errorf("got %d unreadable markers, want 9", unreadable)
	}
	if _, ok := byID[gone]; ok {
		t.Errorf("a 404 opening must be dropped, got %+v", byID[gone])
	}

	t.Run("remote role", func(t *testing.T) {
		j := byID[engineer]
		if j.Unreadable {
			t.Fatal("engineer posting read as unreadable")
		}
		if j.Title != "Senior Full-Stack Engineer, E-commerce (TypeScript, Next.js)" {
			t.Errorf("Title = %q", j.Title)
		}
		if j.URL != "https://10ml.com/careers/"+engineer {
			t.Errorf("URL = %q", j.URL)
		}
		if j.Company != "10ML" {
			t.Errorf("Company = %q", j.Company)
		}
		if j.Location != "Dortmund, DE" {
			t.Errorf("Location = %q, want %q", j.Location, "Dortmund, DE")
		}
		if !j.Remote || j.WorkMode != "remote" {
			t.Errorf("Remote/WorkMode = %v/%q, want true/remote (jobLocationType TELECOMMUTE)", j.Remote, j.WorkMode)
		}
		// The 27 EU member states the role is open in, Czechia included under its short name.
		if len(j.Countries) != 27 || !slices.Contains(j.Countries, "de") ||
			!slices.Contains(j.Countries, "cz") || !slices.Contains(j.Countries, "se") {
			t.Errorf("Countries = %v, want the 27 EU members", j.Countries)
		}
		if j.EmploymentType != "full_time" {
			t.Errorf("EmploymentType = %q", j.EmploymentType)
		}
		if j.SalaryMin == nil || *j.SalaryMin != 110000 || j.SalaryMax == nil || *j.SalaryMax != 155000 ||
			j.SalaryCurrency != "EUR" || j.SalaryPeriod != "year" {
			t.Errorf("salary = %v-%v %s/%s, want 110000-155000 EUR/year",
				deref(j.SalaryMin), deref(j.SalaryMax), j.SalaryCurrency, j.SalaryPeriod)
		}
		want := time.Date(2026, 10, 17, 0, 0, 0, 0, time.UTC)
		if j.PostedAt == nil || !j.PostedAt.Equal(want) {
			t.Errorf("PostedAt = %v, want %v", j.PostedAt, want)
		}
		if !strings.Contains(j.Description, "<h3>What you will do</h3>") || !strings.Contains(j.Description, "<li>") {
			t.Errorf("Description lost its structure: %.200q", j.Description)
		}
	})

	t.Run("on-site role", func(t *testing.T) {
		j := byID[decanter]
		if j.Remote || j.WorkMode != "" {
			t.Errorf("Remote/WorkMode = %v/%q, want false/\"\" (no jobLocationType)", j.Remote, j.WorkMode)
		}
		if !slices.Equal(j.Countries, []string{"de"}) {
			t.Errorf("Countries = %v, want [de] from the office address", j.Countries)
		}
		if j.SalaryMin == nil || *j.SalaryMin != 42000 || j.SalaryMax == nil || *j.SalaryMax != 54000 {
			t.Errorf("salary = %v-%v, want 42000-54000", deref(j.SalaryMin), deref(j.SalaryMax))
		}
	})
}

func TestTenMLFetchListingFailures(t *testing.T) {
	t.Run("listing request fails", func(t *testing.T) {
		fake := (&routedHTTP{}).routeErr("10ml.com/careers", &StatusError{Method: http.MethodGet, Code: http.StatusServiceUnavailable})
		if _, err := NewTenML(fake).Fetch(context.Background(), CompanyEntry{Company: "10ML"}); err == nil {
			t.Fatal("Fetch = nil error, want the listing failure")
		}
	})
	t.Run("listing links nothing", func(t *testing.T) {
		fake := (&routedHTTP{}).route("10ml.com/careers", `<html><body><a href="/careers">Careers</a></body></html>`)
		if _, err := NewTenML(fake).Fetch(context.Background(), CompanyEntry{Company: "10ML"}); err == nil {
			t.Fatal("Fetch = nil error, want an empty listing to fail the crawl")
		}
	})
}
