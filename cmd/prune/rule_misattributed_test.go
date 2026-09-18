package main

import "testing"

// A row closed as source_misattributed is the one deletion target that needs no judgement
// about the posting at all.
//
// apploi's API stopped honouring ?employer=, so each of 5,833 boards stored the whole global
// catalogue under its own company: 1,565,701 rows for 3,024 real postings, 99.81% duplicates,
// every posting filed under 3,898 employers. The true employer was never written to any
// column, so the attribution cannot be repaired — only deleted. cmd/close-apploi-misattributed
// marked them with their own closed_reason precisely so this could be told apart later from
// an ordinarily-closed row.
//
// The other three rules ask whether a posting BELONGS on an IT board. This one asks nothing
// about the posting: whatever it says, we know it is filed under the wrong company.
func TestMisattributedRowsAreATarget(t *testing.T) {
	c := candidate{
		CompanySlug:  "some-nursing-home",
		Title:        "Registered Nurse",
		ClosedReason: "source_misattributed",
	}

	rule, ok := matchRule(c, evidence{}, true, true)
	if !ok {
		t.Fatal("a row closed as source_misattributed must be a deletion target")
	}
	if rule != ruleMisattributed {
		t.Errorf("rule = %q, want %q", rule, ruleMisattributed)
	}
}

// The rule must not reach a technical posting merely because it was closed for some other
// reason — every other closure is ordinary and its row is still correctly attributed.
func TestOrdinaryClosuresAreNotMisattributed(t *testing.T) {
	for _, reason := range []string{"", "unseen", "board_unreachable", "feed_empty", "self_closed"} {
		c := candidate{
			CompanySlug:  "acme",
			Title:        "Backend Engineer",
			ClosedReason: reason,
		}
		if rule, ok := matchRule(c, evidence{anyTech: true}, true, true); ok && rule == ruleMisattributed {
			t.Errorf("closed_reason %q was treated as misattributed", reason)
		}
	}
}

// The technical veto protects every other rule, and must NOT protect this one: a misattributed
// row is wrong about its employer whatever the posting is about. Among apploi's 1.47M open
// rows, 5,074 were is_tech — and those were the same handful of real postings, copied, under
// companies that had nothing to do with them.
func TestMisattributedBeatsTheTechnicalVeto(t *testing.T) {
	c := candidate{
		CompanySlug:  "acme",
		Title:        "Senior Backend Engineer",
		Category:     "backend",
		ClosedReason: "source_misattributed",
	}

	rule, ok := matchRule(c, evidence{anyTech: true, anySkills: true}, true, true)
	if !ok || rule != ruleMisattributed {
		t.Errorf("rule = %q ok = %v; a misattributed row is wrong about its employer whatever the posting says",
			rule, ok)
	}
}

// It must also survive the board gates. The other rules read boardCrawled in opposite
// directions because they care whether a deletion is recoverable by re-crawling — but a
// misattributed row is one nobody wants back, and apploi's boards are retired, so a rule
// requiring a listed board would never fire at all.
func TestMisattributedIgnoresTheBoardGates(t *testing.T) {
	c := candidate{CompanySlug: "acme", Title: "Dietary Aide", ClosedReason: "source_misattributed"}

	for _, boardCrawled := range []bool{true, false} {
		if rule, ok := matchRule(c, evidence{}, true, boardCrawled); !ok || rule != ruleMisattributed {
			t.Errorf("boardCrawled=%v: rule = %q ok = %v, want %q",
				boardCrawled, rule, ok, ruleMisattributed)
		}
	}
}

// The test that was missing, and the reason the first version of this rule did nothing.
//
// Every case above passed knownProvider=true — the value that makes them pass. Production
// passes FALSE for exactly the rows this rule exists for: knownProvider asks whether the
// source still has live boards, and the campaign that labelled these rows retired all 5,833
// of that provider's in the same step. A full scan on 2026-09-18 refused 1,596,766 rows and
// matched none, because the check sat below that gate.
func TestMisattributedFiresForARetiredProvider(t *testing.T) {
	c := candidate{CompanySlug: "some-nursing-home", Title: "Registered Nurse",
		ClosedReason: "source_misattributed"}

	// knownProvider=false is the real shape: no live boards remain.
	rule, ok := matchRule(c, evidence{}, false, false)
	if !ok {
		t.Fatal("the rule must fire for a retired provider — that is the only shape it will " +
			"ever see, since the rows it targets come from a provider retired for this very reason")
	}
	if rule != ruleMisattributed {
		t.Errorf("rule = %q, want %q", rule, ruleMisattributed)
	}
}

// And the gate must still hold for everything else: an ordinary posting from a source with
// no live boards stays out of reach, because no crawl could restore it.
func TestUnknownProviderStillGatesTheOtherRules(t *testing.T) {
	c := candidate{CompanySlug: "acme", Title: "Retail Sales Merchandiser"}

	if rule, ok := matchRule(c, evidence{}, false, false); ok {
		t.Errorf("rule = %q fired for an unknown provider; only the misattribution rule may", rule)
	}
}
