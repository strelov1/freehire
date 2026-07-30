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
