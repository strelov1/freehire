package linksource

import (
	"context"
	"net/url"
	"strings"
	"testing"
)

// doverDetailJSON mirrors the posting detail endpoint as the live API answered it for the
// submitted Deepspatial link 2026-11-05: the client identity fields ride the same response.
const doverDetailJSON = `{
	"id":"a5546acf-45c3-44ff-bedd-a4314fd0f37d",
	"title":"Deepspatial - Full Stack Developer - Node.js/React.js (2-3 yrs)",
	"user_provided_description":"<p>Requirement: 2-3 years of experience.</p>",
	"locations":[],
	"workplace_type":"HYBRID",
	"compensation":{"employment_type":"FULL_TIME","open_to_sharing_comp":false},
	"visa_support":false,
	"active":true,
	"is_private":false,
	"client_id":"1aeeba55-65e0-4803-b70f-fac7ca8a20c7",
	"client_name":"Deepspatial",
	"client_domain":"deepspatial.ai"
}`

// TestDoverResolvesCompanyFromClientRecord pins the acceptance case behind the review
// findings: the submitted jobs.dover.com link resolves under the company's own identity —
// employer name from the posting's client record (NOT a humanized link segment), external id
// namespaced by the link's Org spelling — so a link-imported row and a crawl of the board the
// contribution flow files this link under dedup to one posting.
func TestDoverResolvesCompanyFromClientRecord(t *testing.T) {
	const link = "https://jobs.dover.com/Deepspatial/a5546acf-45c3-44ff-bedd-a4314fd0f37d?utm_source=telegram"
	c := (&fakeClient{}).
		route("inbound/application-portal-job/a5546acf-45c3-44ff-bedd-a4314fd0f37d", doverDetailJSON, "").
		route("careers-page-slug/Deepspatial", `{"id":"1aeeba55-65e0-4803-b70f-fac7ca8a20c7","slug":"deepspatial","name":"Deepspatial"}`, "")

	job, ok, err := NewDover(c).Resolve(context.Background(), link)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if !ok {
		t.Fatal("ok=false, want the vacancy resolved")
	}
	if job.Company != "Deepspatial" {
		t.Errorf("Company = %q, want the client record's name", job.Company)
	}
	// Namespaced by the LINK's Org spelling — the exact spelling the contribution flow files
	// the board under and the crawl pipeline namespaces rows by (Namespace(board, id)) — so
	// a link-imported row and a crawl of that board dedup to one posting.
	if job.ExternalID != "Deepspatial:a5546acf-45c3-44ff-bedd-a4314fd0f37d" {
		t.Errorf("ExternalID = %q, want org-spelling-namespaced posting uuid", job.ExternalID)
	}
	if job.URL != link {
		t.Errorf("URL = %q, want the submitted link kept", job.URL)
	}
	if !strings.Contains(job.Description, "2-3 years") {
		t.Errorf("Description = %q, want the posting body", job.Description)
	}
}

// TestDoverDeclinesNonJobShapes pins the Match gate: only the two-segment UUID shape — the
// same one the atsboard recognizer accepts — resolves; the marketing shell and the vendor's
// own pages are declined outright, and a matched shape whose posting the API does not know
// is a skip (ok=false), not an error.
func TestDoverDeclinesNonJobShapes(t *testing.T) {
	ls := NewDover(&fakeClient{}) // no routes: any fetch would error the resolve
	declined := []string{
		"https://jobs.dover.com/Deepspatial",
		"https://jobs.dover.com/Deepspatial/not-a-uuid",
		"https://jobs.dover.com/startup-jobs",
		"https://www.jobs.dover.com/Deepspatial/a5546acf-45c3-44ff-bedd-a4314fd0f37d",
	}
	for _, link := range declined {
		u, _ := url.Parse(link)
		if ls.Match(u) {
			t.Errorf("Match accepted %s, want declined", link)
		}
	}
	mixedCase, _ := url.Parse("https://JOBS.Dover.COM/Deepspatial/a5546acf-45c3-44ff-bedd-a4314fd0f37d")
	if !ls.Match(mixedCase) {
		t.Errorf("Match declined %s: hostnames are case-insensitive", mixedCase)
	}
}

// statusErr mirrors sources.StatusError's shape (the adapter matches any error carrying a
// status code, not the concrete type), so the 404-vs-transport branch is testable without a
// real HTTP round trip.
type statusErr struct{ code int }

func (e *statusErr) Error() string   { return "status" }
func (e *statusErr) StatusCode() int { return e.code }

// TestDoverSkipsGoneAndInactivePostings pins the liveness gates: a 404 detail is the
// platform's own answer that the posting is gone (a skip, ok=false, NOT an error the caller
// would retry), an inactive/private posting is likewise a skip, and a transport failure is
// still an error.
func TestDoverSkipsGoneAndInactivePostings(t *testing.T) {
	const link = "https://jobs.dover.com/Deepspatial/a5546acf-45c3-44ff-bedd-a4314fd0f37d"
	const api = "inbound/application-portal-job/a5546acf-45c3-44ff-bedd-a4314fd0f37d"

	gone := &fakeClient{}
	gone.routeErr(api, &statusErr{code: 404})
	if _, ok, err := NewDover(gone).Resolve(context.Background(), link); err != nil || ok {
		t.Errorf("404 detail: ok=%v err=%v, want ok=false, nil error", ok, err)
	}

	inactive := (&fakeClient{}).
		route(api, strings.ReplaceAll(doverDetailJSON, `"active":true`, `"active":false`), "").
		route("careers-page-slug/Deepspatial", `{"slug":"deepspatial"}`, "")
	if _, ok, err := NewDover(inactive).Resolve(context.Background(), link); err != nil || ok {
		t.Errorf("inactive detail: ok=%v err=%v, want ok=false, nil error", ok, err)
	}

	transport := &fakeClient{} // no routes: the fetch fails with a plain error
	if _, ok, err := NewDover(transport).Resolve(context.Background(), link); err == nil || ok {
		t.Errorf("transport failure: ok=%v err=%v, want ok=false, non-nil error", ok, err)
	}
}

