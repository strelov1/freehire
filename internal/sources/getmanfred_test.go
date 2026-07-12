package sources

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// getmanfredHTTP is a route-aware test JSONGetter. getmanfred has two endpoints: a single list
// (/api/v2/public/offers) and a per-offer detail (/api/v2/public/offers/{id}). This fake serves
// the list body for the list URL and routes detail requests by the id in the path, recording the
// detail ids so a test can assert a detail is fetched only for ACTIVE offers.
type getmanfredHTTP struct {
	list       string
	details    map[int]string
	detailErr  map[int]bool
	failList   bool
	gotDetails []int
}

var getmanfredDetailRE = regexp.MustCompile(`/offers/(\d+)`)

func (f *getmanfredHTTP) GetJSON(_ context.Context, url string, v any) error {
	if m := getmanfredDetailRE.FindStringSubmatch(url); m != nil {
		id, _ := strconv.Atoi(m[1])
		f.gotDetails = append(f.gotDetails, id)
		if f.detailErr[id] {
			return errors.New("getmanfredHTTP: detail boom")
		}
		raw, ok := f.details[id]
		if !ok {
			return errors.New("getmanfredHTTP: no detail for id")
		}
		return json.Unmarshal([]byte(raw), v)
	}
	if f.failList {
		return errors.New("getmanfredHTTP: list boom")
	}
	return json.Unmarshal([]byte(f.list), v)
}

func TestGetmanfredFetch(t *testing.T) {
	list := `[
	  {"id":8407,"slug":"mango-sr-designer","position":"Sr. Product Designer","status":"ACTIVE",
	   "locations":["Barcelona, España"],"remotePercentage":50,"updatedAt":"2026-07-11T10:00:43.372Z",
	   "company":{"name":"Mango"}},
	  {"id":9001,"slug":"acme-backend","position":"Backend Engineer","status":"ACTIVE",
	   "locations":[],"remotePercentage":100,"updatedAt":"2026-07-10T08:00:00.000Z",
	   "company":{"name":"Acme"}},
	  {"id":7000,"slug":"old-role","position":"Closed Role","status":"CLOSED",
	   "locations":["Madrid, España"],"remotePercentage":0,"updatedAt":"2026-01-01T00:00:00.000Z",
	   "company":{"name":"Gone"}}
	]`
	details := map[int]string{
		8407: `{"id":8407,"introduction":"Intro text.","responsibilities":["Design flows","Ship UI"],
		        "whatOffering":"Nice perks.","techs":[{"name":"Figma"},{"name":"NotARealSkill"}]}`,
		9001: `{"id":9001,"introduction":"Backend intro.","techs":[{"name":"Go"},{"name":"PostgreSQL"}]}`,
	}
	http := &getmanfredHTTP{list: list, details: details}

	jobs, err := getmanfred{http: http}.Fetch(context.Background(), CompanyEntry{})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 2 {
		t.Fatalf("want 2 ACTIVE jobs, got %d", len(jobs))
	}
	// CLOSED offer 7000 must never be fetched for detail.
	if slices.Contains(http.gotDetails, 7000) {
		t.Fatalf("detail fetched for CLOSED offer: %v", http.gotDetails)
	}

	mango := jobs[0]
	if mango.ExternalID != "8407" || mango.Company != "Mango" || mango.Title != "Sr. Product Designer" {
		t.Fatalf("identity mismatch: %+v", mango)
	}
	if mango.URL != "https://www.getmanfred.com/ofertas-empleo/8407/mango-sr-designer" {
		t.Fatalf("url: %q", mango.URL)
	}
	if mango.Location != "Barcelona, España" {
		t.Fatalf("location: %q", mango.Location)
	}
	if mango.WorkMode != "hybrid" || mango.Remote {
		t.Fatalf("50%% remote should be hybrid/non-remote: mode=%q remote=%v", mango.WorkMode, mango.Remote)
	}
	if mango.PostedAt == nil {
		t.Fatalf("posted_at not parsed")
	}
	if !strings.Contains(mango.Description, "Intro text") || !strings.Contains(mango.Description, "Design flows") {
		t.Fatalf("description missing sections: %q", mango.Description)
	}
	// skilltag drops the noise token, keeps the real tech.
	if !slices.Contains(mango.Skills, "figma") || slices.Contains(mango.Skills, "NotARealSkill") {
		t.Fatalf("skills: %v", mango.Skills)
	}

	acme := jobs[1]
	if acme.WorkMode != "remote" || !acme.Remote {
		t.Fatalf("100%% remote should be remote: mode=%q remote=%v", acme.WorkMode, acme.Remote)
	}
	if acme.Location != "" {
		t.Fatalf("empty locations should yield empty location, got %q", acme.Location)
	}
}

// A failed detail must fall back to an empty body, never drop the offer.
func TestGetmanfredDetailFailureKeepsOffer(t *testing.T) {
	http := &getmanfredHTTP{
		list: `[{"id":1,"slug":"s","position":"Dev","status":"ACTIVE","remotePercentage":0,
		         "company":{"name":"Co"}}]`,
		detailErr: map[int]bool{1: true},
	}
	jobs, err := getmanfred{http: http}.Fetch(context.Background(), CompanyEntry{})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 || jobs[0].Description != "" || jobs[0].WorkMode != "onsite" {
		t.Fatalf("offer should survive a failed detail: %+v", jobs)
	}
}

// A failed list is a board-level error.
func TestGetmanfredListFailure(t *testing.T) {
	http := &getmanfredHTTP{failList: true}
	if _, err := (getmanfred{http: http}).Fetch(context.Background(), CompanyEntry{}); err == nil {
		t.Fatal("want error on list failure")
	}
}
