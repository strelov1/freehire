package handler

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/strelov1/freehire/internal/ingest/sources"
)

func TestIntakeSourcesRoutesOnlyLinkedInThroughConfiguredScrapling(t *testing.T) {
	var calls int
	fetch := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		var request struct {
			URL, Mode string
			Proxy     bool
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
			return
		}
		if r.URL.Path != "/fetch" || request.URL != "https://www.linkedin.com/jobs-guest/jobs/api/jobPosting/123" || request.Mode != "http" || !request.Proxy {
			t.Errorf("wrong intake fetch route: path=%s request=%+v", r.URL.Path, request)
		}
		body := `<div class="top-card-layout"><h1 class="top-card-layout__title">Java Engineer</h1><a class="topcard__org-name-link">Acme</a><button data-tracking-control-name="public_jobs_apply-link-onsite">Apply</button></div><div class="show-more-less-html__markup"><p>Build Java services.</p></div>`
		_ = json.NewEncoder(w).Encode(map[string]any{"status": 200, "url": request.URL, "headers": map[string]string{}, "body": base64.StdEncoding.EncodeToString([]byte(body))})
	}))
	defer fetch.Close()
	t.Setenv("SCRAPLING_FETCH_URL", fetch.URL)
	t.Setenv("SCRAPLING_FETCH_TOKEN", "test")
	client := sources.NewClient()
	registry := sources.All(client)
	guarded := make(map[string]sources.Source)
	for _, name := range []string{"hrsmart", "greenhouse", "workday", "bayt"} {
		guarded[name] = registry[name]
	}
	registry = intakeSources(registry)
	for name, original := range guarded {
		if !reflect.DeepEqual(original, registry[name]) {
			t.Fatalf("intake changed unrelated source %q", name)
		}
	}
	reader := registry["linkedin"].(interface {
		Posting(context.Context, string) (sources.Job, error)
	})
	posting, err := reader.Posting(context.Background(), "https://www.linkedin.com/jobs/view/123/")
	if err != nil || posting.ExternalID != "123" || posting.ApplyMethod != sources.ApplyOnSite || calls != 1 {
		t.Fatalf("intake LinkedIn fetch=%+v calls=%d error=%v", posting, calls, err)
	}
}

func TestIntakeSourcesKeepsInvalidLinkedInFetchConfigurationUnavailable(t *testing.T) {
	t.Setenv("SCRAPLING_FETCH_URL", "http://127.0.0.1:1")
	t.Setenv("SCRAPLING_FETCH_TOKEN", "")
	registry := intakeSources(sources.All(sources.NewClient()))
	reader := registry["linkedin"].(interface {
		Posting(context.Context, string) (sources.Job, error)
	})
	if _, err := reader.Posting(context.Background(), "https://www.linkedin.com/jobs/view/123/"); err == nil {
		t.Fatal("invalid configured tier silently fell back to plain LinkedIn")
	}
}
