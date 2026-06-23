package main

import (
	"reflect"
	"testing"
)

func TestSitemapLocs(t *testing.T) {
	xml := `<?xml version="1.0" encoding="UTF-8"?>
<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
<url><loc>https://role.com/jobs/price-scanning-specialist-42600001</loc><lastmod>2026-06-22T10:42:07+00:00</lastmod></url>
<url><loc>https://role.com/jobs/data-scanning-associate-42600002</loc></url>
</urlset>`
	got := sitemapLocs([]byte(xml))
	want := []string{
		"https://role.com/jobs/price-scanning-specialist-42600001",
		"https://role.com/jobs/data-scanning-associate-42600002",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestSitemapIndexLocs(t *testing.T) {
	xml := `<sitemapindex xmlns="http://www.sitemaps.org/schemas/sitemap/0.9"><sitemap><loc>https://role.com/sitemap_jobs853.xml</loc></sitemap><sitemap><loc>https://role.com/sitemap_pages1.xml</loc></sitemap></sitemapindex>`
	got := sitemapLocs([]byte(xml))
	want := []string{"https://role.com/sitemap_jobs853.xml", "https://role.com/sitemap_pages1.xml"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestJobSitemapsNewestFirstDropsPages(t *testing.T) {
	locs := []string{
		"https://role.com/sitemap_pages1.xml",
		"https://role.com/sitemap_jobs401.xml",
		"https://role.com/sitemap_jobs853.xml",
		"https://role.com/sitemap_jobs800.xml",
	}
	got := jobSitemaps(locs)
	want := []string{
		"https://role.com/sitemap_jobs853.xml",
		"https://role.com/sitemap_jobs800.xml",
		"https://role.com/sitemap_jobs401.xml",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestStrideSample(t *testing.T) {
	items := []string{"a", "b", "c", "d", "e", "f", "g", "h", "i", "j"}
	// Fewer wanted than present: evenly spaced, first item always included.
	if got := strideSample(items, 3); !reflect.DeepEqual(got, []string{"a", "d", "g"}) {
		t.Errorf("stride 3 got %v", got)
	}
	// Wanting at least as many as present returns all, in order.
	if got := strideSample(items, 10); !reflect.DeepEqual(got, items) {
		t.Errorf("stride 10 got %v", got)
	}
	if got := strideSample(items, 99); !reflect.DeepEqual(got, items) {
		t.Errorf("stride 99 got %v", got)
	}
	// n <= 0 yields nothing.
	if got := strideSample(items, 0); len(got) != 0 {
		t.Errorf("stride 0 got %v", got)
	}
	if got := strideSample(nil, 3); len(got) != 0 {
		t.Errorf("stride nil got %v", got)
	}
}

func TestClassifyDetailWorkday(t *testing.T) {
	// The role.com detail page links the real posting from an <a class="job-apply ...">
	// and names the employer in its JobPosting JSON-LD.
	html := `<html><head>
<script type="application/ld+json">{"@type":"JobPosting","title":"Senior Software Developer","hiringOrganization":{"@type":"Organization","name":"CACI International Inc"}}</script>
</head><body>
<a href="https://www.caci.com/?utm_source=Role" rel="nofollow" aria-label="Website">Site</a>
<a class="job-apply btn-md btn-purple waves-effect" href="https://caci.wd1.myworkdayjobs.com/en-us/external/job/bethesda/senior-software-developer_328039?utm_source=Role" rel="nofollow" target="_blank">Apply</a>
</body></html>`
	provider, board, company, ok := classifyDetail([]byte(html))
	if !ok {
		t.Fatalf("ok = false, want true")
	}
	if provider != "workday" || board != "caci.wd1.myworkdayjobs.com/external" || company != "CACI International Inc" {
		t.Errorf("got (%q, %q, %q)", provider, board, company)
	}
}

func TestClassifyDetailNoApplyLink(t *testing.T) {
	html := `<html><body><a href="/login">Login</a></body></html>`
	if _, _, _, ok := classifyDetail([]byte(html)); ok {
		t.Error("ok = true, want false for a page with no apply link")
	}
}

func TestClassifyDetailUnsupportedAts(t *testing.T) {
	html := `<a class="job-apply" href="https://myjobs.adp.com/afcindustries/cx/job-details?reqid=5001">Apply</a>`
	if _, _, _, ok := classifyDetail([]byte(html)); ok {
		t.Error("ok = true, want false for an unsupported ATS shape")
	}
}
