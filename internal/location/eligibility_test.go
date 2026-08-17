package location

import (
	"slices"
	"strings"
	"testing"

	"github.com/strelov1/freehire/internal/vocab"
)

func TestEligibilityFromDescription(t *testing.T) {
	tests := []struct {
		name          string
		desc          string
		wantCountries []string
		wantRegions   []string
	}{
		// Positives — hard, US-specific eligibility statements.
		{"citizen and clearance", "Must be a U.S. Citizen and eligible for a U.S. SECRET clearance.", []string{"us"}, []string{"north_america"}},
		{"us citizen", "This role requires a US Citizen.", []string{"us"}, []string{"north_america"}},
		{"united states citizen", "Applicants must be United States citizens.", []string{"us"}, []string{"north_america"}},
		{"us citizenship", "US citizenship is required for this position.", []string{"us"}, []string{"north_america"}},
		{"us citizenship dotted", "U.S. citizenship required.", []string{"us"}, []string{"north_america"}},
		{"secret clearance", "Candidates must hold an active Secret clearance.", []string{"us"}, []string{"north_america"}},
		{"top secret via substring", "An active Top Secret clearance is mandatory.", []string{"us"}, []string{"north_america"}},
		{"ts sci", "Requires a current TS/SCI with polygraph.", []string{"us"}, []string{"north_america"}},

		// Trap negatives — incidental tokens that must NOT trigger a match.
		{"join us", "Join us! We are hiring engineers worldwide.", nil, nil},
		{"corporate citizen", "We strive to be a good corporate citizen.", nil, nil},
		{"global citizen", "We welcome every global citizen to apply.", nil, nil},
		{"trade secret", "You will help protect our trade secrets.", nil, nil},
		{"security engineer", "We are hiring an Application Security Engineer.", nil, nil},
		{"generic security clearance", "A UK SC security clearance is a plus.", nil, nil},
		{"worldwide", "Open to candidates anywhere in the world.", nil, nil},
		{"empty", "", nil, nil},

		// Negated mentions — the phrase is present, but the sentence denies it.
		{"does not require citizenship", "This role does not require US citizenship; applicants worldwide are welcome.", nil, nil},
		{"no clearance required", "No Secret clearance is required for this position.", nil, nil},
		{"non-us citizens welcome", "We welcome non-US citizens to apply for this fully remote role.", nil, nil},
		{"cannot sponsor but no citizenship needed", "We cannot sponsor visas, and US citizenship is not required.", nil, nil},

		// A denial elsewhere must not hide a genuine assertion in a later sentence.
		{"negation in an earlier unrelated sentence", "This is not a contractor role. US citizenship is required.", []string{"us"}, []string{"north_america"}},

		// Visa sponsorship is not a geography statement — it says the employer will not
		// file paperwork, not that only locals may apply. It belongs to the
		// `visa_sponsorship` facet and must never pin a region here.
		{"no sponsorship is not a restriction", "We are unable to offer visa sponsorship for this role.", nil, nil},

		// Work-authorization phrasings, the largest gap the catalogue sweep found.
		{"authorized to work in the us", "Candidates must be authorized to work in the United States without sponsorship.", []string{"us"}, []string{"north_america"}},
		{"legally authorized variant", "Applicants must be legally authorized to work in the United States.", []string{"us"}, []string{"north_america"}},
		{"authorization noun form", "This role requires authorization to work in the United States.", []string{"us"}, []string{"north_america"}},

		// UK.
		{"right to work uk", "You must have the right to work in the UK.", []string{"gb"}, []string{"uk"}},
		{"right to work united kingdom", "Applicants need the right to work in the United Kingdom.", []string{"gb"}, []string{"uk"}},
		{"british citizen", "This post is open to British citizens only.", []string{"gb"}, []string{"uk"}},

		// EU — region only, deliberately no country.
		{"right to work eu", "You must have the right to work in the EU.", nil, []string{"eu"}},
		{"eu work permit", "A valid EU work permit is required.", nil, []string{"eu"}},

		// Australia — both noun forms, since whole-word matching stopped one covering
		// the other (see TestCitizenPhrasesCarryTheirCitizenshipForm).
		{"australian citizen", "Australian citizens only; NV1 clearance required.", []string{"au"}, []string{"apac"}},
		{"australian citizenship", "Eligibility: Australian citizenship and a security clearance.", []string{"au"}, []string{"apac"}},

		// Canada.
		{"eligible to work in canada", "You must be eligible to work in Canada.", []string{"ca"}, []string{"north_america"}},

		// Two areas named together are unioned, not resolved by priority.
		{"us or canada", "You must be authorized to work in the United States. Canadian citizens are also welcome.", []string{"ca", "us"}, []string{"north_america"}},

		// Equal-opportunity boilerplate names "citizenship" among protected traits but
		// never a nationality, so it must not pin anything. This exact shape appears
		// verbatim in production descriptions.
		{"eeo boilerplate", "We do not discriminate on the basis of religion, sex, national origin, age, citizenship, marital status, or disability.", nil, nil},
		{"regardless of citizenship", "We welcome applicants regardless of citizenship or background.", nil, nil},

		// "without sponsorship" strengthens the requirement; "without" must not read as a
		// denial there. The long form is quoted from a real production description.
		{"without the need for sponsorship", "Candidates must be authorized to work in the United States without the need for current or future sponsorship.", []string{"us"}, []string{"north_america"}},
		{"without visa sponsorship", "Must be authorized to work in the United States without visa sponsorship.", []string{"us"}, []string{"north_america"}},

		// ...but "without" still denies when it genuinely negates the requirement.
		{"without still negates elsewhere", "You may join this team without US citizenship.", nil, nil},

		// Word boundaries. "right to work in the uk" is a prefix of "...in the Ukraine",
		// and matching inside that word would file a Ukrainian posting under the UK.
		{"uk does not match ukraine", "You must have the right to work in the Ukraine.", nil, nil},
		{"uk still matches its own sentence", "You must have the right to work in the UK.", []string{"gb"}, []string{"uk"}},

		// A trailing plural "s" is part of the word and still matches; anything longer
		// is a different word and does not.
		{"plural citizens matches", "Open to Australian citizens only.", []string{"au"}, []string{"apac"}},
		{"citizenship matches its own entry, not the citizen prefix", "US citizenship is required.", []string{"us"}, []string{"north_america"}},
		{"spelled-out citizenship has its own entry too", "United States citizenship is required.", []string{"us"}, []string{"north_america"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotCountries, gotRegions := EligibilityFromDescription(tt.desc)
			if !slices.Equal(gotCountries, tt.wantCountries) {
				t.Errorf("countries = %v, want %v", gotCountries, tt.wantCountries)
			}
			if !slices.Equal(gotRegions, tt.wantRegions) {
				t.Errorf("regions = %v, want %v", gotRegions, tt.wantRegions)
			}
		})
	}
}

