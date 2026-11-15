package sources

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestBullhornUsesConfiguredProxy(t *testing.T) {
	t.Setenv("SOURCES_PROXY_URL", "http://proxy.example:8080")
	t.Setenv("SOURCES_ROTATING_PROXY_URL", "")
	registry := map[string]Source{"bullhorn": NewBullhorn(NewClient())}
	if err := ApplyProxyEgress(registry); err != nil {
		t.Fatal(err)
	}
	s := registry["bullhorn"].(bullhorn)
	c, ok := s.http.(*Client)
	if !ok {
		t.Fatal("Bullhorn lost its standard client")
	}
	tr, ok := c.httpClient.Transport.(*http.Transport)
	if !ok || tr.Proxy == nil {
		t.Fatal("Bullhorn has no proxy transport")
	}
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "https://public-rest41.bullhornstaffing.com/rest-services/DJVM1/search/JobOrder", nil)
	if err != nil {
		t.Fatal(err)
	}
	proxy, err := tr.Proxy(req)
	if err != nil || proxy == nil || proxy.Host != "proxy.example:8080" {
		t.Fatalf("proxy=%v err=%v", proxy, err)
	}
}

func TestSeniorScraplingRoutePreservesPOSTFlow(t *testing.T) {
	calls := []string{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			URL, Method, Mode, Body string
			Proxy                   bool
			Headers                 map[string]string
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Error(err)
			w.WriteHeader(400)
			return
		}
		if req.Method != "POST" || req.Mode != "http" || !req.Proxy || req.Headers["Content-Type"] != "application/json" {
			t.Errorf("incorrect Senior wire request: method=%s mode=%s proxy=%v", req.Method, req.Mode, req.Proxy)
		}
		raw, err := base64.StdEncoding.DecodeString(req.Body)
		if err != nil {
			t.Error(err)
			w.WriteHeader(400)
			return
		}
		var body map[string]any
		if err := json.Unmarshal(raw, &body); err != nil {
			t.Error(err)
			w.WriteHeader(400)
			return
		}
		calls = append(calls, req.URL)
		response := ""
		switch {
		case strings.HasSuffix(req.URL, "/actions/getProfileIdBySubdomain"):
			if body["subdomain"] != "intelbras" {
				t.Error("lost tenant")
			}
			response = `{"profileId":"profile-1"}`
		case strings.HasSuffix(req.URL, "/queries/searchVacancies"):
			if body["page"] != float64(0) || body["size"] != float64(seniorPageSize) || !strings.Contains(string(raw), "profile-1") {
				t.Error("lost search payload")
			}
			response = `{"totalPages":1,"contents":[{"vacancy":{"id":"v1","title":"Software Engineer"}}]}`
		case strings.HasSuffix(req.URL, "/queries/findVacancyById"):
			if body["id"] != "v1" {
				t.Error("lost vacancy ID")
			}
			response = `{"vacancy":{"about":{"description":"<p>Build software.</p>"}}}`
		default:
			t.Errorf("unexpected endpoint %s", req.URL)
			w.WriteHeader(400)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"status": 200, "url": req.URL, "headers": map[string]string{}, "body": base64.StdEncoding.EncodeToString([]byte(response))})
	}))
	defer server.Close()
	t.Setenv("SCRAPLING_FETCH_URL", server.URL)
	t.Setenv("SCRAPLING_FETCH_TOKEN", "test")
	registry := map[string]Source{"senior": NewSenior(nil)}
	if err := ApplyScraplingEgress(registry); err != nil {
		t.Fatal(err)
	}
	s := registry["senior"].(senior)
	if s.http == nil {
		t.Fatal("Senior has no routed client")
	}
	jobs, err := s.Fetch(context.Background(), CompanyEntry{Board: "intelbras", Company: "Intelbras"})
	if err != nil || len(jobs) != 1 || len(calls) != 3 {
		t.Fatalf("jobs=%d calls=%d err=%v", len(jobs), len(calls), err)
	}
	if jobs[0].ExternalID != "v1" || jobs[0].Description != "<p>Build software.</p>" {
		t.Fatalf("unexpected job: %+v", jobs[0])
	}
}
