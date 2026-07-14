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
)

// atsPseudoNames are ATS platform brand names that surface where a company name
// is expected ("Thank you for applying to Greenhouse!", "Your Workday
// Application"). They are not employers and must never be treated as a company.
var atsPseudoNames = map[string]bool{
	"greenhouse": true, "workday": true, "myworkday": true, "lever": true,
	"ashby": true, "smartrecruiters": true, "teamtailor": true, "recruitee": true,
	"icims": true, "gem": true, "eightfold": true, "rippling": true,
	"bamboohr": true, "wellfound": true,
}

// nameSuffixes are the recruiting-team suffixes ATS "from" names carry.
// Ordered longest-first so the most specific suffix strips first.
var nameSuffixes = []string{
	" recruiting team", " talent acquisition team", " talent acquisition",
	" hiring team", " talent team", " recruiting", " careers", " - workday",
	" workday", " team",
}

// legalSuffixes are corporate-form suffixes to drop from a company name.
var legalSuffixes = []string{" llc", " inc", " ltd", " gmbh", " corp", " co"}

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
	s = stripFirstSuffix(s, legalSuffixes)
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
