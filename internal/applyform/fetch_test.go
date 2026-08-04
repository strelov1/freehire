package applyform

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"golang.org/x/net/html"
)

// fakeTransport records what was requested and replays a canned body, so a fetcher's URL
// construction and decoding are exercised without the network.
type fakeTransport struct {
	body    string
	err     error
	gotURL  string
	gotBody any
}

func (f *fakeTransport) GetHTML(_ context.Context, url string) (*html.Node, error) {
	f.gotURL = url
	if f.err != nil {
		return nil, f.err
	}
	return html.Parse(strings.NewReader(f.body))
}

func (f *fakeTransport) GetJSON(_ context.Context, url string, v any) error {
	f.gotURL = url
	if f.err != nil {
		return f.err
	}
	return json.Unmarshal([]byte(f.body), v)
}

func (f *fakeTransport) PostJSONWithHeaders(_ context.Context, url string, _ map[string]string, body, v any) error {
	f.gotURL = url
	f.gotBody = body
	if f.err != nil {
		return f.err
	}
	return json.Unmarshal([]byte(f.body), v)
}

func TestGreenhouseFetcher(t *testing.T) {
	tr := &fakeTransport{body: `{
		"questions": [
			{"label": "Email", "required": true,
			 "fields": [{"name": "email", "type": "input_text", "values": []}]}
		]
	}`}

	form, err := Fetchers(tr)["greenhouse"].Fetch(context.Background(), Claimed{ExternalID: "stripe:7954688"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}

	// questions=true is the whole reason this costs a request: without it the endpoint
	// answers 200 with a posting carrying no form at all.
	if !strings.Contains(tr.gotURL, "/boards/stripe/jobs/7954688") || !strings.Contains(tr.gotURL, "questions=true") {
		t.Errorf("requested %q, want the per-posting endpoint with questions=true", tr.gotURL)
	}
	if form.Provider != "greenhouse" || len(form.Fields) != 1 || form.Fields[0].ID != "email" {
		t.Errorf("form = %+v, want the decoded email control", form)
	}
}

func TestAshbyFetcher(t *testing.T) {
	tr := &fakeTransport{body: `{
		"data": {"jobPosting": {"applicationForm": {"sections": [
			{"title": "Personal details", "fieldEntries": [
				{"id": "f1__systemfield_email", "isRequired": true,
				 "field": {"path": "_systemfield_email", "title": "Email", "type": "Email"}}
			]}
		]}}}
	}`}

	form, err := Fetchers(tr)["ashby"].Fetch(context.Background(), Claimed{ExternalID: "n8n:47cc47be"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}

	if !strings.Contains(tr.gotURL, "non-user-graphql") {
		t.Errorf("requested %q, want the job-board GraphQL endpoint", tr.gotURL)
	}
	if form.Provider != "ashby" || len(form.Fields) != 1 || form.Fields[0].ID != "_systemfield_email" {
		t.Errorf("form = %+v, want the decoded email control", form)
	}
}

// Ashby types the entry's `field` as JSON!, so a GraphQL selection set on it fails the
// WHOLE query rather than that one field. The query must therefore ask for it as a bare
// scalar — a regression here would look like every Ashby capture failing at once.
func TestAshbyQueryAsksForFieldAsAScalar(t *testing.T) {
	tr := &fakeTransport{body: `{"data":{"jobPosting":{"applicationForm":{"sections":[]}}}}`}

	if _, err := Fetchers(tr)["ashby"].Fetch(context.Background(), Claimed{ExternalID: "n8n:x"}); err != nil {
		t.Fatalf("Fetch: %v", err)
	}

	body, ok := tr.gotBody.(map[string]any)
	if !ok {
		t.Fatalf("request body = %T, want a JSON object", tr.gotBody)
	}
	query, _ := body["query"].(string)
	if strings.Contains(query, "field {") || strings.Contains(query, "field{") {
		t.Errorf("query selects subfields of `field`, which fails the whole request:\n%s", query)
	}
	if !strings.Contains(query, "field") {
		t.Errorf("query does not ask for `field` at all:\n%s", query)
	}
}

// A posting Ashby does not know about answers 200 with a null jobPosting rather than an
// error, so the fetcher has to notice by itself.
func TestAshbyFetcherRejectsAnAbsentPosting(t *testing.T) {
	tr := &fakeTransport{body: `{"data": {"jobPosting": null}}`}

	if _, err := Fetchers(tr)["ashby"].Fetch(context.Background(), Claimed{ExternalID: "n8n:gone"}); err == nil {
		t.Error("Fetch() = nil error for a posting the platform does not have")
	}
}

func TestFetcherPropagatesTransportFailure(t *testing.T) {
	want := errors.New("boom")
	for _, provider := range []string{"greenhouse", "ashby"} {
		tr := &fakeTransport{err: want}
		if _, err := Fetchers(tr)[provider].Fetch(context.Background(), Claimed{ExternalID: "b:1"}); !errors.Is(err, want) {
			t.Errorf("%s: Fetch() error = %v, want it to wrap %v", provider, err, want)
		}
	}
}

func TestSplitBoardPosting(t *testing.T) {
	for _, tc := range []struct {
		externalID, board, posting string
		ok                         bool
	}{
		{"stripe:7954688", "stripe", "7954688", true},
		{"n8n:47cc47be-f805-4ba1-8e41-a0a182b7960e", "n8n", "47cc47be-f805-4ba1-8e41-a0a182b7960e", true},
		// The board comes first, so a colon inside the posting id belongs to the id.
		{"board:a:b", "board", "a:b", true},
		{"nocolon", "", "", false},
		{"", "", "", false},
	} {
		board, posting, ok := splitBoardPosting(tc.externalID)
		if ok != tc.ok || board != tc.board || posting != tc.posting {
			t.Errorf("splitBoardPosting(%q) = (%q, %q, %v), want (%q, %q, %v)",
				tc.externalID, board, posting, ok, tc.board, tc.posting, tc.ok)
		}
	}
}

// Board names reach us percent-encoded, because that is what belongs in the URL PATH the
// crawl adapter builds. Ashby's GraphQL takes the organization as a VARIABLE, where an
// encoded name is just a wrong name: the API answers 200 with a null posting, so without
// decoding, every board whose name carries a space fails every capture forever. Found on
// the first supervised production drain — "stony%20creek%20homes" returned nothing while
// "stony creek homes" returned the posting.
func TestAshbyFetcherDecodesTheBoardName(t *testing.T) {
	tr := &fakeTransport{body: `{"data":{"jobPosting":{"applicationForm":{"sections":[]}}}}`}

	if _, err := Fetchers(tr)["ashby"].Fetch(context.Background(), Claimed{ExternalID: "stony%20creek%20homes:x"}); err != nil {
		t.Fatalf("Fetch: %v", err)
	}

	body, ok := tr.gotBody.(map[string]any)
	if !ok {
		t.Fatalf("request body = %T, want a JSON object", tr.gotBody)
	}
	vars, _ := body["variables"].(map[string]any)
	if got := vars["organizationHostedJobsPageName"]; got != "stony creek homes" {
		t.Errorf("organization = %q, want the decoded name", got)
	}
}

// A board name that is not valid percent-encoding is passed through rather than dropped:
// a name is a name, and refusing to fetch would be worse than trying the literal one.
func TestAshbyFetcherKeepsAnUndecodableBoardName(t *testing.T) {
	tr := &fakeTransport{body: `{"data":{"jobPosting":{"applicationForm":{"sections":[]}}}}`}

	if _, err := Fetchers(tr)["ashby"].Fetch(context.Background(), Claimed{ExternalID: "100%discount:x"}); err != nil {
		t.Fatalf("Fetch: %v", err)
	}

	body := tr.gotBody.(map[string]any)
	vars := body["variables"].(map[string]any)
	if got := vars["organizationHostedJobsPageName"]; got != "100%discount" {
		t.Errorf("organization = %q, want the literal name kept", got)
	}
}

// A posting the platform no longer has is not a failure to retry. Our catalogue still
// holds postings employers have taken down — the unseen sweep closes them within 48h — and
// there will be thousands across a 226k backlog. Retried, each burns three requests and
// then dead-letters, and a queue steadily accumulating dead letters is indistinguishable
// from one that is genuinely broken.
type statusErr struct{ code int }

func (e statusErr) Error() string   { return "status" }
func (e statusErr) StatusCode() int { return e.code }

func TestGreenhouseFetcherMarksAGonePostingAsGone(t *testing.T) {
	tr := &fakeTransport{err: statusErr{code: 404}}

	_, err := Fetchers(tr)["greenhouse"].Fetch(context.Background(), Claimed{ExternalID: "sentinellabs:7819844003"})

	if !errors.Is(err, ErrPostingGone) {
		t.Errorf("Fetch() error = %v, want it to mark the posting gone", err)
	}
}

// Anything else is worth another try: a 429 clears on its own, and giving up on it would
// throw away a form the platform is perfectly willing to serve.
func TestGreenhouseFetcherKeepsARateLimitRetryable(t *testing.T) {
	tr := &fakeTransport{err: statusErr{code: 429}}

	_, err := Fetchers(tr)["greenhouse"].Fetch(context.Background(), Claimed{ExternalID: "b:1"})

	if err == nil {
		t.Fatal("Fetch() = nil error on a rate limit")
	}
	if errors.Is(err, ErrPostingGone) {
		t.Errorf("Fetch() error = %v, want a rate limit kept retryable", err)
	}
}

func TestAshbyFetcherMarksAnAbsentPostingAsGone(t *testing.T) {
	tr := &fakeTransport{body: `{"data": {"jobPosting": null}}`}

	_, err := Fetchers(tr)["ashby"].Fetch(context.Background(), Claimed{ExternalID: "n8n:gone"})

	if !errors.Is(err, ErrPostingGone) {
		t.Errorf("Fetch() error = %v, want it to mark the posting gone", err)
	}
}

func TestWorkableFetcher(t *testing.T) {
	tr := &fakeTransport{body: `[
	  {"name": "Personal information", "fields": [
	    {"id": "email", "required": true, "label": "Email", "type": "email"}
	  ]},
	  {"name": "Details", "fields": [
	    {"id": "QA_1", "required": true, "label": "Why this role?", "type": "paragraph"}
	  ]}
	]`}

	form, err := Fetchers(tr)["workable"].Fetch(context.Background(), Claimed{ExternalID: "1000heads:9168DF8334"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}

	// Addressed by the shortcode alone: Workable's form endpoint takes no board or
	// account, so the board half of the external id is not part of the URL.
	if !strings.Contains(tr.gotURL, "/jobs/9168DF8334/form") {
		t.Errorf("requested %q, want the shortcode's form endpoint", tr.gotURL)
	}
	if strings.Contains(tr.gotURL, "1000heads") {
		t.Errorf("requested %q, want the board left out — it addresses nothing here", tr.gotURL)
	}
	if form.Provider != "workable" || len(form.Fields) != 2 {
		t.Errorf("form = %+v, want both controls decoded", form)
	}
}

func TestWorkableFetcherMarksAGonePostingAsGone(t *testing.T) {
	tr := &fakeTransport{err: statusErr{code: 404}}

	if _, err := Fetchers(tr)["workable"].Fetch(context.Background(), Claimed{ExternalID: "b:GONE"}); !errors.Is(err, ErrPostingGone) {
		t.Errorf("Fetch() error = %v, want it to mark the posting gone", err)
	}
}

func TestLeverFetcherPicksTheRegionalHost(t *testing.T) {
	for _, tc := range []struct{ url, wantHost string }{
		{"https://jobs.lever.co/acme/abc", "jobs.lever.co"},
		{"https://jobs.eu.lever.co/silverfin/abc", "jobs.eu.lever.co"},
		// No URL at all falls back to the default host rather than refusing: a wrong
		// guess costs one request, and refusing costs the capture.
		{"", "jobs.lever.co"},
	} {
		tr := &fakeTransport{body: leverPage}
		c := Claimed{ExternalID: "acme:abc", URL: tc.url}

		if _, err := Fetchers(tr)["lever"].Fetch(context.Background(), c); err != nil {
			t.Fatalf("%s: Fetch: %v", tc.url, err)
		}
		if !strings.Contains(tr.gotURL, tc.wantHost) {
			t.Errorf("url %q -> requested %q, want host %q", tc.url, tr.gotURL, tc.wantHost)
		}
		if !strings.HasSuffix(tr.gotURL, "/acme/abc/apply") {
			t.Errorf("requested %q, want the apply page", tr.gotURL)
		}
	}
}

// A page that parsed to nothing is not an application that asks nothing — Lever renders
// every form with at least a name and an email, so an empty parse means the markup moved.
// Storing it would state the opposite of what is true.
func TestLeverFetcherRefusesAPageItCouldNotParse(t *testing.T) {
	tr := &fakeTransport{body: "<html><body><p>Nothing here</p></body></html>"}

	if _, err := Fetchers(tr)["lever"].Fetch(context.Background(), Claimed{ExternalID: "acme:abc"}); err == nil {
		t.Error("Fetch() = nil error for a page with no form")
	}
}

func TestLeverFetcherMarksAGonePostingAsGone(t *testing.T) {
	tr := &fakeTransport{err: statusErr{code: 404}}

	if _, err := Fetchers(tr)["lever"].Fetch(context.Background(), Claimed{ExternalID: "acme:gone"}); !errors.Is(err, ErrPostingGone) {
		t.Errorf("Fetch() error = %v, want it to mark the posting gone", err)
	}
}
