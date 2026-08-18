// Package mailmatch resolves an inbox email to one of the caller's own
// applications using deterministic signals (thread continuity and the company
// name carried in the sender name / subject), leaving the probabilistic tail to
// an LLM caller. It never matches on the sender-address domain: inbox mail
// arrives from ATS relay domains (ashbyhq.com, greenhouse-mail.io, …), not from
// employer domains.
package mailmatch

import (
	"strings"
	"unicode"

	"github.com/strelov1/freehire/internal/normalize"
)

// atsPseudoNames are ATS platform brand names that surface where a company name
// is expected ("Thank you for applying to Greenhouse!", "Your Workday
// Application"). They are not employers and must never be treated as a company.
//
// Blocking a name here is deliberately lossy: some of these brands are also real
// employers people apply to, and their own mail will no longer auto-link. That
// asymmetry is the point. A missed auto-link degrades to an LLM suggestion the
// caller confirms — a moment's work. A wrong auto-link silently hands one
// application everyone else's mail, and the damage compounds: on a live mailbox
// `workable` was absent from this list, and one catalog company named Workable
// collected 23 acknowledgements meant for 23 other employers, leaving it
// permanently unable to look silent.
//
// The list is sender *display names*, not the provider slugs in sources/ — a new
// board file does not automatically belong here, and a brand that never appears
// as a display name does not need to be. When adding an ATS board, ask whether
// its relay signs mail with its own brand.
var atsPseudoNames = map[string]bool{
	"greenhouse": true, "workday": true, "myworkday": true, "lever": true,
	"ashby": true, "smartrecruiters": true, "teamtailor": true, "recruitee": true,
	"icims": true, "gem": true, "eightfold": true, "rippling": true,
	"bamboohr": true, "wellfound": true, "workable": true, "jobvite": true,
	"taleo": true, "breezy": true, "breezyhr": true, "jazzhr": true,
	"comeet": true, "personio": true, "pinpoint": true, "freshteam": true,
	"zoho recruit": true, "zohorecruit": true, "manatal": true, "traffit": true,
	"phenom": true, "avature": true, "cornerstone": true, "successfactors": true,
	"bullhorn": true, "jobylon": true, "loxo": true, "neogov": true,
	"softgarden": true, "talentlyft": true, "trakstar": true, "applicantpro": true,
	"catsone": true, "crelate": true, "jobscore": true, "hireology": true,
}

// nameSuffixes are the recruiting-team suffixes ATS "from" names carry.
// Ordered longest-first so the most specific suffix strips first.
var nameSuffixes = []string{
	" recruiting team", " talent acquisition team", " talent acquisition",
	" hiring team", " talent team", " recruiting", " careers", " - workday",
	" workday", " team",
}

// stripLegalForm drops one trailing corporate form from an already-lowercased company name.
//
// The vocabulary is normalize's, not this package's. Mail matching exists to reach a company
// the catalogue holds, and the catalogue keys companies by normalize.CompanySlug — so a form
// this stripped and that did not (or the reverse) produced a name matching nothing, and a mail
// that matches no company links to no application without saying so.
// It repeats, and it splits on punctuation as well as space, for the two shapes senders
// actually use: "Acme GmbH & Co. KG" needs three passes over an ampersand, and
// "Sun Technologies,Inc." attaches the form with no space at all — 13,730 catalogue
// companies are written that way. A name of one word is never stripped, so a sender called
// "Limited" survives.
func stripLegalForm(s string) string {
	for {
		trimmed := trimTrailingPunct(s)
		i := strings.LastIndexFunc(trimmed, func(r rune) bool {
			return !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '.' && r != '/'
		})
		if i <= 0 || !normalize.IsLegalForm(trimmed[i+1:]) {
			return trimmed
		}
		s = trimmed[:i]
	}
}

// subjectPrefixes are the templated subject openers that name the company next.
var subjectPrefixes = []string{
	"thank you for applying to ",
	"thanks for applying to ",
	"thank you for your application to ",
	"thank you for your interest in ",
	"your application to ",
}

// ExtractCompany returns a normalized (lowercased) company name carried by the
// email's sender name or subject, or "" when none can be resolved or the name is
// an ATS pseudo-name. The sender name is preferred over the subject.
func ExtractCompany(fromName, subject string) string {
	if c := fromSenderName(fromName); c != "" {
		return c
	}
	return fromSubject(subject)
}

func fromSenderName(fromName string) string {
	s := strings.ToLower(strings.TrimSpace(fromName))
	if s == "" {
		return ""
	}
	// Trim trailing punctuation ("Acme Inc.", "Sardine Hiring Team,") before
	// stripping suffixes, so a trailing period/comma can't hide the suffix.
	s = trimTrailingPunct(s)
	s = stripFirstSuffix(s, nameSuffixes)
	s = stripLegalForm(s)
	return cleanCompany(s)
}

// stripFirstSuffix removes the first matching suffix from s (suffixes are
// ordered longest-first by their callers), or returns s unchanged.
func stripFirstSuffix(s string, suffixes []string) string {
	for _, suf := range suffixes {
		if strings.HasSuffix(s, suf) {
			return strings.TrimSuffix(s, suf)
		}
	}
	return s
}

func fromSubject(subject string) string {
	s := strings.ToLower(strings.TrimSpace(subject))
	if strings.HasPrefix(s, "your ") && strings.HasSuffix(s, " application") {
		mid := strings.TrimSuffix(strings.TrimPrefix(s, "your "), " application")
		return cleanCompany(mid)
	}
	for _, p := range subjectPrefixes {
		if strings.HasPrefix(s, p) {
			return cleanCompany(strings.TrimPrefix(s, p))
		}
	}
	return ""
}

// cleanCompany trims trailing non-alphanumeric noise (punctuation, emoji) and
// surrounding space, then drops the value if it is an ATS pseudo-name.
func cleanCompany(s string) string {
	s = trimTrailingPunct(s)
	if s == "" || atsPseudoNames[s] {
		return ""
	}
	return s
}

// trimTrailingPunct drops trailing non-alphanumeric runes (punctuation, emoji)
// and surrounding whitespace.
func trimTrailingPunct(s string) string {
	s = strings.TrimRightFunc(s, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
	return strings.TrimSpace(s)
}
