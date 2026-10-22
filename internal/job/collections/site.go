package collections

import (
	"fmt"
	"strconv"
	"strings"

	"golang.org/x/net/publicsuffix"

	"github.com/strelov1/freehire/internal/job/logodomain"
)

// MetaWebsite is the Record.Meta key a source fills with the member's own website,
// when it publishes one. Members reads it to refuse a name match whose company
// demonstrably runs a different site.
const MetaWebsite = "website"

// MetaTeamSize is the Record.Meta key a source fills with the member's headcount,
// when it publishes one (the YC directory does).
const MetaTeamSize = "team_size"

// mismatch says why a name-matched company is not the member the record describes,
// or "" when nothing the source published contradicts the match.
func mismatch(r Record, company Company) string {
	if !sameSite(r.Meta[MetaWebsite], company.Website) {
		return fmt.Sprintf("%s ≠ %s", r.Meta[MetaWebsite], company.Website)
	}
	if !plausibleSize(r.Meta[MetaTeamSize], company.OpenJobs) {
		return fmt.Sprintf("team of %s, %d open jobs", r.Meta[MetaTeamSize], company.OpenJobs)
	}
	return ""
}

// Headcount bounds on how many postings a member can plausibly hold open at once.
// A company rarely has more open roles than a few times its staff. The floor is there
// because the directory's headcounts go stale — it lists ShipBob as a team of one — so
// below it a small mismatch is more likely an old number than another company, while
// every false match the rule exists for held hundreds of postings.
const (
	maxOpenJobsPerHead = 3
	openJobsFloor      = 50
)

// plausibleSize reports whether a member of the given published headcount can be the
// catalogue company holding openJobs postings. Measured on the YC directory: every
// false match leading the YC tag on 2026-10-22 was a large same-named employer —
// Pennant (a team of 2) against the Pennant Group's 693 nursing postings, Arrow (18)
// against Arrow Electronics' 534 — at 8x to 350x its team, while the busiest real
// member hired at 2.3x (Finni Health, 91 postings for 40 people). An unknown
// headcount says nothing and refuses nothing.
func plausibleSize(teamSize string, openJobs int) bool {
	n, err := strconv.Atoi(strings.TrimSpace(teamSize))
	if err != nil || n <= 0 {
		return true
	}
	return openJobs <= openJobsFloor || openJobs <= maxOpenJobsPerHead*n
}

// sameSite reports whether two website values can belong to one organisation. It
// answers true whenever either side is unknown: most catalogue companies carry no
// website, and a missing fact must not veto a match the name already made.
//
// Two known sites agree when their registrable domains share the label left of the
// public suffix — "stripe.com" and "stripe.co", or "careers.stripe.com" — and
// disagree otherwise. That is the case a name alone cannot see: Y Combinator's
// "Apollo" is apollographql.com and Accel's "Atlas" is theatlas.ai, neither of them
// the company the catalogue holds under that name.
//
// A label differing only by a corporate suffix still agrees — "yahooinc.com" is
// Yahoo, "redditinc.com" is Reddit. A leading "get", "use", "try" or "join" does NOT:
// that is the very shape two unrelated startups sharing a name take ("mesh.ai" and
// "usemesh.com"), so treating it as one company would undo the check where it
// matters most.
func sameSite(a, b string) bool {
	la, lb := siteLabel(a), siteLabel(b)
	if la == "" || lb == "" || la == lb {
		return true
	}
	return stripCorporate(la) == stripCorporate(lb)
}

// corporateSuffixes are the label endings that name the same company: the legal or
// group form a firm puts on its corporate site.
var corporateSuffixes = []string{"inc", "hq", "corp", "group", "global"}

func stripCorporate(label string) string {
	for _, suffix := range corporateSuffixes {
		if trimmed, ok := strings.CutSuffix(label, suffix); ok && trimmed != "" {
			return trimmed
		}
	}
	return label
}

// notACompanySite are registrable domains a stored "website" sometimes holds that
// belong to no one company: a job board's or ATS's hosted page, a link shortener, a
// template placeholder. Comparing against one would refuse a real member (Groupon's
// stored website is its Greenhouse board), so such a value counts as unknown. A host
// missing from this list only costs a refused match, which is the check's cautious
// direction, never a wrong one.
var notACompanySite = map[string]bool{
	"greenhouse.io":       true,
	"lever.co":            true,
	"ashbyhq.com":         true,
	"workable.com":        true,
	"smartrecruiters.com": true,
	"myworkdayjobs.com":   true,
	"recruitee.com":       true,
	"breezy.hr":           true,
	"linkedin.com":        true,
	"notion.site":         true,
	"bit.ly":              true,
	"linktr.ee":           true,
	"domain.com":          true,
}

// siteLabel reduces a website value to the label left of its public suffix
// ("www.acme.co.uk/jobs" → "acme"), or "" when the value is not a usable hostname or
// names no company of its own. Hyphens are dropped: "sunflower-labs.com" and
// "sunflowerlabs.com" are one site.
func siteLabel(website string) string {
	host := logodomain.Domain(website)
	if host == "" {
		return ""
	}
	registrable, err := publicsuffix.EffectiveTLDPlusOne(host)
	if err != nil || notACompanySite[registrable] {
		return ""
	}
	label, tld, _ := strings.Cut(registrable, ".")
	// A brand TLD is the brand: "about.google" is Google's own site.
	if tld == "google" {
		label = tld
	}
	return strings.ReplaceAll(label, "-", "")
}
