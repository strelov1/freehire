package jobhash

import (
	"testing"

	"github.com/strelov1/freehire/internal/platform/db"
)

func TestNormalizedRoleTitle_MatchesAcrossCosmeticVariation(t *testing.T) {
	cases := []struct {
		name string
		a, b string
	}{
		{"case", "Senior Backend Engineer", "senior backend engineer"},
		{"whitespace", "Senior   Backend Engineer", "Senior Backend Engineer"},
		{"trailing city clause", "Senior Backend Engineer, Krakow", "Senior Backend Engineer"},
		{"trailing dash clause", "Senior Backend Engineer - Remote", "Senior Backend Engineer"},
		{"html markup", "<b>Senior</b> Backend Engineer", "Senior Backend Engineer"},
		{"html entity", "Senior Backend Engineer &amp; Lead", "Senior Backend Engineer & Lead"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, want := NormalizedRoleTitle(c.a), NormalizedRoleTitle(c.b)
			if got != want {
				t.Errorf("NormalizedRoleTitle(%q) = %q, NormalizedRoleTitle(%q) = %q, want equal", c.a, got, c.b, want)
			}
		})
	}
}

func TestNormalizedRoleTitle_DiffersForDifferentRoles(t *testing.T) {
	a := NormalizedRoleTitle("Senior Backend Engineer")
	b := NormalizedRoleTitle("Senior Frontend Engineer")
	if a == b {
		t.Errorf("NormalizedRoleTitle collapsed two different roles to %q", a)
	}
}

// The whole point of this function over RoleFingerprint: two different companies'
// postings of the same role must normalize identically, since RoleFingerprint folds
// the company slug into its hash and would never let that happen — this is the
// exact confusion the recentfeed grouping key design initially got wrong (see
// openspec/changes/add-homepage-recent-jobs-feed/design.md).
func TestNormalizedRoleTitle_UnlikeRoleFingerprint_IgnoresCompanyIdentity(t *testing.T) {
	acme := db.UpsertJobParams{CompanySlug: "acme", Title: "Senior Backend Engineer", Description: "Build things."}
	globex := db.UpsertJobParams{CompanySlug: "globex", Title: "Senior Backend Engineer", Description: "Build things."}

	if RoleFingerprint(acme) == RoleFingerprint(globex) {
		t.Fatal("RoleFingerprint should differ across companies for the same role — it is scoped per company")
	}
	if got, want := NormalizedRoleTitle(acme.Title), NormalizedRoleTitle(globex.Title); got != want {
		t.Fatalf("NormalizedRoleTitle should ignore company identity, unlike RoleFingerprint: %q != %q", got, want)
	}
}
