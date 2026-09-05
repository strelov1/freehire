package jobhash

import (
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/platform/db"
)

func TestRoleFingerprint_StableForEqualContent(t *testing.T) {
	h := RoleFingerprint(sample())
	if h == "" {
		t.Fatal("fingerprint is empty")
	}
	if again := RoleFingerprint(sample()); again != h {
		t.Fatalf("fingerprint not stable: %q != %q", h, again)
	}
}

// The fingerprint is the repost IDENTITY: a role reposted under a new external_id
// with a refreshed posted date (and new url/slug) must resolve to one fingerprint,
// so it deliberately ignores every volatile field.
func TestRoleFingerprint_IgnoresVolatileFields(t *testing.T) {
	base := RoleFingerprint(sample())
	cases := map[string]func(*db.UpsertJobParams){
		"posted_at": func(p *db.UpsertJobParams) {
			p.PostedAt = pgtype.Timestamptz{Time: time.Unix(1_800_000_000, 0).UTC(), Valid: true}
		},
		"posted_at_null": func(p *db.UpsertJobParams) { p.PostedAt = pgtype.Timestamptz{} },
		"url":            func(p *db.UpsertJobParams) { p.URL = "https://example.com/jobs/999" },
		"public_slug":    func(p *db.UpsertJobParams) { p.PublicSlug = "staff-full-stack-engineer-cookunity-zzzz" },
		"external_id":    func(p *db.UpsertJobParams) { p.ExternalID = "cookunity:9999999999" },
		"source":         func(p *db.UpsertJobParams) { p.Source = "lever" },
		"location":       func(p *db.UpsertJobParams) { p.Location = "Remote - EU" },
		"remote":         func(p *db.UpsertJobParams) { p.Remote = false },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			p := sample()
			mutate(&p)
			if got := RoleFingerprint(p); got != base {
				t.Errorf("fingerprint changed after mutating volatile field %s (should cluster)", name)
			}
		})
	}
}

func TestRoleFingerprint_ChangesWhenRoleChanges(t *testing.T) {
	base := RoleFingerprint(sample())
	cases := map[string]func(*db.UpsertJobParams){
		"company_slug": func(p *db.UpsertJobParams) { p.CompanySlug = "acme" },
		"title":        func(p *db.UpsertJobParams) { p.Title = "Senior Backend Engineer" },
		"description":  func(p *db.UpsertJobParams) { p.Description = "A completely different role." },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			p := sample()
			mutate(&p)
			if got := RoleFingerprint(p); got == base {
				t.Errorf("fingerprint unchanged after mutating role field %s (collision)", name)
			}
		})
	}
}

// Normalization is narrow: case and surrounding/collapsing whitespace do not split a
// role, so a re-post with cosmetic title/description whitespace still clusters.
func TestRoleFingerprint_NormalizesCaseAndWhitespace(t *testing.T) {
	base := RoleFingerprint(sample())
	cases := map[string]func(*db.UpsertJobParams){
		"title_case":       func(p *db.UpsertJobParams) { p.Title = "STAFF FULL STACK ENGINEER" },
		"title_whitespace": func(p *db.UpsertJobParams) { p.Title = "  Staff   Full  Stack   Engineer " },
		"desc_case":        func(p *db.UpsertJobParams) { p.Description = "BUILD SMART FRIDGES." },
		"desc_whitespace":  func(p *db.UpsertJobParams) { p.Description = "Build   smart fridges.  " },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			p := sample()
			mutate(&p)
			if got := RoleFingerprint(p); got != base {
				t.Errorf("fingerprint changed after cosmetic %s (should normalize)", name)
			}
		})
	}
}

// A role posted per-city with the city appended to the title (Personio-style) must
// cluster: the trailing ", <city>" clause is stripped before hashing, so the postings
// share one fingerprint. The description is identical (the real over-merge guard).
func TestRoleFingerprint_CollapsesCitySuffix(t *testing.T) {
	variants := []string{
		"Senior Fullstack Engineer (m/w/d), Krakau",
		"Senior Fullstack Engineer (m/w/d), Wien",
		"Senior Fullstack Engineer (m/w/d) - München",
		"Senior Fullstack Engineer (m/w/d) | Düsseldorf",
	}
	var base string
	for i, title := range variants {
		p := sample()
		p.Title = title
		p.Location = title // location differs too; must not matter
		got := RoleFingerprint(p)
		if i == 0 {
			base = got
			continue
		}
		if got != base {
			t.Errorf("city-variant %q did not cluster with the first (%q != base)", title, got)
		}
	}
}

