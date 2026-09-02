package main

import "testing"

func techPtr(b bool) *bool { return &b }

// The rule decides what a permanent deletion targets, so every branch is pinned:
// which signal wins, which company state each company-scoped rule needs, and — most
// importantly — everything it must refuse to touch.
func TestMatchRule(t *testing.T) {
	cases := []struct {
		name    string
		c       candidate
		ev      evidence
		crawled bool
		want    string // "" = keep
	}{
		{
			name:    "blue-collar title is removed regardless of the company",
			c:       candidate{CompanySlug: "acme", Title: "Registered Nurse"},
			ev:      evidence{anyTech: true, anySkills: true},
			crawled: true,
			want:    ruleTitle,
		},
		{
			name:    "technical evidence vetoes the title dictionary",
			c:       candidate{CompanySlug: "acme", Title: "DevOps Engineer (HVAC IoT Platform)", IsTech: techPtr(true)},
			ev:      evidence{},
			crawled: true,
			want:    "",
		},
		{
			name: "business role at a company with no technical evidence is removed",
			c:    candidate{CompanySlug: "acme", Title: "Account Manager", Category: "sales", IsTech: techPtr(false)},
			ev:   evidence{},
			want: ruleBusiness,
		},
		{
			// The business rule reads NonTechCategories directly, so engineering_design
			// joining that set opened a hard-delete path the ConfirmedNonTech veto never
			// sees. Draughting is not a business role: an engineering employer whose
			// board was retired would have had its whole catalogue removed.
			name: "engineering design is not a business role",
			c:    candidate{CompanySlug: "acme", Title: "Mechanical Design Engineer", Category: "engineering_design", IsTech: techPtr(false)},
			ev:   evidence{},
			want: "",
		},
		{
			// The second craft category. One category spared by an inline name could
			// not express a set, and this one would have been deletable the moment it
			// joined NonTechCategories — by whoever added it to the vocabulary and
			// never opened cmd/prune.
			name: "industrial engineering is not a business role",
			c:    candidate{CompanySlug: "acme", Title: "Process Engineer", Category: "industrial_engineering", IsTech: techPtr(false)},
			ev:   evidence{},
			want: "",
		},
		{
			name:    "the same business role is kept where the company has posted technical work",
			c:       candidate{CompanySlug: "acme", Title: "Account Manager", Category: "sales", IsTech: techPtr(false)},
			ev:      evidence{anyTech: true},
			crawled: true,
			want:    "",
		},
		{
			name: "unclassified job at a company showing nothing at all is removed",
			c:    candidate{CompanySlug: "acme", Title: "Team Member"},
			ev:   evidence{},
			want: ruleUnknown,
		},
		{
			name:    "tagged skills alone keep an unclassified job",
			c:       candidate{CompanySlug: "acme", Title: "Team Member"},
			ev:      evidence{anySkills: true},
			crawled: true,
			want:    "",
		},
		{
			name:    "an unclassified job is kept wherever the company has any evidence",
			c:       candidate{CompanySlug: "acme", Title: "Team Member"},
			ev:      evidence{anyTech: true},
			crawled: true,
			want:    "",
		},
		{
			name:    "a technical job is never a target",
			c:       candidate{CompanySlug: "acme", Title: "Backend Engineer", Category: "backend", IsTech: techPtr(true)},
			ev:      evidence{},
			crawled: true,
			want:    "",
		},
		{
			name: "a posting from no listed board is never touched, even by the title rule",
			c:    candidate{CompanySlug: "acme", Title: "Registered Nurse"},
			ev:   evidence{},
			want: "",
		},
		{
			name:    "a posting with no company is exempt from the company rules",
			c:       candidate{Title: "Team Member"},
			ev:      evidence{},
			crawled: true,
			want:    "",
		},
		{
			name: "tagged skills do not save a business role — they veto only the unknown rule",
			c:    candidate{CompanySlug: "acme", Title: "Account Manager", Category: "sales", IsTech: techPtr(false)},
			ev:   evidence{anySkills: true},
			want: ruleBusiness,
		},
		{
			name:    "a job placed as non-technical with no category is kept — no rule covers it",
			c:       candidate{CompanySlug: "acme", Title: "Some Role", IsTech: techPtr(false)},
			ev:      evidence{},
			crawled: true,
			want:    "",
		},
		{
			name:    "a still-crawled board blocks the company rules — what they remove would return",
			c:       candidate{CompanySlug: "acme", Title: "Account Manager", Category: "sales", IsTech: techPtr(false)},
			ev:      evidence{},
			crawled: true,
			want:    "",
		},
	}

	// A source with no boards at all satisfies "the board is absent" for free, so the
	// company-scoped rules would fire on it — the first prod dry run planned to remove
	// 2991 Telegram vacancies that way. Nothing re-crawls them, so no rule may apply.
	t.Run("a source that is not a board platform is out of reach of every rule", func(t *testing.T) {
		for _, c := range []candidate{
			{CompanySlug: "acme", Title: "Registered Nurse"},
			{CompanySlug: "acme", Title: "Crane Operator"},
			{CompanySlug: "acme", Title: "Менеджер по продажам", Category: "sales", IsTech: techPtr(false)},
		} {
			if rule, ok := matchRule(c, evidence{}, false, false); ok {
				t.Errorf("matchRule(%q) matched %q, want kept — no crawl restores it", c.Title, rule)
			}
		}
	})

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := matchRule(tc.c, tc.ev, true, tc.crawled)
			if tc.want == "" {
				if ok {
					t.Errorf("matched %q, want kept", got)
				}
				return
			}
			if !ok || got != tc.want {
				t.Errorf("matched %q (ok=%v), want %q", got, ok, tc.want)
			}
		})
	}
}

// The title rule is self-sufficient because ingest applies the same dictionary, but
// the company-scoped rules have no ingest counterpart: the bucket does not exist at
// crawl time. Deleting under them without retiring the board undoes itself within the
// hour, so the worker has to be able to tell the two classes apart.
func TestCompanyScopedRulesAreIdentifiable(t *testing.T) {
	if companyScoped(ruleTitle) {
		t.Error("the title rule is enforced at ingest, so it needs no board retirement")
	}
	for _, rule := range []string{ruleBusiness, ruleUnknown} {
		if !companyScoped(rule) {
			t.Errorf("%q depends on the company bucket and has no ingest counterpart, so it requires board retirement", rule)
		}
	}
}
