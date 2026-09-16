package classify

import (
	"sort"
	"strings"
	"testing"
)

// The two ways this catalogue names the same craft. "Python Developer" and "Python
// Engineer" are one job posted by two employers with different habits, and a title that
// resolves under one spelling must resolve under the other.
//
// The check runs through Parse, not over categoryTable's entries: a stem often resolves
// through a DIFFERENT entry than its own (there is no "backend engineer" alias, and
// "Backend Engineer" resolves to backend all the same), so comparing aliases reports 175
// pairs that are not actually broken. What matters is the answer, not the row.
const (
	devSuffix           = " developer"
	engSuffix           = " engineer"
	softwareEngineering = "software_engineering"
)

// unpairedStems are the stems that deliberately answer differently — or only once —
// under the two spellings, each with the reason. Short on purpose: a long list would mean
// the pairing rule is wrong rather than that these titles are special.
var unpairedStems = map[string]string{
	// Sales. "Business Developer" develops the BUSINESS; there is no software in it.
	"business": "a sales role under `developer`; the noun is the business, not a product",
	// One of the genuinely different pairs. "Application Developer" writes software;
	// "Applications Engineer" is the industrial pre-sales/field role that supports a
	// manufacturer's product line. Same word, two trades.
	"application": "application developer writes software; applications engineer is the industrial field role",
	// Property, not software, under `developer`.
	"real estate": "property development",
	"property":    "property development",
	"land":        "property development",
	// "Software Development Engineer" is Amazon's house name for a software engineer.
	// "Software Development Developer" is not a title anybody has ever posted.
	"software development": "the engineer spelling is a house style; the developer one says developer twice",
}

// pairStems returns every stem categoryTable spells with either suffix.
func pairStems() []string {
	seen := map[string]bool{}
	for _, e := range categoryTable {
		switch {
		case strings.HasSuffix(e.alias, devSuffix):
			seen[strings.TrimSuffix(e.alias, devSuffix)] = true
		case strings.HasSuffix(e.alias, engSuffix):
			seen[strings.TrimSuffix(e.alias, engSuffix)] = true
		}
	}
	out := make([]string, 0, len(seen))
	for stem := range seen {
		out = append(out, stem)
	}
	sort.Strings(out)
	return out
}

func TestBothSpellingsOfACraftResolveTheSame(t *testing.T) {
	// This test exists because of what the gap costs. A title the dictionary cannot read
	// gets no category AND no is_tech; EnqueuePendingJobs gates enrichment on
	// `is_tech IS TRUE`, so the LLM never sees it and never supplies the missing
	// category; and search.CategoryUnresolved then hides it forever rather than until
	// the next enrichment cycle. Measured 2026-09-16: "senior java engineer" alone was
	// 233 open postings on the wrong side of that loop.
	//
	// Deriving the stems from the table is the point — a test written from its own hand
	// list would have passed while 15 languages had a `developer` entry and no
	// `engineer` one, which is exactly how this survived.
	var broken []string
	for _, stem := range pairStems() {
		if _, exempt := unpairedStems[stem]; exempt {
			continue
		}
		dev := Parse(stem + devSuffix).Category
		eng := Parse(stem + engSuffix).Category
		// Scoped to the stems one side already calls SOFTWARE ENGINEERING, and there for
		// two reasons.
		//
		// It must not demand the same answer on both sides: "Database Developer" is
		// software engineering and "Database Engineer" is devops, and both are right —
		// one system, two teams. What must never happen is one side resolving to
		// NOTHING, because nothing is the value that takes a posting out of search
		// permanently.
		//
		// And it must not demand a pair for every stem: an unscoped version reports 128
		// gaps, of which most are titles nobody posts — "Commissioning Developer",
		// "Geotechnical Developer", "Environmental Developer". A software technology is
		// different: "Python Developer" and "Python Engineer" are both ordinary, and an
		// employer's choice between them is a habit, not a job description.
		if dev != softwareEngineering && eng != softwareEngineering {
			continue
		}
		switch {
		case dev == "":
			broken = append(broken, stem+devSuffix+" = nothing, while "+stem+engSuffix+" = "+quote(eng))
		case eng == "":
			broken = append(broken, stem+engSuffix+" = nothing, while "+stem+devSuffix+" = "+quote(dev))
		}
	}
	if len(broken) > 0 {
		t.Errorf("%d craft(s) resolve under one noun and vanish under the other.\n"+
			"Add the missing alias, or name the stem in unpairedStems with the reason:\n  %s",
			len(broken), strings.Join(broken, "\n  "))
	}
}

func quote(s string) string {
	if s == "" {
		return "nothing"
	}
	return `"` + s + `"`
}

func TestEngineeringResolvesTheSameCraftAsEngineer(t *testing.T) {
	// Whole-word matching means "engineer" does not occur inside "engineering", so
	// "Software Engineering Intern" resolved to nothing while "Software Engineer Intern"
	// resolved fine — one letter apart, and 192 open postings on the wrong side of it.
	tests := []struct{ title, want string }{
		{"Software Engineering Intern", "software_engineering"},
		{"Software Engineer Intern", "software_engineering"},
		{"Software Engineering Lead", "software_engineering"},
		{"Software Engineering Co-op", "software_engineering"},
		// The manager spelling is a management role and stays one: "engineering manager"
		// sits earlier in the table and must keep winning.
		{"Software Engineering Manager", "management"},
	}
	for _, tt := range tests {
		if got := Parse(tt.title).Category; got != tt.want {
			t.Errorf("Parse(%q).Category = %q, want %q", tt.title, got, tt.want)
		}
	}
}

func TestLanguageTitlesResolveUnderBothSpellings(t *testing.T) {
	for _, lang := range []string{
		"Python", "Java", "JavaScript", "TypeScript", ".NET", "PHP", "Ruby", "C#", "C++",
		"Node.js", "ABAP",
	} {
		for _, suffix := range []string{" Developer", " Engineer"} {
			title := lang + suffix
			if got := Parse(title).Category; got == "" {
				t.Errorf("Parse(%q).Category is empty; a language name is not an ambiguous craft", title)
			}
		}
	}
}

func TestAIAndMLResolveUnderBothSpellings(t *testing.T) {
	for _, title := range []string{"AI Engineer", "AI Developer", "ML Engineer", "ML Developer"} {
		if got := Parse(title).Category; got == "" {
			t.Errorf("Parse(%q).Category is empty", title)
		}
	}
}
