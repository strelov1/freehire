package handler

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/job/jobview"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/search/search"
)

// GetCompany returns a company together with a page of its jobs. The jobs must
// go through the same public DTO as the jobs endpoints — the internal numeric id
// must not leak here either. A typed companyDetailResponse whose Jobs field is
// []jobview.Job makes that a compile-time guarantee; this test locks the wire
// contract (no "id", a "public_slug" per job).
func TestCompanyDetailHidesJobID(t *testing.T) {
	views, err := jobview.FromRows([]db.Job{
		{ID: 123, Title: "Go Developer", PublicSlug: "go-developer-acme-t35nijto"},
	})
	if err != nil {
		t.Fatalf("FromRows: %v", err)
	}
	resp := companyDetailResponse{Company: companyViewFrom(db.Company{Slug: "acme", Name: "Acme"}), Jobs: views}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var body struct {
		Jobs []map[string]json.RawMessage `json:"jobs"`
	}
	if err := json.Unmarshal(data, &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(body.Jobs) != 1 {
		t.Fatalf("jobs len = %d, want 1", len(body.Jobs))
	}
	if _, leaked := body.Jobs[0]["id"]; leaked {
		t.Error("company jobs leak the internal numeric id")
	}
	if got := string(body.Jobs[0]["public_slug"]); got != `"go-developer-acme-t35nijto"` {
		t.Errorf("public_slug: want the slug, got %s", got)
	}
}

// The Meilisearch path and the Postgres path serve the same list endpoint, so a
// field present in one and absent in the other makes the catalogue card render
// differently depending on whether a search term was typed. Collections drive the
// backer marks, so the omission would be visible: the marks would disappear the
// moment a user searched.
func TestCompanyListItemFromDocCarriesCollections(t *testing.T) {
	item := companyListItemFromDoc(search.CompanyDocument{
		Slug:        "euro-lab",
		Name:        "Euro Lab",
		Collections: []string{"yc", "a16z-portfolio"},
	})
	if !reflect.DeepEqual(item.Collections, []string{"yc", "a16z-portfolio"}) {
		t.Errorf("collections = %+v, want [yc a16z-portfolio]", item.Collections)
	}
}

// An absent array must serialize as [] like the Postgres '{}', not as null — the
// same normalization industries already gets.
func TestCompanyListItemFromDocNormalizesAbsentCollections(t *testing.T) {
	item := companyListItemFromDoc(search.CompanyDocument{Slug: "x", Name: "X"})
	if item.Collections == nil {
		t.Error("absent collections stayed nil — it will serialize as null, unlike the Postgres path")
	}
}

// The two golden bodies below ARE the /companies contract. They are asserted as bytes rather
// than as Go values because the distinction that would break a client — a null tagline versus an
// empty one — is invisible to a struct comparison, and because the field ORDER is part of what a
// caller receives. Any change here is an API change and must be argued for, not merged as a
// refactor.
const (
	goldenCompanyFull  = `{"slug":"euro-lab","name":"Euro Lab","job_count":12,"tagline":"We ship fridges","industries":["fintech"],"hq_country":"de","collections":["yc"],"feedback_count":0,"feedback_rating_avg":null}`
	goldenCompanyEmpty = `{"slug":"x","name":"X","job_count":0,"tagline":null,"industries":[],"hq_country":null,"collections":[],"feedback_count":0,"feedback_rating_avg":null}`
)

// The endpoint has two backends and one response, so the assertion that matters is that they
// agree byte for byte — a field one branch sets and the other forgets would make the catalogue
// card change shape the moment a user typed a search term.
func TestCompanyListItemJSONIsStableAcrossBothBackends(t *testing.T) {
	fromPostgres := companyListItemFromRow(db.ListCompaniesRow{
		Slug: "euro-lab", Name: "Euro Lab", JobCount: 12,
		Tagline:     pgtype.Text{String: "We ship fridges", Valid: true},
		Industries:  []string{"fintech"},
		HqCountry:   pgtype.Text{String: "de", Valid: true},
		Collections: []string{"yc"},
	})
	fromSearch := companyListItemFromDoc(search.CompanyDocument{
		Slug: "euro-lab", Name: "Euro Lab", JobCount: 12,
		Tagline:     "We ship fridges",
		Industries:  []string{"fintech"},
		HqCountry:   "de",
		Collections: []string{"yc"},
	})

	assertJSON(t, "postgres", fromPostgres, goldenCompanyFull)
	assertJSON(t, "search", fromSearch, goldenCompanyFull)
}

// Absence is the half a struct comparison cannot see: a missing tagline must serialize as null
// and a missing facet array as [], from either backend.
func TestCompanyListItemJSONKeepsAbsenceShapes(t *testing.T) {
	fromPostgres := companyListItemFromRow(db.ListCompaniesRow{Slug: "x", Name: "X", Industries: []string{}, Collections: []string{}})
	fromSearch := companyListItemFromDoc(search.CompanyDocument{Slug: "x", Name: "X"})

	assertJSON(t, "postgres", fromPostgres, goldenCompanyEmpty)
	assertJSON(t, "search", fromSearch, goldenCompanyEmpty)
}

// A populated rating must agree between backends too — CompanyDocument's 0-means-
// absent convention (see its doc comment) is only safe if a real, non-zero average
// actually survives the round trip on both paths.
func TestCompanyListItemJSONCarriesRating(t *testing.T) {
	const golden = `{"slug":"euro-lab","name":"Euro Lab","job_count":0,"tagline":null,"industries":[],"hq_country":null,"collections":[],"feedback_count":3,"feedback_rating_avg":4.5}`
	fromPostgres := companyListItemFromRow(db.ListCompaniesRow{
		Slug: "euro-lab", Name: "Euro Lab", Industries: []string{}, Collections: []string{},
		FeedbackCount: 3, FeedbackRatingAvg: pgtype.Float4{Float32: 4.5, Valid: true},
	})
	fromSearch := companyListItemFromDoc(search.CompanyDocument{
		Slug: "euro-lab", Name: "Euro Lab", FeedbackCount: 3, FeedbackRatingAvg: 4.5,
	})

	assertJSON(t, "postgres", fromPostgres, golden)
	assertJSON(t, "search", fromSearch, golden)
}

func assertJSON(t *testing.T, label string, v any, want string) {
	t.Helper()
	got, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("%s: marshal: %v", label, err)
	}
	if string(got) != want {
		t.Errorf("%s body changed — this is an API change, not a refactor\n got: %s\nwant: %s", label, got, want)
	}
}

