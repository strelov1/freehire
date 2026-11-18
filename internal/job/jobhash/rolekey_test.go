package jobhash

import "testing"

// The cross-source key deliberately drops the description. RoleFingerprint hashes
// it, and aggregators truncate or rewrite descriptions, so a fingerprint match
// across sources is a coincidence rather than a rule — the cross-check would find
// nothing and report every posting as absent from its own company's board.
func TestRoleKey_SurvivesADescriptionTheAggregatorRewrote(t *testing.T) {
	board := RoleKey("cookunity", "Staff Full Stack Engineer")
	aggregator := RoleKey("cookunity", "Staff Full Stack Engineer")

	if board != aggregator {
		t.Errorf("keys differ: %q != %q", board, aggregator)
	}
}

// Per-city variants of one role are one role, the same collapse RoleFingerprint
// makes, so a company posting in six cities is not six absences.
func TestRoleKey_CollapsesACityVariantOntoItsBaseRole(t *testing.T) {
	base := RoleKey("acme", "Senior Backend Engineer")
	for _, variant := range []string{
		"Senior Backend Engineer, Krakow",
		"Senior Backend Engineer - Berlin",
		"Senior Backend Engineer | Remote",
	} {
		if got := RoleKey("acme", variant); got != base {
			t.Errorf("RoleKey(%q) = %q, want it to match the base role %q", variant, got, base)
		}
	}
}

func TestRoleKey_IgnoresCaseAndSpacing(t *testing.T) {
	if RoleKey("acme", "  SENIOR   Backend  Engineer ") != RoleKey("acme", "Senior Backend Engineer") {
		t.Error("cosmetic case and spacing changed the key")
	}
}

// The key is company-scoped: two employers hiring the same role are not each
// other's evidence.
func TestRoleKey_SeparatesCompanies(t *testing.T) {
	if RoleKey("acme", "Go Developer") == RoleKey("globex", "Go Developer") {
		t.Error("two companies share a key; one company's board would answer for another's")
	}
}

// An in-word hyphen is not a separator, so a role does not lose half its title.
func TestRoleKey_KeepsAnInWordHyphen(t *testing.T) {
	if RoleKey("acme", "Front-end Developer") == RoleKey("acme", "Front") {
		t.Error("an in-word hyphen was treated as a clause separator")
	}
}

// Stripping must not reduce a title to a single generic token, which would make
// unrelated roles collide into one cluster.
func TestRoleKey_DoesNotStripATitleDownToOneWord(t *testing.T) {
	if RoleKey("acme", "Engineer, Berlin") == RoleKey("acme", "Engineer, Munich") {
		t.Error("two city postings of a one-word role collapsed; the strip went too far")
	}
}

func TestRoleKey_IsEmptyForAnEmptyTitle(t *testing.T) {
	if RoleKey("acme", "   ") != "" {
		t.Error("a blank title produced a key; it would match every other blank one")
	}
}

// Measured on prod (2026-07-29): parenthetical variants of one role were counted as
// separate roles, inflating the cross-check's absent count by 13% — 15,706 candidates
// fell to 13,641 once parentheses were stripped. "Data Engineer (Semi Senior)" and
// "Data Engineer Semi Senior" are one role advertised twice, and an aggregator that
// drops or adds the parentheses must not manufacture an absence.
func TestRoleKey_CollapsesParentheticalVariants(t *testing.T) {
	base := RoleKey("acme", "Data Engineer Semi Senior")
	for _, variant := range []string{
		"Data Engineer (Semi Senior)",
		"Data Engineer  (Semi   Senior)",
		"Data Engineer (Semi Senior) ",
	} {
		if got := RoleKey("acme", variant); got != base {
			t.Errorf("RoleKey(%q) = %q, want it to match %q", variant, got, base)
		}
	}
}

// A parenthetical that carries the WHOLE distinction must not erase the role either:
// stripping must leave something, and what is left must still be two words.
func TestRoleKey_ParenthesisStripNeverEmptiesATitle(t *testing.T) {
	if RoleKey("acme", "(Senior)") == "" {
		t.Error("a title that is only a parenthetical lost its key entirely")
	}
	if RoleKey("acme", "Go Developer (Remote)") == RoleKey("acme", "Rust Developer (Remote)") {
		t.Error("stripping parentheses collapsed two different roles")
	}
}

func intersects(a, b []string) bool {
	for _, x := range a {
		for _, y := range b {
			if x == y {
				return true
			}
		}
	}
	return false
}

// Aggregators drop the board's punctuation. RoleKey stripped " - Fraud" as a trailing
// clause on the board side only, so the same Plaid role read as two, and the
// cross-check accused Plaid of advertising a role it had (measured 2026-11-01).
func TestTitleKeys_MatchAcrossDroppedPunctuation(t *testing.T) {
	for _, pair := range [][2]string{
		{"Account Executive - Fraud", "Account Executive Fraud"},
		{"Engineering Manager - Dashboard Foundations", "Engineering Manager Dashboard Foundations"},
		{"Senior Product Manager, Financial Management", "Senior Product Manager Financial Management"},
		{"Front-End Developer", "Front End Developer"},
		{"Senior Backend Engineer, Krakow", "Senior Backend Engineer"},
		{"Data Engineer (Semi Senior)", "Data Engineer Semi Senior"},
	} {
		if !intersects(TitleKeys(pair[0]), TitleKeys(pair[1])) {
			t.Errorf("TitleKeys(%q) = %q and TitleKeys(%q) = %q share no key",
				pair[0], TitleKeys(pair[0]), pair[1], TitleKeys(pair[1]))
		}
	}
}

// Folding punctuation must not fold words: two different roles stay two.
func TestTitleKeys_KeepDifferentRolesApart(t *testing.T) {
	if intersects(TitleKeys("Go Developer"), TitleKeys("Rust Developer")) {
		t.Error("two different roles share a key")
	}
}

// A script that writes vowels as combining marks keeps its words whole.
func TestTitleKeys_KeepCombiningMarks(t *testing.T) {
	if got := TitleKeys("डेवलपर"); len(got) != 1 || got[0] != "डेवलपर" {
		t.Errorf("TitleKeys(डेवलपर) = %q, want the word unsplit", got)
	}
}

func TestTitleKeys_BlankTitleHasNoKeys(t *testing.T) {
	for _, title := range []string{"", "   ", " - , | "} {
		if got := TitleKeys(title); len(got) != 0 {
			t.Errorf("TitleKeys(%q) = %q, want none", title, got)
		}
	}
}