// The strip is suffix-only: a leading seniority grade is part of the role identity and
// must never be stripped, so a graded role does not collapse into the ungraded one.
func TestRoleFingerprint_PreservesSeniorityPrefix(t *testing.T) {
	junior := sample()
	junior.Title = "Software Engineer, Berlin"
	senior := sample()
	senior.Title = "Senior Software Engineer, Berlin"
	if RoleFingerprint(junior) == RoleFingerprint(senior) {
		t.Error("seniority prefix collapsed: graded and ungraded roles share a fingerprint")
	}
}

// Guard: stripping must not reduce a title below two words, so a too-generic single
// token cannot become the cluster key (e.g. "Engineer - Backend" / "- Frontend" stay
// distinct even if descriptions were to match). The original title is kept instead.
func TestRoleFingerprint_KeepsTitleWhenStripLeavesTooFewWords(t *testing.T) {
	backend := sample()
	backend.Title = "Engineer - Backend"
	frontend := sample()
	frontend.Title = "Engineer - Frontend"
	if RoleFingerprint(backend) == RoleFingerprint(frontend) {
		t.Error("guard failed: single-word strip merged distinct specialties")
	}
}

// The description remains in the key: two postings with the same stripped title but
// different descriptions (distinct specialties) must NOT collapse.
func TestRoleFingerprint_DifferentDescriptionStaysSeparate(t *testing.T) {
	a := sample()
	a.Title = "Software Engineer, Data Infrastructure, Wien"
	a.Description = "Own the data ingestion pipeline."
	b := sample()
	b.Title = "Software Engineer, Data Infrastructure, Berlin"
	b.Description = "Own the data ingestion pipeline."
	if RoleFingerprint(a) != RoleFingerprint(b) {
		t.Fatal("same role in two cities with identical description should cluster")
	}
	c := sample()
	c.Title = "Software Engineer, Platform, Wien"
	c.Description = "Own the internal developer platform."
	if RoleFingerprint(a) == RoleFingerprint(c) {
		t.Error("different descriptions collapsed: over-merge across specialties")
	}
}

// The fingerprint compares VISIBLE text, not markup: two postings whose rendered
// title and description are identical must share a fingerprint even when their HTML
// differs (a stray tag, a different wrapper, or an entity vs its literal). Descriptions
// are stored as sanitized HTML, so markup churn from a re-post or a second source must
// not split one role.
func TestRoleFingerprint_IgnoresDescriptionMarkup(t *testing.T) {
	base := sample()
	base.Description = "<p>Build <strong>smart</strong> fridges. Q&amp;A included.</p>"
	baseFP := RoleFingerprint(base)
	cases := map[string]string{
		"extra_br":        "<p>Build <strong>smart</strong> fridges. Q&amp;A included.</p><br>",
		"different_wrap":  "<div>Build <b>smart</b> fridges. Q&amp;A included.</div>",
		"entity_vs_liter": "<p>Build <strong>smart</strong> fridges. Q&A included.</p>",
		"no_markup":       "Build smart fridges. Q&A included.",
	}
	for name, desc := range cases {
		t.Run(name, func(t *testing.T) {
			p := sample()
			p.Description = desc
			if got := RoleFingerprint(p); got != baseFP {
				t.Errorf("markup-only difference %s split the fingerprint (should collapse)", name)
			}
		})
	}
}

// Entity decoding applies to the title too, so a source that HTML-encodes an ampersand
// in the title clusters with one that does not.
func TestRoleFingerprint_DecodesEntitiesInTitle(t *testing.T) {
	literal := sample()
	literal.Title = "R&D Platform Engineer"
	encoded := sample()
	encoded.Title = "R&amp;D Platform Engineer"
	if RoleFingerprint(literal) != RoleFingerprint(encoded) {
		t.Error("entity-encoded title did not cluster with its decoded form")
	}
}

// An `&nbsp;` entity folds to a plain space, so a posting that glues words with the
// no-break-space entity clusters with one that uses a regular space.
func TestRoleFingerprint_FoldsNonBreakingSpaceEntity(t *testing.T) {
	nbsp := sample()
	nbsp.Description = "Build&nbsp;smart&nbsp;fridges."
	plain := sample()
	plain.Description = "Build smart fridges."
	if RoleFingerprint(nbsp) != RoleFingerprint(plain) {
		t.Error("&nbsp;-glued description did not cluster with its space-separated form")
	}
}

// Markup that carries no visible text (an empty or tags-only description) normalizes to
// the same empty text, so two such postings with an equal title still share a fingerprint.
func TestRoleFingerprint_EmptyAndTagsOnlyDescriptionCollapse(t *testing.T) {
	empty := sample()
	empty.Description = ""
	tagsOnly := sample()
	tagsOnly.Description = "<p></p><br>"
	if RoleFingerprint(empty) != RoleFingerprint(tagsOnly) {
		t.Error("empty and tags-only descriptions produced different fingerprints")
	}
}

