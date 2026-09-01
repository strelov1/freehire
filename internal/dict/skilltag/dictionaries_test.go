package skilltag

import (
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/strelov1/freehire/internal/dict/vocab"
)

// TestDictionaryInvariants guards the two properties the engine relies on:
// every canonical is a stable slug (lowercase, no spaces), and the vocabulary is
// at least the launch floor so an accidental truncation is caught.
func TestDictionaryInvariants(t *testing.T) {
	for alias, c := range wordAliases {
		assertSlug(t, "wordAliases["+alias+"]", c)
	}
	for _, p := range phraseAliases {
		assertSlug(t, "phraseAliases "+p.alias, p.canonical)
	}
	if got := len(wordAliases) + len(phraseAliases); got < 200 {
		t.Errorf("vocabulary size = %d, want >= 200 (launch floor)", got)
	}
	// Ambiguous English words must never be bare word aliases (they resolve only
	// via an unambiguous alias or a phrase).
	for _, w := range []string{"go", "c", "r"} {
		if _, ok := wordAliases[w]; ok {
			t.Errorf("ambiguous word %q must not be a wordAliases key", w)
		}
	}

	// Acronym canonicals must be valid slugs AND already reachable via an existing
	// alias — an acronym is another route to a known canonical, never a new facet value.
	existing := map[string]bool{}
	for _, c := range wordAliases {
		existing[c] = true
	}
	for _, p := range phraseAliases {
		existing[p.canonical] = true
	}
	for tier, acr := range map[string]map[string]string{"sharedAcronyms": sharedAcronyms, "resumeAcronyms": resumeAcronyms} {
		for surface, c := range acr {
			assertSlug(t, tier+"["+surface+"]", c)
			if !existing[c] {
				t.Errorf("%s[%q] → %q is not an existing canonical (would create a new facet value)", tier, surface, c)
			}
		}
	}

	// categoryScopedAcronyms shares the same invariant as the two tiers above (its
	// canonical must be a valid slug already reachable via an existing alias), plus
	// its own: every allow-listed category must be a real vocab.CategoryValues
	// member, so a typo'd category string can't silently make the tier permanently
	// dead (it would never match any job's resolved category).
	for surface, ca := range categoryScopedAcronyms {
		assertSlug(t, "categoryScopedAcronyms["+surface+"]", ca.canonical)
		if !existing[ca.canonical] {
			t.Errorf("categoryScopedAcronyms[%q] → %q is not an existing canonical (would create a new facet value)", surface, ca.canonical)
		}
		for category := range ca.allowedCategories {
			if !slices.Contains(vocab.CategoryValues, category) {
				t.Errorf("categoryScopedAcronyms[%q] allows category %q, not a member of vocab.CategoryValues", surface, category)
			}
		}
	}
}

func assertSlug(t *testing.T, what, s string) {
	t.Helper()
	if s == "" || s != trimLower(s) {
		t.Errorf("%s: canonical %q is not a lowercase no-space slug", what, s)
	}
}

// The dictionary deliberately covers the NON-engineering roles an IT company hires
// for — recruiting, HR, finance, legal, operations, customer success, sales and support.
// That is right for a skills facet, which describes any posting. It is wrong as
// evidence that a board is a technical employer: cmd/prune's board report treated
// "has any skill" as "has posted something technical", so a recruiting coordinator tagged
// {stakeholder-management, candidate-experience} vouched for the whole board.
//
// HasEngineering is the narrower question that report should have been asking.
// Tech-industry craft that is not software engineering — developer relations,
// technical writing, business analysis, pre-sales — counts as engineering here on
// purpose: it is posted by technical employers, and the conservative error is to keep
// a board, never to retire a live one.
func TestHasEngineering(t *testing.T) {
	cases := []struct {
		name   string
		skills []string
		want   bool
	}{
		{"nothing tagged", nil, false},
		{"pure recruiting", []string{"talent-sourcing", "candidate-experience"}, false},
		{"pure hr", []string{"employee-relations", "performance-management"}, false},
		{"pure finance", []string{"accounts-payable", "financial-reporting"}, false},
		{"pure legal", []string{"contract-negotiation", "regulatory-compliance"}, false},
		{"pure operations", []string{"stakeholder-management", "process-improvement"}, false},
		{"pure customer success", []string{"customer-onboarding", "churn-prevention"}, false},
		{"pure sales", []string{"account-executive", "pipeline-management", "lead-generation"}, false},
		{"pure support", []string{"help-desk", "ticket-resolution"}, false},
		{"pure customer success renewal", []string{"renewal-management"}, false},
		// The bare-word canonicals "seo" and "ecommerce" have no multi-word phrase alias,
		// so they fall outside professionalPhraseAliases; they must still read as
		// non-engineering, same as every other marketing/business discipline above.
		{"pure marketing seo", []string{"seo", "ecommerce"}, false},
		// The exact tag set Parse produces for "SEO Specialist using Ahrefs and SEMrush" —
		// a purely non-technical marketing posting must not be read as having posted
		// "something technical".
		{"seo specialist with tools", []string{"ahrefs", "semrush", "seo"}, false},
		// One engineering canonical is enough — the board has posted something technical.
		{"recruiter who also names a stack", []string{"talent-sourcing", "python"}, true},
		{"plain engineering", []string{"kubernetes"}, true},
		// Tech-industry craft counts, deliberately.
		{"developer relations", []string{"developer-advocacy"}, true},
		{"technical writing", []string{"docs-as-code"}, true},
		{"business analysis", []string{"user-stories"}, true},
		{"pre-sales", []string{"solution-design"}, true},
		// An unknown canonical is not evidence of absence: keep the board.
		{"unknown canonical", []string{"something-new"}, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := HasEngineering(c.skills); got != c.want {
				t.Errorf("HasEngineering(%v) = %v, want %v", c.skills, got, c.want)
			}
		})
	}
}

