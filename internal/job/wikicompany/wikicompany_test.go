package wikicompany

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

// stubServer serves a fixed wbsearchentities candidate, a fixed ASK answer, and a
// fixed Wikipedia summary, so Lookup can be exercised end to end against a real HTTP
// server without touching the network. sparqlCalls/summaryCalls let a test assert
// short-circuiting behavior (e.g. a rejected match should never fetch a summary).
type stubServer struct {
	searchID          string
	searchDescription string
	asks              bool
	enwikiTitle       string
	summaryExtract    string

	sparqlCalls  int
	summaryCalls int
}

func (s *stubServer) start(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/w/api.php", func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Query().Get("action") {
		case "wbsearchentities":
			if s.searchID == "" {
				fmt.Fprint(w, `{"search":[]}`)
				return
			}
			fmt.Fprintf(w, `{"search":[{"id":%q,"description":%q}]}`, s.searchID, s.searchDescription)
		case "wbgetentities":
			fmt.Fprintf(w, `{"entities":{%q:{"sitelinks":{"enwiki":{"title":%q}}}}}`, s.searchID, s.enwikiTitle)
		default:
			t.Fatalf("unexpected action: %s", r.URL.Query().Get("action"))
		}
	})
	mux.HandleFunc("/sparql", func(w http.ResponseWriter, r *http.Request) {
		s.sparqlCalls++
		fmt.Fprintf(w, `{"boolean":%v}`, s.asks)
	})
	mux.HandleFunc("/api/rest_v1/page/summary/", func(w http.ResponseWriter, r *http.Request) {
		s.summaryCalls++
		fmt.Fprintf(w, `{"extract":%q}`, s.summaryExtract)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func newTestClient(srv *httptest.Server) *Client {
	c := New(srv.Client())
	c.WikidataAPIURL = srv.URL + "/w/api.php"
	c.SPARQLURL = srv.URL + "/sparql"
	c.WikipediaURL = srv.URL
	return c
}

func TestLookup_ConfidentCompanyMatch(t *testing.T) {
	stub := &stubServer{
		searchID:          "Q123456",
		searchDescription: "Uranium company based in Western Australia",
		asks:              true,
		enwikiTitle:       "Paladin Energy",
		summaryExtract:    "Paladin Energy Ltd is a Western Australian based uranium production company.",
	}
	client := newTestClient(stub.start(t))

	match, err := client.Lookup(context.Background(), "Paladin Energy")
	if err != nil {
		t.Fatalf("Lookup returned error: %v", err)
	}
	if match == nil {
		t.Fatal("expected a match, got nil")
	}
	if match.QID != "Q123456" {
		t.Errorf("QID = %q, want Q123456", match.QID)
	}
	if match.Tagline != "Uranium company based in Western Australia" {
		t.Errorf("Tagline = %q, want the Wikidata description", match.Tagline)
	}
	if match.Summary != "Paladin Energy Ltd is a Western Australian based uranium production company." {
		t.Errorf("Summary = %q, want the Wikipedia extract", match.Summary)
	}
}

func TestLookup_RejectsNonOrganizationMatch(t *testing.T) {
	stub := &stubServer{
		searchID:          "Q999",
		searchDescription: "British television executive",
		asks:              false,
	}
	client := newTestClient(stub.start(t))

	match, err := client.Lookup(context.Background(), "Boardroom Appointments")
	if err != nil {
		t.Fatalf("Lookup returned error: %v", err)
	}
	if match != nil {
		t.Fatalf("expected no match for a non-organization entity, got %+v", match)
	}
	if stub.summaryCalls != 0 {
		t.Errorf("expected no summary fetch for a rejected match, got %d calls", stub.summaryCalls)
	}
}

// TestLookup_AcceptsSubtypeNotInAnchorSet locks in the reason buildOrganizationCheckQuery
// walks wdt:P31/wdt:P279* instead of checking wdt:P31 directly: "defense contractor" is a
// Wikidata subclass of company, not a direct instance of it, and the spike's flat keyword
// heuristic missed exactly this shape of real company (CACI, rosendin). The transitive walk
// itself is exercised on the live graph, not by this stub — buildOrganizationCheckQuery's own
// test asserts the query shape; this test documents that Lookup accepts whatever the
// SPARQL endpoint answers, so a positive answer for a non-anchor subtype still yields a match.
func TestLookup_AcceptsSubtypeNotInAnchorSet(t *testing.T) {
	stub := &stubServer{
		searchID:          "Q5223346",
		searchDescription: "American defense contractor",
		asks:              true,
		enwikiTitle:       "CACI",
		summaryExtract:    "CACI International Inc is an American multinational professional services and information technology company.",
	}
	client := newTestClient(stub.start(t))

	match, err := client.Lookup(context.Background(), "CACI")
	if err != nil {
		t.Fatalf("Lookup returned error: %v", err)
	}
	if match == nil {
		t.Fatal("expected a match for a company subtype outside the anchor set, got nil")
	}
	if match.Tagline != "American defense contractor" {
		t.Errorf("Tagline = %q, want the Wikidata description", match.Tagline)
	}
}

func TestLookup_NoSearchResults(t *testing.T) {
	stub := &stubServer{}
	client := newTestClient(stub.start(t))

	match, err := client.Lookup(context.Background(), "some name with no wikidata entry")
	if err != nil {
		t.Fatalf("Lookup returned error: %v", err)
	}
	if match != nil {
		t.Fatalf("expected no match when search returns nothing, got %+v", match)
	}
	if stub.sparqlCalls != 0 {
		t.Errorf("expected no SPARQL call when there is no candidate, got %d calls", stub.sparqlCalls)
	}
}

// TestLookup_RejectsMalformedEntityID guards against a malformed or unexpectedly
// shaped id ever being interpolated into the SPARQL query text: it must surface as
// an explicit error, not run isOrganization with a corrupted qid.
func TestLookup_RejectsMalformedEntityID(t *testing.T) {
	stub := &stubServer{
		searchID:          "Q123 } VALUES { wd:Q1",
		searchDescription: "not a real Wikidata id",
	}
	client := newTestClient(stub.start(t))

	match, err := client.Lookup(context.Background(), "whatever")
	if err == nil {
		t.Fatal("expected an error for a malformed entity id, got nil")
	}
	if match != nil {
		t.Fatalf("expected no match alongside the error, got %+v", match)
	}
	if stub.sparqlCalls != 0 {
		t.Errorf("expected no SPARQL call with a malformed entity id, got %d calls", stub.sparqlCalls)
	}
}

// TestLookup_NoEnwikiSitelinkSkipsSummaryFetch: a description-only match (the
// entity has no enwiki sitelink) must not attempt the Wikipedia summary request.
func TestLookup_NoEnwikiSitelinkSkipsSummaryFetch(t *testing.T) {
	stub := &stubServer{
		searchID:          "Q1",
		searchDescription: "American defense contractor",
		asks:              true,
		// enwikiTitle left empty: no sitelink.
	}
	client := newTestClient(stub.start(t))

	match, err := client.Lookup(context.Background(), "CACI")
	if err != nil {
		t.Fatalf("Lookup returned error: %v", err)
	}
	if match == nil {
		t.Fatal("expected a match from the description alone")
	}
	if match.Summary != "" {
		t.Errorf("Summary = %q, want empty with no enwiki sitelink", match.Summary)
	}
	if stub.summaryCalls != 0 {
		t.Errorf("expected no Wikipedia summary request with no enwiki sitelink, got %d calls", stub.summaryCalls)
	}
}

// TestLookup_EmptyDescriptionStillMatchesOnSummaryAlone documents a deliberate
// choice, not an accident: Wikidata's own description field is blank for many
// otherwise well-typed entities, and the entity's Wikipedia extract alone is
// still enough to accept the match. The caller (the backfill worker) still marks
// such a company checked — see cmd/backfill-company-info-wikipedia's own tests —
// because a description that doesn't exist today won't appear on a mechanical
// retry; only company_info.summary gets filled from this match, tagline stays
// empty via this source.
func TestLookup_EmptyDescriptionStillMatchesOnSummaryAlone(t *testing.T) {
	stub := &stubServer{
		searchID:          "Q2",
		searchDescription: "", // Wikidata carries no short description for this entity.
		asks:              true,
		enwikiTitle:       "Some Company",
		summaryExtract:    "Some Company is a widget manufacturer founded in 1990.",
	}
	client := newTestClient(stub.start(t))

	match, err := client.Lookup(context.Background(), "Some Company")
	if err != nil {
		t.Fatalf("Lookup returned error: %v", err)
	}
	if match == nil {
		t.Fatal("expected a match from the Wikipedia summary alone")
	}
	if match.Tagline != "" {
		t.Errorf("Tagline = %q, want empty (Wikidata carries no description)", match.Tagline)
	}
	if match.Summary != "Some Company is a widget manufacturer founded in 1990." {
		t.Errorf("Summary = %q, want the Wikipedia extract", match.Summary)
	}
}

// TestLookup_NoDescriptionAndNoSummaryYieldsNoMatch: an organization-typed
// candidate with neither a Wikidata description nor a reachable Wikipedia extract
// carries nothing worth writing, so Lookup reports no match at all rather than an
// empty one.
func TestLookup_NoDescriptionAndNoSummaryYieldsNoMatch(t *testing.T) {
	stub := &stubServer{
		searchID:          "Q3",
		searchDescription: "",
		asks:              true,
		// No enwiki sitelink either, so there is no summary to fall back on.
	}
	client := newTestClient(stub.start(t))

	match, err := client.Lookup(context.Background(), "Some Company")
	if err != nil {
		t.Fatalf("Lookup returned error: %v", err)
	}
	if match != nil {
		t.Fatalf("expected no match with neither a description nor a summary, got %+v", match)
	}
}
