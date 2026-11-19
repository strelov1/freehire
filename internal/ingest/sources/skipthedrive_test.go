package sources

import (
	"context"
	"slices"
	"strings"
	"testing"
)

const skipthedriveSitemap = `<urlset><url><loc>https://www.skipthedrive.com/</loc></url>
<url><loc>https://www.skipthedrive.com/job-category/remote-sales-jobs/</loc></url>
<url><loc>https://www.skipthedrive.com/job-category/remote-it-jobs/</loc></url></urlset>`

// row mirrors a live listing row: the company span leads with an &nbsp;.
func skipthedriveRow(slug, title, company string) string {
	return `<div class="post-content"><h2 class="post-title entry-title"><a href="https://www.skipthedrive.com/job/` + slug + `/">` + title +
		`</a></h2><div class='company_date_results_wrapper'><div id='favicon-div-wrapper'><span class='custom_fields_company_name_display_search_results'>&nbsp;` +
		company + `</span></div></div><p>excerpt</p></div>`
}

const skipthedriveDetail = `<html><head><meta property="article:published_time" content="2026-07-28T04:00:00+00:00"></head><body>
<div class="entry-content"><div class='job-meta-div'><div class="custom_fields_display"><span class="fa fa-map-marker"></span>Denver (United States)
</div></div><div id='skipthedrive_section'><h2>Description</h2><p>Build solar software</p></div>
<div class="apply_to_position_div"><button><a href="https://apply.workable.com/j/7CA9F2F7F8" target="_blank">Apply</a></button></div></div></body></html>`

func skipthedriveFake() *routedHTTP {
	return (&routedHTTP{}).
		route("sitemap.xml", skipthedriveSitemap).
		route("remote-sales-jobs/page/2/", `<html><body></body></html>`).
		route("remote-it-jobs/page/2/", `<html><body></body></html>`).
		route("remote-sales-jobs/", `<html><body>`+
			skipthedriveRow("acme-account-executive-111", "Account Executive", "Acme")+
			skipthedriveRow("nocompany-thing-222", "No Company", " ")+
			`<div class="post-content"><h2 class="post-title"><a href="https://www.skipthedrive.com/about-us/">x</a></h2></div></body></html>`).
		route("remote-it-jobs/", `<html><body>`+
			skipthedriveRow("acme-account-executive-111", "Account Executive", "Acme")+ // same posting in two categories
			skipthedriveRow("globex-sysadmin-333", "Sysadmin", "Globex")+`</body></html>`).
		route("acme-account-executive-111", skipthedriveDetail).
		route("globex-sysadmin-333", `<html><body>a wall</body></html>`)
}

func TestSkipTheDriveMarkers(t *testing.T) {
	s := NewSkipTheDrive(nil)
	if s.Provider() != "skipthedrive" {
		t.Errorf("Provider() = %q", s.Provider())
	}
	if _, ok := s.(boardless); !ok {
		t.Error("should be boardless")
	}
	if _, ok := s.(aggregator); !ok {
		t.Error("should be an aggregator")
	}
	if !slices.Contains(FilterableProviders(), "skipthedrive") {
		t.Error("FilterableProviders() should include skipthedrive")
	}
}

func TestSkipTheDriveFetchDedupsAndMapsListing(t *testing.T) {
	jobs, err := NewSkipTheDrive(skipthedriveFake()).Fetch(context.Background(), CompanyEntry{})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 2 {
		t.Fatalf("got %d jobs, want 2 (dup, company-less and non-job rows dropped): %+v", len(jobs), jobs)
	}
	j := jobs[0]
	if j.ExternalID != "111" || j.Title != "Account Executive" || j.Company != "Acme" || !j.Remote || j.WorkMode != "remote" ||
		j.URL != "https://www.skipthedrive.com/job/acme-account-executive-111/" {
		t.Errorf("mapped wrong: %+v", j)
	}
}

func TestSkipTheDriveFetchNewHydratesOnlyUnseen(t *testing.T) {
	seen := func(id string) bool { return id == "333" }
	jobs, err := NewSkipTheDrive(skipthedriveFake()).(HydratingSource).FetchNew(context.Background(), CompanyEntry{}, seen)
	if err != nil {
		t.Fatalf("FetchNew: %v", err)
	}
	byID := map[string]Job{}
	for _, j := range jobs {
		byID[j.ExternalID] = j
	}
	if !byID["333"].SeenRefresh {
		t.Errorf("seen posting should be a bare refresh: %+v", byID["333"])
	}
	a := byID["111"]
	if !strings.Contains(a.Description, "Build solar software") || a.Location != "Denver (United States)" ||
		a.ExternalApplyURL != "https://apply.workable.com/j/7CA9F2F7F8" || a.ApplyMethod != ApplyExternal ||
		a.PostedAt == nil || a.PostedAt.Format("2006-01-02") != "2026-07-28" {
		t.Errorf("detail mapped wrong: %+v", a)
	}
}

func TestSkipTheDriveFetchNewFailsWhenNoPageReadable(t *testing.T) {
	_, err := NewSkipTheDrive(skipthedriveFake()).(HydratingSource).FetchNew(context.Background(), CompanyEntry{},
		func(id string) bool { return id == "111" }) // only the walled page is due
	if err == nil {
		t.Fatal("want an error when every due page is unreadable")
	}
}

func TestSkipTheDriveSitemapWithoutCategoriesFails(t *testing.T) {
	fake := (&routedHTTP{}).route("sitemap.xml", `<urlset></urlset>`)
	if _, err := NewSkipTheDrive(fake).Fetch(context.Background(), CompanyEntry{}); err == nil {
		t.Fatal("want an error for a sitemap with no category")
	}
}
