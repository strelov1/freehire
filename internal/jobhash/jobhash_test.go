package jobhash

import (
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/db"
)

// sample is a fully populated set of write params to fingerprint. Each test mutates
// one field off this baseline and asserts the hash does or does not move.
func sample() db.UpsertJobParams {
	return db.UpsertJobParams{
		Source:             "greenhouse",
		ExternalID:         "cookunity:6619930003",
		URL:                "https://example.com/jobs/1",
		Title:              "Staff Full Stack Engineer",
		Company:            "CookUnity",
		CompanySlug:        "cookunity",
		Location:           "Latam (Remote)",
		Remote:             true,
		Description:        "Build smart fridges.",
		PostedAt:           pgtype.Timestamptz{Time: time.Unix(1_700_000_000, 0).UTC(), Valid: true},
		PublicSlug:         "staff-full-stack-engineer-cookunity-abcd",
		Countries:          []string{"ar", "br"},
		Regions:            []string{"latam"},
		WorkMode:           "remote",
		Skills:             []string{"go", "php"},
		Seniority:          "staff",
		Category:           "engineering",
		PostingLanguage:    "en",
		EmploymentType:     "full_time",
		EducationLevel:     "bachelor",
		ExperienceYearsMin: pgtype.Int4{Int32: 5, Valid: true},
	}
}

func TestOf_StableForEqualContent(t *testing.T) {
	h := Of(sample())
	if h == "" {
		t.Fatal("hash is empty")
	}
	if again := Of(sample()); again != h {
		t.Fatalf("hash not stable: %q != %q", h, again)
	}
}

func TestOf_ChangesWhenAnyIndexedFieldChanges(t *testing.T) {
	base := Of(sample())
	cases := map[string]func(*db.UpsertJobParams){
		"url":          func(p *db.UpsertJobParams) { p.URL = "https://example.com/jobs/2" },
		"title":        func(p *db.UpsertJobParams) { p.Title = "Senior Engineer" },
		"company":      func(p *db.UpsertJobParams) { p.Company = "Acme" },
		"company_slug": func(p *db.UpsertJobParams) { p.CompanySlug = "acme" },
		"location":     func(p *db.UpsertJobParams) { p.Location = "Remote - USA" },
		"remote":       func(p *db.UpsertJobParams) { p.Remote = false },
		"description":  func(p *db.UpsertJobParams) { p.Description = "Different." },
		"posted_at": func(p *db.UpsertJobParams) {
			p.PostedAt = pgtype.Timestamptz{Time: time.Unix(1_700_000_001, 0).UTC(), Valid: true}
		},
		"posted_at_null":       func(p *db.UpsertJobParams) { p.PostedAt = pgtype.Timestamptz{} },
		"public_slug":          func(p *db.UpsertJobParams) { p.PublicSlug = "other-slug" },
		"countries":            func(p *db.UpsertJobParams) { p.Countries = []string{"ar"} },
		"regions":              func(p *db.UpsertJobParams) { p.Regions = []string{"eu"} },
		"work_mode":            func(p *db.UpsertJobParams) { p.WorkMode = "onsite" },
		"skills":               func(p *db.UpsertJobParams) { p.Skills = []string{"go"} },
		"seniority":            func(p *db.UpsertJobParams) { p.Seniority = "senior" },
		"category":             func(p *db.UpsertJobParams) { p.Category = "design" },
		"posting_language":     func(p *db.UpsertJobParams) { p.PostingLanguage = "ru" },
		"employment_type":      func(p *db.UpsertJobParams) { p.EmploymentType = "contract" },
		"education_level":      func(p *db.UpsertJobParams) { p.EducationLevel = "master" },
		"experience_years_min": func(p *db.UpsertJobParams) { p.ExperienceYearsMin = pgtype.Int4{Int32: 3, Valid: true} },
		"experience_null":      func(p *db.UpsertJobParams) { p.ExperienceYearsMin = pgtype.Int4{} },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			p := sample()
			mutate(&p)
			if got := Of(p); got == base {
				t.Errorf("hash unchanged after mutating %s (collision)", name)
			}
		})
	}
}

// Identity columns are not content: the same row keeps its (source, external_id),
// so they must not influence the change signal.
func TestOf_IgnoresIdentityFields(t *testing.T) {
	base := Of(sample())
	for name, mutate := range map[string]func(*db.UpsertJobParams){
		"source":      func(p *db.UpsertJobParams) { p.Source = "lever" },
		"external_id": func(p *db.UpsertJobParams) { p.ExternalID = "x:1" },
	} {
		t.Run(name, func(t *testing.T) {
			p := sample()
			mutate(&p)
			if got := Of(p); got != base {
				t.Errorf("hash changed after mutating identity field %s", name)
			}
		})
	}
}