// Every non-engineering canonical must be one the dictionary actually emits — a
// marker on a canonical no alias resolves to would be dead config, and would hide a
// typo that silently reclassifies nothing.
func TestNonEngineeringCanonicalsAreReal(t *testing.T) {
	emitted := map[string]bool{}
	for _, c := range wordAliases {
		emitted[c] = true
	}
	for _, p := range phraseAliases {
		emitted[p.canonical] = true
	}
	for c := range nonEngineeringCanonicals {
		if !emitted[c] {
			t.Errorf("nonEngineeringCanonicals[%q] is not a canonical any alias resolves to", c)
		}
	}
}

// nonCorroboratingPhrases names canonicals by hand, so it can drift away from the
// phrase list it describes: a renamed canonical would silently start corroborating
// again, and nothing else would fail. This pins the two together.
func TestNonCorroboratingPhrasesAllExist(t *testing.T) {
	declared := make(map[string]bool, len(phraseAliases))
	for _, p := range phraseAliases {
		declared[p.canonical] = true
	}
	for c := range nonCorroboratingPhrases {
		if !declared[c] {
			t.Errorf("nonCorroboratingPhrases names %q, which no phrase alias emits", c)
		}
	}
}

func TestParse_SalesAndSupportVocab(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want []string
	}{
		{
			name: "sales",
			in:   "Account executive responsible for business development, pipeline management, cold outreach, sales enablement, and lead generation",
			want: []string{
				"account-executive",
				"business-development",
				"cold-outreach",
				"lead-generation",
				"pipeline-management",
				"sales-enablement",
			},
		},
		{
			name: "support",
			in:   "Help desk specialist responsible for ticket resolution and service desk operations",
			want: []string{
				"help-desk",
				"service-desk",
				"ticket-resolution",
			},
		},
		{
			name: "customer success renewal",
			in:   "Customer success manager owning renewal management for the book of business",
			want: []string{
				"renewal-management",
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := Parse(c.in); !reflect.DeepEqual(got, c.want) {
				t.Fatalf("Parse(%q) = %v, want %v", c.in, got, c.want)
			}
		})
	}
}

// TestNoNearDuplicateCanonicals guards the one failure a growing dictionary makes
// silently: the same facet value declared twice under different separators. Nothing
// errors when it happens — a posting simply comes back carrying BOTH spellings, the
// facet splits its count between them, and a filter on either shows half the jobs.
//
// It is easy to do because the tiers are independent: a batch can add a phrase whose
// canonical already exists as a word alias written the other way. That is exactly how
// "hugging-face" landed beside "huggingface". Folding the separators out is enough to
// catch it, and cheap enough to run on every canonical in the vocabulary.
func TestNoNearDuplicateCanonicals(t *testing.T) {
	fold := strings.NewReplacer("-", "", "_", "", ".", "")
	seen := map[string]string{}
	check := func(where, canonical string) {
		key := fold.Replace(canonical)
		if prev, ok := seen[key]; ok && prev != canonical {
			t.Errorf("%s: canonical %q duplicates %q — they differ only by separators, so a posting matching both splits the facet", where, canonical, prev)
			return
		}
		seen[key] = canonical
	}
	for alias, c := range wordAliases {
		check("wordAliases["+alias+"]", c)
	}
	for _, p := range phraseAliases {
		check("phraseAliases "+p.alias, p.canonical)
	}
}