// TestCitizenPhrasesCarryTheirCitizenshipForm guards the trap that whole-word matching
// introduced: "australian citizen" used to cover "Australian citizenship" by substring,
// and once matching became whole-word every such pair had to be spelled out. The pairs
// were then added one review finding at a time — "united states citizenship" was missed
// twice — so the invariant is asserted instead of remembered.
func TestCitizenPhrasesCarryTheirCitizenshipForm(t *testing.T) {
	for _, rule := range eligibilityRules {
		for _, p := range rule.phrases {
			if !strings.HasSuffix(p, " citizen") {
				continue
			}
			want := p + "ship"
			if !slices.Contains(rule.phrases, want) {
				t.Errorf("phrase %q has no %q alongside it; whole-word matching will miss the noun form", p, want)
			}
		}
	}
}

// TestEligibilityRulesPinARegion guards the one invariant every rule must hold: a rule
// exists to move a posting OUT of the global bucket, so a rule with phrases but no
// region would match and change nothing. Countries stay optional — the EU rule is
// deliberately region-only.
func TestEligibilityRulesPinARegion(t *testing.T) {
	for _, rule := range eligibilityRules {
		if len(rule.phrases) == 0 {
			continue
		}
		if len(rule.regions) == 0 {
			t.Errorf("rule with phrases %v pins no region", rule.phrases)
		}
	}
}

// TestEligibilityRegionsAreVocabulary keeps the rules honest against the shared region
// vocabulary: a typo'd region here would produce a facet value nothing can filter on,
// and the failure would be silent in production.
func TestEligibilityRegionsAreVocabulary(t *testing.T) {
	for _, rule := range eligibilityRules {
		for _, r := range rule.regions {
			if !slices.Contains(vocab.RegionValues, r) {
				t.Errorf("rule region %q is not in vocab.RegionValues", r)
			}
		}
	}
}
