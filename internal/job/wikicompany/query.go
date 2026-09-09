package wikicompany

import (
	"fmt"
	"strings"
)

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

// buildOrganizationCheckQuery returns a SPARQL ASK query testing whether qid is a
// company/organization: it walks the subclass hierarchy (wdt:P31/wdt:P279*) rather
// than checking wdt:P31 directly, so a subtype the anchor set doesn't name outright
// (e.g. "defense contractor", a subclass of company) still matches.
func buildOrganizationCheckQuery(qid string) string {
	anchors := make([]string, len(organizationAnchorQIDs))
	for i, a := range organizationAnchorQIDs {
		anchors[i] = "wd:" + a
	}
	return fmt.Sprintf(
		"ASK { wd:%s wdt:P31/wdt:P279* ?type . VALUES ?type { %s } }",
		qid, strings.Join(anchors, " "),
	)
}
