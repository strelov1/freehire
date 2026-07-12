package sources

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// ijPage wraps an inner __INITIAL_PROPS__ JSON document in the exact double-encoding the real
// pages use: JSON.parse("<the document as a JS string literal>"). json.Marshal of the document
// string produces that literal (quoted, with the inner quotes escaped), so the adapter's decoder
// is exercised end-to-end.
func ijPage(t *testing.T, innerJSON string) string {
	t.Helper()
	lit, err := json.Marshal(innerJSON)
	if err != nil {
		t.Fatalf("marshal literal: %v", err)
	}
	return `<html><head><script>window.__INITIAL_PROPS__ = JSON.parse(` + string(lit) + `);</script></head><body></body></html>`
}

// infojobsHTTP is a route-aware test TextGetter: listing pages keyed by ?page=N, detail pages by
// full URL.
type infojobsHTTP struct {
	listPages map[int]string
	details   map[string]string
	detailErr map[string]bool
	failList  bool
	got       []string
}

var infojobsPageRE = regexp.MustCompile(`[?&]page=(\d+)`)

func (f *infojobsHTTP) GetText(_ context.Context, url string) (string, error) {
	f.got = append(f.got, url)
	if strings.Contains(url, "/ofertas-trabajo/") {
		if f.failList {
			return "", errors.New("infojobsHTTP: list boom")
		}
		page := 1
		if m := infojobsPageRE.FindStringSubmatch(url); m != nil {
			page, _ = strconv.Atoi(m[1])
		}
		if body, ok := f.listPages[page]; ok {
			return body, nil
		}
		return `<html>window.__INITIAL_PROPS__ = JSON.parse("{\"offers\":[]}");</html>`, nil
	}
	if f.detailErr[url] {
		return "", errors.New("infojobsHTTP: detail boom")
	}
	if body, ok := f.details[url]; ok {
		return body, nil
	}
	return "", errors.New("infojobsHTTP: no detail")
}

func TestInfojobsFetchNewHydrates(t *testing.T) {
	// Two offers on page 1; totalElements=2 stops pagination after one page. Offer B's teaser
	// carries a description with an embedded quote and a `");` sequence to exercise the decoder.
	list := `{"overview":{"totalElements":2},"offers":[
	  {"code":"AAA","title":"Backend Engineer","companyName":"Acme","city":"Madrid",
	   "publishedAt":"2026-07-12T08:00:00Z","teleworking":"Presencial","link":"//www.infojobs.net/madrid/backend/of-iAAA?applicationOrigin=search"},
	  {"code":"BBB","title":"Data Scientist","companyName":"Beta","city":"Barcelona",
	   "publishedAt":"2026-07-11T09:00:00Z","teleworking":"Solo teletrabajo","link":"//www.infojobs.net/barcelona/data/of-iBBB?applicationOrigin=search"}
	]}`
	detailBBB := `{"offer":{"description":"Full body. He said \"hi\"); and more text."}}`
	http := &infojobsHTTP{
		listPages: map[int]string{1: ijPage(t, list)},
		details:   map[string]string{"https://www.infojobs.net/barcelona/data/of-iBBB": ijPage(t, detailBBB)},
	}

	// AAA already ingested → seen; BBB new → hydrated.
	seen := func(id string) bool { return id == "AAA" }
	jobs, err := infojobs{http: http}.FetchNew(context.Background(), CompanyEntry{}, seen)
	if err != nil {
		t.Fatalf("FetchNew: %v", err)
	}
	if len(jobs) != 2 {
		t.Fatalf("want 2 jobs, got %d: %+v", len(jobs), jobs)
	}

	byID := map[string]Job{}
	for _, j := range jobs {
		byID[j.ExternalID] = j
	}

	aaa := byID["AAA"]
	if !aaa.SeenRefresh {
		t.Fatalf("seen offer AAA should be a liveness refresh: %+v", aaa)
	}
	if aaa.URL != "https://www.infojobs.net/madrid/backend/of-iAAA" {
		t.Fatalf("AAA url (query must be stripped, scheme restored): %q", aaa.URL)
	}
	if aaa.WorkMode != "onsite" {
		t.Fatalf("Presencial should be onsite, got %q", aaa.WorkMode)
	}

	bbb := byID["BBB"]
	if bbb.SeenRefresh {
		t.Fatalf("new offer BBB must not be a seen-refresh")
	}
	if bbb.WorkMode != "remote" || !bbb.Remote {
		t.Fatalf("Solo teletrabajo should be remote: mode=%q remote=%v", bbb.WorkMode, bbb.Remote)
	}
	if !strings.Contains(bbb.Description, "Full body") || !strings.Contains(bbb.Description, "more text") {
		t.Fatalf("BBB should carry the full hydrated body: %q", bbb.Description)
	}
	if bbb.PostedAt == nil {
		t.Fatalf("BBB posted_at not parsed")
	}
}

