package sources

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
)

func TestBeehireProvider(t *testing.T) {
	if got := NewBeehire(nil).Provider(); got != "beehire" {
		t.Errorf("Provider() = %q, want beehire", got)
	}
}

func TestBeehireRegisteredAsFullBoardListing(t *testing.T) {
	if !FullBoardListingProviders(All(nil))["beehire"] {
		t.Error("FullBoardListingProviders(All(nil)) should include beehire")
	}
	if !EmptyBoardListingProviders(All(nil))["beehire"] {
		t.Error("EmptyBoardListingProviders(All(nil)) should include beehire")
	}
}

func TestBeehireFetchListsPostings(t *testing.T) {
	fake := (&routedHTTP{}).route("/users/getPublicCampaigns/actemium", `{
		"campaigns": [{
			"_id": "6ae467ada76492faea85cbcd",
			"inviteLink": "https://app.beehire.com/invite/CGSzB_UaR",
			"language": 2,
			"title": {"2": "E&I Project Engineer"},
			"description": {"2": "<p>Build the install.</p>"},
			"location": {"name": "2910 Essen, Belgium", "city": "Essen", "country": "Belgium"},
			"details": {"contract": {
				"duration": "contractDuration_fullTime",
				"type": "contractType_permanent",
				"remote": "remoteWork_partial"
			}}
		}, {
			"_id": "69b9625cba2f31bcd114cac3",
			"inviteKey": "-r5U9zp-C",
			"language": 1,
			"title": {"0": "Business Developer Junior", "1": "Développeur"},
			"description": {"1": "Sell the platform.\n\nCall customers."},
			"location": {"name": "Bruxelles, Belgique", "country": "Belgique"},
			"details": {"contract": {
				"duration": "contractDuration_partTime",
				"type": "contractType_internship",
				"remote": "remoteWork_none"
			}}
		}]
	}`)
	jobs, err := NewBeehire(fake).Fetch(context.Background(), CompanyEntry{
		Company: "ACTEMIUM", Provider: "beehire", Board: "Actemium",
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 2 {
		t.Fatalf("len = %d, want 2", len(jobs))
	}
	eng := jobs[0]
	if eng.ExternalID != "6ae467ada76492faea85cbcd" || eng.Title != "E&I Project Engineer" || eng.Company != "ACTEMIUM" {
		t.Errorf("engineer = %+v", eng)
	}
	if eng.URL != "https://app.beehire.com/invite/CGSzB_UaR" {
		t.Errorf("URL = %q", eng.URL)
	}
	if eng.EmploymentType != "full_time" || eng.WorkMode != "hybrid" || len(eng.Countries) != 1 || eng.Countries[0] != "be" {
		t.Errorf("facets = %s %s %v", eng.EmploymentType, eng.WorkMode, eng.Countries)
	}
	if !strings.Contains(eng.Description, "<p>Build the install.</p>") {
		t.Errorf("Description = %q", eng.Description)
	}
	sales := jobs[1]
	if sales.Title != "Développeur" || sales.URL != "https://app.beehire.com/invite/-r5U9zp-C" {
		t.Errorf("sales = %q %q", sales.Title, sales.URL)
	}
	if sales.EmploymentType != "internship" || sales.WorkMode != "onsite" || sales.Countries[0] != "be" {
		t.Errorf("sales facets = %s %s %v", sales.EmploymentType, sales.WorkMode, sales.Countries)
	}
	if !strings.Contains(sales.Description, "<p>Sell the platform.</p>") || !strings.Contains(sales.Description, "<p>Call customers.</p>") {
		t.Errorf("Description = %q, want the plain text rebuilt as paragraphs", sales.Description)
	}
}

func TestBeehireFetchEmptyBoard(t *testing.T) {
	fake := (&routedHTTP{}).route("/users/getPublicCampaigns/qplus-consult", `{"campaigns":[]}`)
	jobs, err := NewBeehire(fake).Fetch(context.Background(), CompanyEntry{Board: "qplus-consult"})
	if err != nil || len(jobs) != 0 {
		t.Fatalf("jobs=%v err=%v, want an empty board", jobs, err)
	}
}

func TestBeehireFetchGoneBoard(t *testing.T) {
	fake := (&routedHTTP{}).routeErr("beehire.com", &StatusError{Code: http.StatusNotFound})
	_, err := NewBeehire(fake).Fetch(context.Background(), CompanyEntry{Board: "missing"})
	if !errors.Is(err, ErrBoardGone) {
		t.Fatalf("err = %v, want ErrBoardGone", err)
	}
}

func TestBeehireFetchRejectsAMissingCollection(t *testing.T) {
	fake := (&routedHTTP{}).route("/users/getPublicCampaigns/actemium", `{"user":{}}`)
	if _, err := NewBeehire(fake).Fetch(context.Background(), CompanyEntry{Board: "actemium"}); err == nil {
		t.Fatal("Fetch succeeded without a campaigns collection")
	}
}

func TestBeehireFetchRejectsADuplicateID(t *testing.T) {
	fake := (&routedHTTP{}).route("/users/getPublicCampaigns/actemium", `{
		"campaigns": [
			{"_id": "same", "inviteLink": "https://app.beehire.com/invite/a", "title": {"0": "Engineer"}},
			{"_id": "same", "inviteLink": "https://app.beehire.com/invite/a", "title": {"0": "Engineer"}}
		]
	}`)
	if _, err := NewBeehire(fake).Fetch(context.Background(), CompanyEntry{Board: "actemium"}); err == nil {
		t.Fatal("Fetch succeeded with fewer distinct postings than the source listed")
	}
}

func TestBeehireFetchRejectsABadLabel(t *testing.T) {
	if _, err := NewBeehire(&routedHTTP{}).Fetch(context.Background(), CompanyEntry{Board: "../x"}); err == nil {
		t.Fatal("Fetch accepted a board label that is not a slug")
	}
}
