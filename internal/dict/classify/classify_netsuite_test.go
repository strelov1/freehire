package classify

import "testing"

// NetSuite is a vendor platform like Salesforce and SAP: the developer, administrator
// and consultant spellings state a discipline.
func TestParse_NetSuiteRoles(t *testing.T) {
	cases := []struct{ title, want string }{
		{"NetSuite Developer", "software_engineering"},
		{"Senior NetSuite Administrator", "software_engineering"},
		{"NetSuite Consultant", "solutions_engineering"},
	}
	for _, c := range cases {
		if got := Parse(c.title).Category; got != c.want {
			t.Errorf("Parse(%q).Category = %q, want %q", c.title, got, c.want)
		}
	}
}