// The over-merge guard survives visible-text normalization: two postings with the same
// markup shape but a real visible-text difference (e.g. a city-specific legal clause in
// one and not the other, like the Austrian Kollektivvertrag case) must stay separate.
func TestRoleFingerprint_VisibleTextDifferenceStaysSeparate(t *testing.T) {
	withClause := sample()
	withClause.Description = "<p>Build smart fridges.</p><p>Kollektivvertrag Wien applies.</p>"
	without := sample()
	without.Description = "<p>Build smart fridges.</p>"
	if RoleFingerprint(withClause) == RoleFingerprint(without) {
		t.Error("distinct visible descriptions collapsed: over-merge across a real text difference")
	}
}

// Field-boundary guard: title/description content must not shift across the boundary
// and collide.
func TestRoleFingerprint_FieldsAreDelimited(t *testing.T) {
	a := sample()
	a.Title, a.Description = "ab", "c"
	b := sample()
	b.Title, b.Description = "a", "bc"
	if RoleFingerprint(a) == RoleFingerprint(b) {
		t.Error("field boundary not delimited: content shifted across fields collides")
	}
}

// TestRoleFingerprint_InputsAreCoveredByTheContentHash is what lets the ingest write path
// skip a row whose content_hash matches: if the hash did not move, the fingerprint cannot
// have moved either, so there is nothing to recompute. Break that and the cheap path serves
// a stale role identity — the row stops clustering with its own reposts, silently, until a
// backfill happens to rewrite it.
//
// The field walk is reflective, not the hand-written table TestOfRow_CarriesEveryFieldTheHashReads
// uses: a field ADDED to UpsertJobParams must be covered the day it appears, and a table would
// simply not list it. A field type mutateField cannot change fails the test rather than being
// skipped — an unmutated field is an unchecked one that reads as checked.
func TestRoleFingerprint_InputsAreCoveredByTheContentHash(t *testing.T) {
	base := sample()
	baseHash, baseFingerprint := Of(base), RoleFingerprint(base)

	typ := reflect.TypeOf(base)
	for i := range typ.NumField() {
		field := typ.Field(i)
		mutated, ok := mutateField(base, i)
		if !ok {
			t.Fatalf("no mutation for %s of type %s: extend mutateField — an unmutated field "+
				"is an unchecked one that reads as checked", field.Name, field.Type)
		}

		t.Run(field.Name, func(t *testing.T) {
			hashMoved := Of(mutated) != baseHash
			fingerprintMoved := RoleFingerprint(mutated) != baseFingerprint

			// One direction only. The converse — the hash moves while the fingerprint does
			// not — is the normal case for every volatile field Of reads and RoleFingerprint
			// deliberately ignores, and asserting it would contradict
			// TestRoleFingerprint_IgnoresVolatileFields.
			if fingerprintMoved && !hashMoved {
				t.Errorf("changing %s moved the role fingerprint but not the content hash: an "+
					"unchanged re-ingest skips such a row, leaving it clustered under the role "+
					"identity of content it no longer has", field.Name)
			}
		})
	}
}

// mutateField returns a copy of p with exactly field i changed to a value distinct from the
// one it held, reporting false for a type it does not know how to change.
func mutateField(p db.UpsertJobParams, i int) (db.UpsertJobParams, bool) {
	f := reflect.ValueOf(&p).Elem().Field(i)
	switch v := f.Interface().(type) {
	case string:
		f.SetString(v + " mutated")
	case bool:
		f.SetBool(!v)
	case []string:
		f.Set(reflect.ValueOf(append(slices.Clone(v), "mutated")))
	// The jsonb columns. requirements_derived is the only one today: a pure function
	// of description, which both hashes already read, so it moves neither hash — but
	// it must still be MUTATED here, because the guard's whole point is that a field
	// nothing mutates reads as covered when it is not.
	case []byte:
		f.SetBytes(append(slices.Clone(v), []byte(`{"mutated":true}`)...))
	case pgtype.Timestamptz:
		f.Set(reflect.ValueOf(pgtype.Timestamptz{Time: v.Time.Add(time.Hour), Valid: true}))
	case pgtype.Int4:
		f.Set(reflect.ValueOf(pgtype.Int4{Int32: v.Int32 + 1, Valid: true}))
	case pgtype.Bool:
		f.Set(reflect.ValueOf(pgtype.Bool{Bool: !v.Bool, Valid: true}))
	case pgtype.Text:
		f.Set(reflect.ValueOf(pgtype.Text{String: v.String + " mutated", Valid: true}))
	default:
		return p, false
	}
	return p, true
}
