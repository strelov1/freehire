package sources

import (
	"context"
	"encoding/json"
	"testing"
)

// Team is the ATS's own department label: Greenhouse's most specific department, Lever's and
// Ashby's team falling back to their department.
func TestGreenhouseTeam(t *testing.T) {
	for _, tc := range []struct{ name, body, want string }{
		{"single", `{"departments":[{"name":"Engineering","child_ids":[]}]}`, "Engineering"},
		{"leaf beats parent", `{"departments":[{"name":"Engineering","child_ids":[7]},{"name":" Platform ","child_ids":null}]}`, "Platform"},
		{"no leaf takes first", `{"departments":[{"name":"A","child_ids":[1]},{"name":"B","child_ids":[2]}]}`, "A"},
		{"blank name skipped", `{"departments":[{"name":"  "},{"name":"Data"}]}`, "Data"},
		{"none", `{"departments":[]}`, ""},
		{"absent", `{}`, ""},
	} {
		var p GreenhousePosting
		if err := json.Unmarshal([]byte(tc.body), &p); err != nil {
			t.Fatal(err)
		}
		if got := MapGreenhousePosting(p).Team; got != tc.want {
			t.Errorf("%s: Team = %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestAshbyTeam(t *testing.T) {
	for _, tc := range []struct{ name, body, want string }{
		{"team wins", `{"team":" Billing ","department":"Product"}`, "Billing"},
		{"department fallback", `{"team":"","department":"Product"}`, "Product"},
		{"none", `{}`, ""},
	} {
		var p AshbyPosting
		if err := json.Unmarshal([]byte(tc.body), &p); err != nil {
			t.Fatal(err)
		}
		if got := MapAshbyPosting(p).Team; got != tc.want {
			t.Errorf("%s: Team = %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestLeverTeam(t *testing.T) {
	fake := &fakeHTTP{body: `[
		{"id":"1","text":"A","hostedUrl":"u1","categories":{"team":" Growth ","department":"Marketing"}},
		{"id":"2","text":"B","hostedUrl":"u2","categories":{"department":"Sales"}},
		{"id":"3","text":"C","hostedUrl":"u3","categories":{}}
	]`}
	jobs, err := NewLever(fake).Fetch(context.Background(), CompanyEntry{Board: "acme", Company: "Acme"})
	if err != nil {
		t.Fatal(err)
	}
	for i, want := range []string{"Growth", "Sales", ""} {
		if jobs[i].Team != want {
			t.Errorf("job %d: Team = %q, want %q", i, jobs[i].Team, want)
		}
	}
}
