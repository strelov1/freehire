package pipeline

import (
	"errors"
	"regexp"
)

// errPlaceholderPosting is normalizeJob's answer for a posting an employer published to try its
// ATS rather than to hire: "Test", "Test Job 2", "DO NOT APPLY - TEST SC". The caller skips it
// like any posting that cannot become a job, so it is never refreshed and the board's sweep
// retires a stored copy. Measured on 2026-11-24: over 300 open canonical postings were such
// placeholders, on SmartRecruiters, Trakstar, BambooHR, Jobylon, Lever, Greenhouse and others.
var errPlaceholderPosting = errors.New("ingest: placeholder posting (a test, not a vacancy)")

var (
	// placeholderTitleRe is a title that is ONLY a test word, optionally with a posting noun
	// and a number. Anchored at both ends, so "Test Engineer" and "Testing Lead" stay jobs.
	placeholderTitleRe = regexp.MustCompile(`(?i)^\s*(test|testing|dummy|sample)(\s+(job|opening|posting|position|role|vacancy|listing|req|requisition))?\s*\d*\s*$`)
	// placeholderWarnRe and testWordRe together: a title telling applicants to stay away that
	// also calls itself a test. "Do not apply" alone is not enough — "DO NOT APPLY UNLESS YOU
	// HAVE INSURANCE LICENSE" is a real vacancy.
	placeholderWarnRe = regexp.MustCompile(`(?i)\b(do\s*n[o']?t\s+apply|please\s+ignore)\b`)
	testWordRe        = regexp.MustCompile(`(?i)\btest(ing)?\b`)
)

// placeholderTitle reports whether a posting's title says it is a test placeholder.
func placeholderTitle(title string) bool {
	return placeholderTitleRe.MatchString(title) ||
		(placeholderWarnRe.MatchString(title) && testWordRe.MatchString(title))
}