// A failed detail falls back to the listing teaser, never dropping the offer.
func TestInfojobsDetailFailureKeepsTeaser(t *testing.T) {
	list := `{"overview":{"totalElements":1},"offers":[
	  {"code":"X","title":"Dev","companyName":"Co","city":"Vigo","teleworking":"Híbrido",
	   "description":"Teaser body.","link":"//www.infojobs.net/vigo/dev/of-iX?x=1"}]}`
	http := &infojobsHTTP{
		listPages: map[int]string{1: ijPage(t, list)},
		detailErr: map[string]bool{"https://www.infojobs.net/vigo/dev/of-iX": true},
	}
	jobs, err := infojobs{http: http}.FetchNew(context.Background(), CompanyEntry{}, func(string) bool { return false })
	if err != nil {
		t.Fatalf("FetchNew: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("want 1 job, got %d", len(jobs))
	}
	if jobs[0].WorkMode != "hybrid" {
		t.Fatalf("Híbrido should be hybrid, got %q", jobs[0].WorkMode)
	}
	if !strings.Contains(jobs[0].Description, "Teaser body") {
		t.Fatalf("should fall back to the teaser: %q", jobs[0].Description)
	}
}

// Fetch (list-only) yields the teaser description without any detail request.
func TestInfojobsFetchListOnly(t *testing.T) {
	list := `{"overview":{"totalElements":1},"offers":[
	  {"code":"Y","title":"QA","companyName":"Co","city":"Sevilla","teleworking":"",
	   "description":"Just the teaser.","link":"//www.infojobs.net/sevilla/qa/of-iY"}]}`
	http := &infojobsHTTP{listPages: map[int]string{1: ijPage(t, list)}}
	jobs, err := infojobs{http: http}.Fetch(context.Background(), CompanyEntry{})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 || !strings.Contains(jobs[0].Description, "Just the teaser") {
		t.Fatalf("list-only teaser expected: %+v", jobs)
	}
	if jobs[0].WorkMode != "" {
		t.Fatalf("empty teleworking should yield empty work mode, got %q", jobs[0].WorkMode)
	}
	// No detail URL should have been requested.
	for _, u := range http.got {
		if !strings.Contains(u, "/ofertas-trabajo/") {
			t.Fatalf("unexpected non-listing request: %q", u)
		}
	}
}

// A first-page listing failure is a board-level error.
func TestInfojobsListFailure(t *testing.T) {
	http := &infojobsHTTP{failList: true}
	if _, err := (infojobs{http: http}).Fetch(context.Background(), CompanyEntry{}); err == nil {
		t.Fatal("want error on list failure")
	}
}

// The double-encoded __INITIAL_PROPS__ decodes even when the payload contains escaped quotes and
// a `");` sequence that would defeat a lazy regex.
func TestInfojobsDecodeProps(t *testing.T) {
	inner := `{"offer":{"description":"a \"quote\" then ); and \"end\");"}}`
	page := ijPage(t, inner)
	var props infojobsDetailProps
	if err := infojobsDecodeProps(page, &props); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if props.Offer.Description != `a "quote" then ); and "end");` {
		t.Fatalf("decoded description mismatch: %q", props.Offer.Description)
	}
}
