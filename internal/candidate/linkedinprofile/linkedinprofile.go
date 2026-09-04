// Package linkedinprofile reads a public LinkedIn member profile and returns the
// fields LinkedIn actually releases to an anonymous reader.
//
// LinkedIn serves a member profile to anyone, but returns most of its text as runs
// of asterisks that preserve the original string's length — every job title and
// every position description arrives that way. What survives is the headline, the
// address, the languages, the display name and the first employer. This package
// exists to lift exactly that, and to make it impossible for a masked run to leave
// here looking like a value.
//
// It runs no dictionary and knows no vocabulary: the caller derives facets from the
// raw strings, through the same helpers the CV path already uses, so that a profile
// and a CV carrying the same words can never resolve differently.
package linkedinprofile
