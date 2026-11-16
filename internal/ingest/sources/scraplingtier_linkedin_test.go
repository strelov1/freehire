package sources

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/time/rate"
)

func TestScraplingLinkedInSharesItsPacedHTTPProxyRouteAcrossAllReads(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			URL, Mode string
			Proxy     bool
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Error(err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		calls.Add(1)
		if r.URL.Path != "/fetch" || req.Mode != "http" || !req.Proxy {
			t.Errorf("wrong LinkedIn fetch route: path=%s mode=%s proxy=%v", r.URL.Path, req.Mode, req.Proxy)
		}
		u, err := url.Parse(req.URL)
		if err != nil {
			t.Error(err)
			return
		}
		body := ""
		switch {
		case u.Path == "/jobs-guest/jobs/api/seeMoreJobPostings/search":
			if u.Query().Get("start") == "0" {
				body = linkedInRelatedListing("123")
			}
		case strings.HasPrefix(u.Path, "/jobs-guest/jobs/api/jobPosting/"):
			body = linkedInRelatedPosting(strings.TrimPrefix(u.Path, "/jobs-guest/jobs/api/jobPosting/"))
		case u.Path == "/jobs/view/123/":
			body = relatedTestPage(relatedTestCard("456", "https://www.linkedin.com/jobs/view/456/"))
		case u.Path == "/company/acme":
			body = `<script type="application/ld+json">{"@type":"Organization","sameAs":"https://acme.example"}</script>`
		default:
			t.Errorf("unexpected upstream URL %s", req.URL)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"status": 200, "url": req.URL, "headers": map[string]string{}, "body": base64.StdEncoding.EncodeToString([]byte(body))})
	}))
	defer server.Close()
	t.Setenv("SCRAPLING_FETCH_URL", server.URL)
	t.Setenv("SCRAPLING_FETCH_TOKEN", "test")
	registry := map[string]Source{"linkedin": NewLinkedIn(nil)}
	if err := ApplyScraplingEgress(registry); err != nil {
		t.Fatal(err)
	}
	source := registry["linkedin"].(linkedIn)
	client, ok := source.http.(*Client)
	if !ok || !isScrapling(client) {
		t.Fatalf("LinkedIn was not rebuilt over the fetch service: %T", source.http)
	}
	limiter, ok := source.pace.(*rate.Limiter)
	if !ok || limiter.Limit() != rate.Every(2*time.Second) || limiter.Burst() != 1 {
		t.Fatal("LinkedIn request pacing was lost during transport rebuild")
	}
	pace := &recordingWaiter{}
	source.pace = pace
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	jobs, err := source.Fetch(ctx, CompanyEntry{Board: "software engineer", Region: "Netherlands"})
	if err != nil || len(jobs) != 2 || jobs[0].ExternalID != "123" || jobs[1].ExternalID != "456" {
		t.Fatalf("listing/detail/recommendations did not traverse the tier: jobs=%+v error=%v", jobs, err)
	}
	website, err := source.CompanyWebsite(ctx, "https://www.linkedin.com/company/acme")
	if err != nil || website != "https://acme.example" {
		t.Fatalf("company profile did not traverse the same tier: website=%s error=%v", website, err)
	}
	if calls.Load() != 6 || pace.calls != 6 {
		t.Fatalf("requests bypassed or duplicated the shared budget: fetches=%d paced=%d", calls.Load(), pace.calls)
	}
}
