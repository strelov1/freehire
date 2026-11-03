package engagement

import (
	"slices"
	"testing"

	"github.com/strelov1/freehire/internal/dict/vocab"
)

// Every case is a phrase read off a real catalogue posting in the 2026-11-03 sample
// (the 1,354 postings the public search returns for "IR35"), trimmed to the clause
// that decides it.
func TestFromText(t *testing.T) {
	cases := []struct {
		name, title, desc string
		want              []string
	}{
		// Inside.
		{"title", "Java Developer - Inside IR35 - Investment Banking", "", []string{"ir35_inside"}},
		{"rate line", "", "Up to £700/day (Inside IR35)", []string{"ir35_inside"}},
		{"status field", "", "Day rate: £500 per day. IR35 Status: Inside IR35 (Umbrella)", []string{"ir35_inside"}},
		{"bare field value", "", "Location: Filton - 60% onsite IR35: Inside SC Clearance: BPSS", []string{"ir35_inside"}},
		{"field with spaced colon", "", "Remote – occasional travel may be required IR35 : Inside Pay rate to intermediary", []string{"ir35_inside"}},
		{"value before the name", "", "The role is IR35 inside and requires SC clearance", []string{"ir35_inside"}},
		{"within", "", "Staff or contract (within IR35) options available.", []string{"ir35_inside"}},
		{"falls within", "", "The contract will last 3 to 6 months, falls within IR35 and offers a daily rate", []string{"ir35_inside"}},
		{"in-scope", "", "Pay rate: an hourly rate, in-scope IR35, of £14.90 PAYE", []string{"ir35_inside"}},
		{"inside the scope of", "", "The day rate is £350, inside the scope of IR35.", []string{"ir35_inside"}},
		{"inside scope of", "", "The rate is £552 per day inside scope of IR35.", []string{"ir35_inside"}},
		{"glued to umbrella", "", "Rate: up to £538.20 p/d Umbrellainside IR35 Own and evolve our Jenkins", []string{"ir35_inside"}},
		{"spaced and hyphenated name", "", "£450 per day, Inside IR-35. Also inside IR 35 for the extension.", []string{"ir35_inside"}},
		{"next line glued on inside", "", "Hybrid, 2–3 days onsite per weekContract – Inside IR35November 26 Start", []string{"ir35_inside"}},
		{"classification field", "", "Contract classification: Inside IR35 Daily rate breakdown", []string{"ir35_inside"}},
		// Outside.
		{"title outside", "Solution Architect Outside IR35", "", []string{"ir35_outside"}},
		{"outside of", "", "6 month (rolling) contract outside of IR35 £500 to £550 per day", []string{"ir35_outside"}},
		{"misspelt", "", "£500-£550 p/d outsdie of IR35 please call", []string{"ir35_outside"}},
		{"field outside", "", "Rate: up to £570 per day IR35: Outside Duration: initial 6 months", []string{"ir35_outside"}},
		{"status is outside", "", "a day rate between £500 and £600. IR35 status is outside. You will develop", []string{"ir35_outside"}},
		{"value before the name outside", "", "ensure stable releases. IR35 Outside.", []string{"ir35_outside"}},
		{"hashtag", "", "#sourcetopay #architecture #outsideir35 #contractjobs", []string{"ir35_outside"}},
		{"next line glued on", "", "Clojure | React | £550/day Outside IR35We are looking for an engineer", []string{"ir35_outside"}},
		{"b2b outside", "", "Assignment; B2B contract (outside IR35)", []string{"ir35_outside"}},
		// Both stated.
		{"either", "", "The roles can be engaged Inside or Outside IR35 and will suit engineers", []string{"ir35_outside", "ir35_inside"}},
		{"field offers either", "", "Rate: £600/day IR35: Inside or Outside", []string{"ir35_outside", "ir35_inside"}},
		{"field offers either, reversed", "", "IR35 status: Outside / Inside, by agreement", []string{"ir35_outside", "ir35_inside"}},
		{"title and body disagree", "Data Engineer - Outside IR35", "Rate £500 inside IR35 via umbrella", []string{"ir35_outside", "ir35_inside"}},
		// Not stated.
		{"pending", "", "Contract Rate: £350 - £400 per day (LTD - IR35 Determination Pending)", nil},
		{"being confirmed", "", "Day rate and IR35 determination being confirmed.", nil},
		{"exempt client", "", "As our client is considered exempt for IR35 purposes, the successful candidate will be responsible for assessing IR35 status", nil},
		{"hr duty", "IR35 Specialist (London, ENG)", "Conduct and document IR35 status assessments and issue SDSs.", nil},
		{"tax adviser", "Senior Manager (Employment Tax)", "matters (including PAYE, NICs, benefits-in-kind, IR35, global mobility)", nil},
		{"bare mention", "FullStack Infrastructure Engineer", "London - UK Contract - IR35 The Role We are seeking", nil},
		{"denied", "", "Please note this role is not inside IR35.", nil},
		{"denied with an article", "", "This is not an Inside IR35 role.", nil},
		{"denied with to be", "", "This is not to be inside IR35.", nil},
		{"word inside another", "", "Outside IR350 is a part number; inside ir3500 too.", nil},
		{"empty", "", "", nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := FromText(c.title, c.desc); !slices.Equal(got, c.want) {
				t.Errorf("FromText(%q, %q) = %v, want %v", c.title, c.desc, got, c.want)
			}
		})
	}
}

// A field read as visible text: a tag between the name and its value cannot hide it.
func TestFromText_Markup(t *testing.T) {
	got := FromText("", "<ul><li><b>IR35:</b> <i>Outside</i></li></ul>")
	if want := []string{"ir35_outside"}; !slices.Equal(got, want) {
		t.Errorf("FromText = %v, want %v", got, want)
	}
}

// The rules are the vocabulary, in its order, and every value has a label: the SPA's
// chips are generated from both, so a missing rule is a chip that never matches.
func TestRulesCoverVocabulary(t *testing.T) {
	var slugs []string
	for _, r := range rules {
		slugs = append(slugs, r.slug)
	}
	if !slices.Equal(slugs, vocab.EngagementValues) {
		t.Errorf("rules = %v, want vocab.EngagementValues %v", slugs, vocab.EngagementValues)
	}
	labels := Labels()
	for _, v := range vocab.EngagementValues {
		if labels[v] == "" {
			t.Errorf("no label for %q", v)
		}
	}
}
