package wikicompany

import (
	"fmt"
	"regexp"
	"strings"
)

// qidPattern matches a Wikidata entity ID: an uppercase Q followed by digits, and
// nothing else. wbsearchentities is expected to only ever return values of this
// shape, but validating before a QID is interpolated into a SPARQL query string
// turns a malformed or unexpectedly-shaped response into an explicit error instead
// of silently building a corrupted (or, worst case, injected) query.
var qidPattern = regexp.MustCompile(`^Q[0-9]+$`)

// isValidQID reports whether qid has the shape of a genuine Wikidata entity ID.
func isValidQID(qid string) bool { return qidPattern.MatchString(qid) }

// organizationAnchorQIDs are the curated Wikidata classes a candidate must descend
// from (via wdt:P31/wdt:P279*) to be accepted as a company/organization match:
// business (Q4830453), organization (Q43229), enterprise (Q6881511), company
// (Q783794), public company (Q891723), corporation (Q328664).
//
// Reviewed against the spike's confirmed-good sample before rollout; see
// design.md ("Decisions" #2) for why a transitive walk is used instead of a flat
// P31 check or a keyword scan of the description.
var organizationAnchorQIDs = []string{
	"Q4830453", // business
	"Q43229",   // organization
	"Q6881511", // enterprise
	"Q783794",  // company
	"Q891723",  // public company
	"Q328664",  // corporation
}

// nonOrganizationAnchorQIDs are classes that must NOT be reachable from a candidate
// (via wdt:P31/wdt:P279*) even when it also reaches an organization anchor:
// geographic location (Q2221906), administrative territorial entity (Q56061).
//
// Found in production, not in the spike: "Nissan" resolved to Q270195, a commune
// in Hérault, France. A commune's class descends from "administrative territorial
// entity", which Wikidata's multi-parent class hierarchy also places somewhere
// under "organization" — so the positive walk alone accepted a place sharing a
// name with the company it was meant to describe. Both anchors were confirmed
// live: Q270195 reaches Q56061/Q2221906, and every one of the spike's confirmed
// company matches (Paladin Energy, Hitachi Energy, Royal Bank of Canada, CACI)
// reaches neither.
var nonOrganizationAnchorQIDs = []string{
	"Q2221906", // geographic location
	"Q56061",   // administrative territorial entity
}

// buildOrganizationCheckQuery returns a SPARQL ASK query testing whether qid is a
// company/organization: it walks the subclass hierarchy (wdt:P31/wdt:P279*) rather
// than checking wdt:P31 directly, so a subtype the anchor set doesn't name outright
// (e.g. "defense contractor", a subclass of company) still matches — and it
// requires the same walk to NOT reach a geographic/administrative anchor, so a
// same-named place is rejected even when it shares an organization ancestor.
func buildOrganizationCheckQuery(qid string) string {
	orgAnchors := make([]string, len(organizationAnchorQIDs))
	for i, a := range organizationAnchorQIDs {
		orgAnchors[i] = "wd:" + a
	}
	excludedAnchors := make([]string, len(nonOrganizationAnchorQIDs))
	for i, a := range nonOrganizationAnchorQIDs {
		excludedAnchors[i] = "wd:" + a
	}
	return fmt.Sprintf(
		"ASK { wd:%s wdt:P31/wdt:P279* ?type . VALUES ?type { %s } "+
			"FILTER NOT EXISTS { wd:%s wdt:P31/wdt:P279* ?excluded . VALUES ?excluded { %s } } }",
		qid, strings.Join(orgAnchors, " "), qid, strings.Join(excludedAnchors, " "),
	)
}
