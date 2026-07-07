package jobhash

import (
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/db"
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