// A field-boundary guard: concatenation must not let content shift across fields
// produce the same hash (e.g. title "ab"+company "c" vs title "a"+company "bc").
func TestOf_FieldsAreDelimited(t *testing.T) {
	a := sample()
	a.Title, a.Company = "ab", "c"
	b := sample()
	b.Title, b.Company = "a", "bc"
	if Of(a) == Of(b) {
		t.Error("field boundary not delimited: content shifted across fields collides")
	}
}

// fullRow is a db.Job with every hashed field set to something distinctive, so a mapping that
// drops a field is visible as a hash that does not move when that field does.
func fullRow() db.Job {
	return db.Job{
		URL:                "https://example.com/j/1",
		Title:              "Backend Engineer",
		Company:            "Acme",
		CompanySlug:        "acme",
		Location:           "Berlin",
		Remote:             true,
		Description:        "the stored description",
		PostedAt:           pgtype.Timestamptz{Time: time.Unix(1700000000, 0).UTC(), Valid: true},
		PublicSlug:         "backend-engineer-acme-abc123",
		Countries:          []string{"de"},
		Regions:            []string{"eu"},
		WorkMode:           "remote",
		Skills:             []string{"go"},
		Seniority:          "senior",
		Category:           "backend",
		PostingLanguage:    "en",
		EmploymentType:     "full_time",
		EducationLevel:     "bachelor",
		ExperienceYearsMin: pgtype.Int4{Int32: 5, Valid: true},
	}
}

// TestOfRow_CarriesEveryFieldTheHashReads is the guard the duplication used to need a reviewer
// for. Of names the fields the fingerprint covers; OfRow names where each comes from on a stored
// row. They are one decision in two halves, so a field added to Of and forgotten in OfRow leaves
// a backfill computing the OLD fingerprint — the rewritten rows then carry a hash the ingest path
// will not reproduce, and the next crawl of each reports `changed` once for nothing.
//
// Each case mutates ONE field of a fully-populated row. If OfRow does not carry that field, the
// hash cannot move, and the case fails naming the field.
func TestOfRow_CarriesEveryFieldTheHashReads(t *testing.T) {
	base := OfRow(fullRow(), "the stored description")

	mutations := map[string]func(*db.Job){
		"url":                  func(j *db.Job) { j.URL = "https://example.com/j/2" },
		"title":                func(j *db.Job) { j.Title = "Frontend Engineer" },
		"company":              func(j *db.Job) { j.Company = "Globex" },
		"company_slug":         func(j *db.Job) { j.CompanySlug = "globex" },
		"location":             func(j *db.Job) { j.Location = "Munich" },
		"remote":               func(j *db.Job) { j.Remote = false },
		"posted_at":            func(j *db.Job) { j.PostedAt = pgtype.Timestamptz{Time: time.Unix(1800000000, 0).UTC(), Valid: true} },
		"public_slug":          func(j *db.Job) { j.PublicSlug = "backend-engineer-acme-zzz999" },
		"countries":            func(j *db.Job) { j.Countries = []string{"fr"} },
		"regions":              func(j *db.Job) { j.Regions = []string{"latam"} },
		"work_mode":            func(j *db.Job) { j.WorkMode = "onsite" },
		"skills":               func(j *db.Job) { j.Skills = []string{"rust"} },
		"seniority":            func(j *db.Job) { j.Seniority = "junior" },
		"category":             func(j *db.Job) { j.Category = "frontend" },
		"posting_language":     func(j *db.Job) { j.PostingLanguage = "de" },
		"employment_type":      func(j *db.Job) { j.EmploymentType = "contract" },
		"education_level":      func(j *db.Job) { j.EducationLevel = "master" },
		"experience_years_min": func(j *db.Job) { j.ExperienceYearsMin = pgtype.Int4{Int32: 9, Valid: true} },
	}

	for field, mutate := range mutations {
		t.Run(field, func(t *testing.T) {
			row := fullRow()
			mutate(&row)
			if OfRow(row, "the stored description") == base {
				t.Errorf("changing %s left the fingerprint unchanged — OfRow does not carry it, "+
					"so a backfill would rewrite rows with a hash ingest will not reproduce", field)
			}
		})
	}

	// The description is OfRow's separate argument, not read off the row, because replacing it
	// is what a backfill is for.
	if OfRow(fullRow(), "a different body") == base {
		t.Error("changing the description left the fingerprint unchanged")
	}
	if OfRow(fullRow(), "the stored description") != base {
		t.Error("OfRow is not deterministic for the same input")
	}
}
