package classify

import "testing"

// Purchasing titles that are procurement work file under operations, like the buyer and
// purchasing-specialist titles already there. A bare "sourcing specialist" is not one of them:
// it names a recruiter's sourcing as often as a buyer's, so it stays unresolved.
func TestParse_PurchasingRoles(t *testing.T) {
	cases := []struct{ title, want, why string }{
		{"Purchasing Agent", "operations", ""},
		{"Purchasing Analyst", "operations", ""},
		{"Purchasing Coordinator", "operations", ""},
		{"Senior Purchasing Analyst", "operations", "seniority is read separately"},
		{"Purchasing Specialist", "operations", "unchanged"},
		{"Sourcing Specialist", "", "talent sourcing or procurement sourcing: no guess"},
		{"Technical Sourcer", "recruiting", "the recruiter's sourcing keeps its category"},
		{"Data Analyst - Purchasing", "data_analytics", "an earlier, more specific alias wins"},
	}
	for _, c := range cases {
		if got := Parse(c.title).Category; got != c.want {
			t.Errorf("Parse(%q).Category = %q, want %q %s", c.title, got, c.want, c.why)
		}
	}
}
