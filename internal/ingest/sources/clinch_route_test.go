package sources

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"golang.org/x/time/rate"
)

func TestClinchScraplingDetailRoute(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			URL, Mode string
			Proxy     bool
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Error(err)
		}
		if req.Mode != "http" || !req.Proxy || !strings.Contains(req.URL, "/jobs/") {
			t.Errorf("request=%+v", req)
		}
		calls++
		body, err := os.ReadFile("testdata/clinch/586.html")
		if err != nil {
			t.Error(err)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"status": 200, "url": req.URL, "headers": map[string]string{}, "body": base64.StdEncoding.EncodeToString(body)})
	}))
	defer server.Close()
	t.Setenv("SCRAPLING_FETCH_URL", server.URL)
	t.Setenv("SCRAPLING_FETCH_TOKEN", "test")
	registry := map[string]Source{"clinch": NewClinch(nil, nil)}
	if err := ApplyScraplingEgress(registry); err != nil {
		t.Fatal(err)
	}
	c := registry["clinch"].(clinch)
	if _, ok := c.sitemap.(*Client); !ok {
		t.Fatal("sitemap lost direct client")
	}
	paced, ok := c.detail.(rateLimitedHTMLGetter)
	if !ok {
		t.Fatal("detail lost pacing")
	}
	limiter := paced.limiter.(*rate.Limiter)
	if limiter.Limit() != rate.Every(clinchRequestInterval) || limiter.Burst() != clinchRequestBurst {
		t.Fatal("detail budget changed")
	}
	url := "https://careers.moretonbay.qld.gov.au/jobs/parks-labourer-expression-of-interest-eoi-grounds-maintenance-natural-areas-trees-city-of-moreton-bay-qld-australia"
	c.sitemap = (&routedHTTP{}).route("/sitemap.xml", clinchSitemapXML(url))
	jobs, err := c.Fetch(context.Background(), CompanyEntry{Board: "careers.moretonbay.qld.gov.au", Company: "City of Moreton Bay"})
	if err != nil || len(jobs) != 1 || calls != 1 {
		t.Fatalf("jobs=%d calls=%d err=%v", len(jobs), calls, err)
	}
	if jobs[0].Description == "" || jobs[0].Title == "" || jobs[0].Company != "City of Moreton Bay" {
		t.Fatalf("job=%+v", jobs[0])
	}
}

func TestPageupCustomTransportLeavesCanonicalDirect(t *testing.T) {
	plain := &fakePageup{search: `{"count":0,"results":""}`}
	custom := &fakePageup{search: `{"count":0,"results":""}`}
	p := NewPageUp(pageupCustomHTTP{plain: plain, custom: custom})
	for _, board := range []string{"889", "1023"} {
		if _, err := p.Fetch(context.Background(), CompanyEntry{Board: board}); err != nil {
			t.Fatal(err)
		}
	}
	if len(plain.calls) != 1 || len(custom.calls) != 1 || !strings.HasPrefix(plain.calls[0], pageupHost) {
		t.Fatalf("plain=%v custom=%v", plain.calls, custom.calls)
	}
}

func TestClinchWaymoUsesVerifiedProxyRoute(t *testing.T) {
	url := "https://careers.withwaymo.com/jobs/" +
		"software-engineer-mountain-view-california-united-states-050e0262-1397-4e90-a506-f0087e68b30b"
	plain := &routedDetail{routes: map[string]string{}}
	proxy := &routedDetail{routes: map[string]string{url: clinchDetailHTML("Build autonomous driving systems.")}}
	getter := clinchDetailHTTP{plain: plain, migrated: proxy}
	if _, err := getter.GetHTML(context.Background(), url); err != nil {
		t.Fatal(err)
	}
	if len(proxy.calls) != 1 || len(plain.calls) != 0 {
		t.Fatalf("proxy calls=%d, direct calls=%d", len(proxy.calls), len(plain.calls))
	}
}

func TestClinchOtherHostsKeepDirectDetails(t *testing.T) {
	plain := &routedDetail{routes: map[string]string{"/jobs/test": clinchDetailHTML("Direct body")}}
	migrated := &routedDetail{routes: map[string]string{}}
	getter := clinchDetailHTTP{plain: plain, migrated: migrated}
	if _, err := getter.GetHTML(context.Background(), "https://careers.example.org/jobs/test"); err != nil {
		t.Fatal(err)
	}
	if len(plain.calls) != 1 || len(migrated.calls) != 0 {
		t.Fatal("non-migrated site changed egress")
	}
	if len(clinchMigratedHosts) != 39 {
		t.Fatalf("hosts=%d", len(clinchMigratedHosts))
	}
}

func TestClinchPrivateSitemapLinkNeverReachesProxy(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { t.Error("private endpoint reached") }))
	defer server.Close()
	migrated := &routedDetail{routes: map[string]string{}}
	getter := clinchDetailHTTP{plain: NewClient(), migrated: migrated}
	if _, err := getter.GetHTML(context.Background(), server.URL+"/jobs/internal"); err == nil {
		t.Fatal("private target accepted")
	}
	if len(migrated.calls) != 0 {
		t.Fatal("private target reached proxy lane")
	}
}

func TestPageupRegistryRoutesSearchAndDetailThroughScrapling(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			URL, Mode string
			Proxy     bool
			Headers   map[string]string
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Error(err)
			return
		}
		if req.Mode != "http" || !req.Proxy || req.Headers["X-Requested-With"] != "XMLHttpRequest" {
			t.Errorf("request=%+v", req)
		}
		calls++
		body := `{"results":"<div id=\"job-details\"><p>Hospital posting</p></div>"}`
		if strings.Contains(req.URL, "/search/") {
			body = `{"count":1,"results":"<a class=\"job-link\" href=\"/nmh/en-us/job/514883/environmental-services-aide\">Environmental Services Aide</a>"}`
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"status": 200, "url": req.URL, "headers": map[string]string{}, "body": base64.StdEncoding.EncodeToString([]byte(body))})
	}))
	defer server.Close()
	t.Setenv("SCRAPLING_FETCH_URL", server.URL)
	t.Setenv("SCRAPLING_FETCH_TOKEN", "test")
	registry := map[string]Source{"pageup": NewPageUp(nil)}
	if err := ApplyScraplingEgress(registry); err != nil {
		t.Fatal(err)
	}
	jobs, err := registry["pageup"].Fetch(context.Background(), CompanyEntry{Board: "1023", Company: "1023"})
	if err != nil || len(jobs) != 1 || calls != 2 {
		t.Fatalf("jobs=%v calls=%d err=%v", jobs, calls, err)
	}
	if jobs[0].ExternalID != "514883" || jobs[0].Description != "<p>Hospital posting</p>" {
		t.Fatalf("job=%+v", jobs[0])
	}
	canonical := &fakePageup{search: `{"count":0,"results":""}`}
	p := registry["pageup"].(pageup)
	dispatch := p.http.(pageupCustomHTTP)
	if _, ok := dispatch.plain.(*Client); !ok {
		t.Fatal("canonical client is not direct")
	}
	dispatch.plain = canonical
	if _, err := NewPageUp(dispatch).Fetch(context.Background(), CompanyEntry{Board: "889"}); err != nil {
		t.Fatal(err)
	}
	if len(canonical.calls) != 1 || calls != 2 {
		t.Fatal("canonical board entered Scrapling")
	}
}
