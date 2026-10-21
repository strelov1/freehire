package collections

import (
	"strings"

	"golang.org/x/net/publicsuffix"

	"github.com/strelov1/freehire/internal/job/logodomain"
)

// MetaWebsite is the Record.Meta key a source fills with the member's own website,
// when it publishes one. Members reads it to refuse a name match whose company
// demonstrably runs a different site.
const MetaWebsite = "website"

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