// TestDoverDeclinesMalformedUUID pins the uuid gate: a 36-character segment that is not the
// canonical 8-4-4-4-12 hex spelling is not matched, same as the recognizer declines it.
func TestDoverDeclinesMalformedUUID(t *testing.T) {
	ls := NewDover(&fakeClient{})
	for _, link := range []string{
		"https://jobs.dover.com/Deepspatial/zzzzzzzz-45c3-44ff-bedd-a4314fd0f37d",
		"https://jobs.dover.com/Deepspatial/a5546acf45c344ffbedda4314fd0f37d00000000",
		"https://jobs.dover.com/Deepspatial/a5546acf-45c3-44ff-bedd-a4314fd0f37d-x/slug",
	} {
		u, _ := url.Parse(link)
		if ls.Match(u) {
			t.Errorf("Match accepted %s, want declined", link)
		}
	}
}

// TestDoverRejectsEmptyClientIdentityAndUnknownSlug pins the identity gates: an empty
// client_name or an empty slug from the lookup is a skip (never a ":<id>" external id or a
// blank company), and a slug the careers API does not resolve (the shell serves any Org
// spelling) is likewise a skip, not a retriable error.
func TestDoverRejectsEmptyClientIdentityAndUnknownSlug(t *testing.T) {
	const link = "https://jobs.dover.com/Deepspatial/a5546acf-45c3-44ff-bedd-a4314fd0f37d"
	const api = "inbound/application-portal-job/a5546acf-45c3-44ff-bedd-a4314fd0f37d"

	noName := (&fakeClient{}).
		route(api, strings.ReplaceAll(doverDetailJSON, `"client_name":"Deepspatial"`, `"client_name":""`), "").
		route("careers-page-slug/", `{"slug":""}`, "")
	if _, ok, err := NewDover(noName).Resolve(context.Background(), link); err != nil || ok {
		t.Errorf("empty client_name: ok=%v err=%v, want ok=false, nil error", ok, err)
	}

	noSlug := (&fakeClient{}).
		route(api, doverDetailJSON, "").
		route("careers-page-slug/Deepspatial", `{"id":"1aeeba55-65e0-4803-b70f-fac7ca8a20c7","slug":"","name":"Deepspatial"}`, "")
	if _, ok, err := NewDover(noSlug).Resolve(context.Background(), link); err != nil || ok {
		t.Errorf("empty slug: ok=%v err=%v, want ok=false, nil error", ok, err)
	}

	unknownSlug := &fakeClient{}
	unknownSlug.routeErr("careers-page-slug/", &statusErr{code: 404})
	unknownSlug.route(api, doverDetailJSON, "")
	if _, ok, err := NewDover(unknownSlug).Resolve(context.Background(), link); err != nil || ok {
		t.Errorf("unknown slug: ok=%v err=%v, want ok=false, nil error", ok, err)
	}
}

// TestDoverDeclinesOrgOwnedByAnotherClient pins the ownership gate: an Org that names a real
// company other than the posting's client is a skip. Accepting it would store the posting under
// a board whose crawl never writes it, forking the dedup key.
func TestDoverDeclinesOrgOwnedByAnotherClient(t *testing.T) {
	const link = "https://jobs.dover.com/Acme/a5546acf-45c3-44ff-bedd-a4314fd0f37d"
	c := (&fakeClient{}).
		route("inbound/application-portal-job/a5546acf-45c3-44ff-bedd-a4314fd0f37d", doverDetailJSON, "").
		route("careers-page-slug/Acme", `{"id":"99999999-0000-4000-8000-000000000000","slug":"acme","name":"Acme"}`, "")
	if _, ok, err := NewDover(c).Resolve(context.Background(), link); err != nil || ok {
		t.Errorf("Org of another client: ok=%v err=%v, want ok=false, nil error", ok, err)
	}
}

// TestDoverNamespacesTheAPIPostingID pins the id's source: the API answers any case of the uuid
// with its lower-case id, which is what the crawl stores, so an upper-case link dedups with it.
func TestDoverNamespacesTheAPIPostingID(t *testing.T) {
	const link = "https://jobs.dover.com/Deepspatial/A5546ACF-45C3-44FF-BEDD-A4314FD0F37D"
	c := (&fakeClient{}).
		route("inbound/application-portal-job/A5546ACF-45C3-44FF-BEDD-A4314FD0F37D", doverDetailJSON, "").
		route("careers-page-slug/Deepspatial", `{"id":"1aeeba55-65e0-4803-b70f-fac7ca8a20c7","slug":"deepspatial","name":"Deepspatial"}`, "")
	job, ok, err := NewDover(c).Resolve(context.Background(), link)
	if err != nil || !ok {
		t.Fatalf("Resolve: ok=%v err=%v", ok, err)
	}
	if job.ExternalID != "Deepspatial:a5546acf-45c3-44ff-bedd-a4314fd0f37d" {
		t.Errorf("ExternalID = %q, want the API's lower-case posting id", job.ExternalID)
	}
}