// The detail endpoint's golden bodies. GET /companies/:slug had no byte-level pin at all while
// its nullable fields were pgtype, so the shape a client received was decided by pgx's
// MarshalJSON rather than by this file — a pgx major bump could have changed the public
// contract with nothing here failing. These are the same three states the listing pins.
const (
	goldenCompanyDetailFull = `{"slug":"euro-lab","name":"Euro Lab","collections":["yc"],"job_count":12,` +
		`"regions":null,"countries":null,"domains":null,"company_types":null,"company_sizes":null,` +
		`"industries":["fintech"],"year_founded":1999,"employee_count":250,"hq_country":"de",` +
		`"organization_type":"private","tagline":"We ship fridges","company_info":null,` +
		`"remote_regions":null,"yc_batch":null,"yc_status":null,"yc_stage":null,"yc_flags":null,` +
		`"maturity":"growth","upvote_count":3,"downvote_count":1,"my_vote":0,` +
		`"feedback_count":2,"feedback_rating_avg":4.25}`
	goldenCompanyDetailEmpty = `{"slug":"x","name":"X","collections":null,"job_count":0,` +
		`"regions":null,"countries":null,"domains":null,"company_types":null,"company_sizes":null,` +
		`"industries":null,"year_founded":null,"employee_count":null,"hq_country":null,` +
		`"organization_type":null,"tagline":null,"company_info":null,` +
		`"remote_regions":null,"yc_batch":null,"yc_status":null,"yc_stage":null,"yc_flags":null,` +
		`"maturity":null,"upvote_count":0,"downvote_count":0,"my_vote":0,` +
		`"feedback_count":0,"feedback_rating_avg":null}`
)

func TestCompanyViewJSONIsStable(t *testing.T) {
	got := companyViewFrom(db.Company{
		Slug: "euro-lab", Name: "Euro Lab", JobCount: 12,
		Collections:       []string{"yc"},
		Industries:        []string{"fintech"},
		YearFounded:       pgtype.Int4{Int32: 1999, Valid: true},
		EmployeeCount:     pgtype.Int4{Int32: 250, Valid: true},
		HqCountry:         pgtype.Text{String: "de", Valid: true},
		OrganizationType:  pgtype.Text{String: "private", Valid: true},
		Tagline:           pgtype.Text{String: "We ship fridges", Valid: true},
		Maturity:          pgtype.Text{String: "growth", Valid: true},
		UpvoteCount:       3,
		DownvoteCount:     1,
		FeedbackCount:     2,
		FeedbackRatingAvg: pgtype.Float4{Float32: 4.25, Valid: true},
	})
	assertJSON(t, "detail", got, goldenCompanyDetailFull)
}

// Absence is the half a struct comparison cannot see, and the half the listing and the detail
// endpoint used to disagree about: an unset column must serialize as null on both.
func TestCompanyViewJSONKeepsAbsenceShapes(t *testing.T) {
	assertJSON(t, "detail", companyViewFrom(db.Company{Slug: "x", Name: "X"}), goldenCompanyDetailEmpty)
}

// A stored empty string is NOT absence: a company whose tagline was deliberately cleared reads
// as "" and must not collapse into null, which is the one case a naive pointer conversion gets
// wrong.
func TestCompanyViewKeepsAnEmptyStringDistinctFromNull(t *testing.T) {
	got := companyViewFrom(db.Company{Slug: "x", Name: "X", Tagline: pgtype.Text{String: "", Valid: true}})
	if got.Tagline == nil {
		t.Fatal("a stored empty tagline collapsed to null — absence and emptiness are different answers")
	}
	if *got.Tagline != "" {
		t.Errorf("Tagline = %q, want the stored empty string", *got.Tagline)
	}
}
