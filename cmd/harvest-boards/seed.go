package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

// seedItem is one candidate board from a seed file. A seed may be a plain JSON array of
// board tokens (Company empty) or an array of {board, company} objects — the latter lets a
// discovery source that already knows the employer (e.g. harvest-role, which reads it from
// role.com's JSON-LD) supply a name for providers whose own API exposes none (Oracle).
type seedItem struct {
	Board   string `json:"board"`
	Company string `json:"company"`
}

// loadSeedItems reads a seed file in either supported shape: a JSON array of strings or a
// JSON array of {board, company} objects.
func loadSeedItems(path string) ([]seedItem, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read seed %s: %w", path, err)
	}
	var strs []string
	if json.Unmarshal(data, &strs) == nil {
		items := make([]seedItem, len(strs))
		for i, s := range strs {
			items[i] = seedItem{Board: s}
		}
		return items, nil
	}
	var items []seedItem
	if err := json.Unmarshal(data, &items); err != nil {
		return nil, fmt.Errorf("parse seed %s: %w", path, err)
	}
	return items, nil
}

// legalSuffixes are corporate-form words dropped from the tail of a company name before two
// names are compared. Ordered longest-first so the most specific one strips first. The
// single-letter entries (" a s", " s a") are the punctuated Nordic and Romance forms after
// punctuation has already been reduced to word breaks: "Trafalgar A/S" arrives here as
// "trafalgar a s".
var legalSuffixes = []string{
	" corporation", " limited", " gmbh", " corp", " llc", " ltd", " inc", " plc", " llp",
	" srl", " pty", " a s", " s a", " bv", " nv", " ab", " ag", " kg", " oy", " sa",
	" as", " co",
}

// sameEmployer reports whether two company names denote the same employer. It is the gate
// that keeps a live board belonging to somebody else out of the board file: a name-derived
// candidate slug regularly resolves to an unrelated tenant, and "the board answers with
// jobs" alone cannot tell the two apart.
//
// The comparison is deliberately mild — case, punctuation and a trailing corporate form are
// noise between an aggregator's label and an ATS's own. It is equally deliberately not
// fuzzy: substring or prefix matching would admit every tenant whose name happens to contain
// a short common word, which is the collision this gate exists to catch.
func sameEmployer(expected, reported string) bool {
	a, b := normalizeEmployer(expected), normalizeEmployer(reported)
	return a != "" && a == b
}

// normalizeEmployer folds a company name to its comparable core: lower-cased, diacritics
// dropped, punctuation reduced to word breaks, trailing corporate forms removed, and the
// words joined. The word breaks must survive until the suffix strip — collapsing "Derq,
// Inc." to "derqinc" first would glue the suffix onto the name and hide it.
//
// Stripping repeats rather than stopping at one match: compound forms are ordinary ("Acme
// GmbH & Co. KG", "Atlassian Pty Ltd"), and a single pass would leave half the form behind
// and report a match as a mismatch.
func normalizeEmployer(name string) string {
	folded := strings.Map(func(r rune) rune {
		if unicode.Is(unicode.Mn, r) {
			return -1
		}
		return r
	}, norm.NFD.String(strings.ToLower(name)))

	words := strings.FieldsFunc(folded, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
	s := strings.Join(words, " ")
	for stripped := true; stripped; {
		stripped = false
		for _, suf := range legalSuffixes {
			if strings.HasSuffix(s, suf) {
				s, stripped = strings.TrimSuffix(s, suf), true
				break
			}
		}
	}
	return strings.ReplaceAll(s, " ", "")
}

// chooseCompany picks the board entry's company label. A prober returns "" when the platform
// publishes no employer name of its own (the contract every prober follows), so a non-empty
// name is always the platform's own and wins; otherwise a seed-supplied company fills in, and
// the board id is the last resort.
func chooseCompany(proberName, seedCompany, board string) string {
	if proberName != "" {
		return proberName
	}
	if seedCompany != "" {
		return seedCompany
	}
	return board
}
