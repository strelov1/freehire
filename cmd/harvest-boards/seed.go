package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"unicode"
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

// legalSuffixes are corporate-form words dropped from the tail of a company name before
// two names are compared. Ordered longest-first so the most specific one strips first.
var legalSuffixes = []string{
	" corporation", " limited", " gmbh", " corp", " llc", " ltd", " inc", " bv", " ab",
	" oy", " as", " co",
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

// normalizeEmployer folds a company name to its comparable core: lower-cased, punctuation
// reduced to word breaks, one trailing corporate form removed, and the words joined. The
// word breaks must survive until the suffix strip — collapsing "Derq, Inc." to "derqinc"
// first would glue the suffix onto the name and hide it.
func normalizeEmployer(name string) string {
	words := strings.FieldsFunc(strings.ToLower(name), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
	s := strings.Join(words, " ")
	for _, suf := range legalSuffixes {
		if strings.HasSuffix(s, suf) {
			s = strings.TrimSuffix(s, suf)
			break
		}
	}
	return strings.ReplaceAll(s, " ", "")
}

// reportedName returns the employer name the platform genuinely reports for a board, or ""
// when it reports none. Probers for platforms that expose no name echo the board id back as
// their fallback, so an answer equal to the board id is not a reported name — a distinction
// both the labelling and the gate depend on, which is why it lives in one place.
func reportedName(proberName, board string) string {
	if proberName == board {
		return ""
	}
	return proberName
}

// chooseCompany picks the board entry's company label. The prober's API-reported name wins
// when it is a real name (not just the board id echoed back by the slug fallback); otherwise
// a seed-supplied company fills in, and the board id is the last resort.
func chooseCompany(proberName, seedCompany, board string) string {
	if name := reportedName(proberName, board); name != "" {
		return name
	}
	if seedCompany != "" {
		return seedCompany
	}
	return board
}
