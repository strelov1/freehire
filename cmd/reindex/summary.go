package main

import (
	"html"
	"slices"
	"strings"
	"unicode"

	"github.com/strelov1/freehire/internal/job/jobdedup"
	"github.com/strelov1/freehire/internal/platform/db"
)

// Summary comparisons stay inside a known market, place, role family, level and language.
// Over-budget groups are not judged, preserving markers until they can be reconsidered.
func summaryBuckets(rows []db.FuzzyDedupCandidateTitlesForCompanyRow, eligible map[int64]bool) (
	map[string][]db.FuzzyDedupCandidateTitlesForCompanyRow, map[int64]bool,
) {
	groups := make(map[string][]db.FuzzyDedupCandidateTitlesForCompanyRow)
	for _, row := range rows {
		if !eligible[row.ID] {
			continue
		}
		if row.Source != "whatjobs" && !strings.HasPrefix(row.Source, "whatjobs-") {
			continue
		}
		if row.Location == "" || row.Category == "" || row.Seniority == "" || row.PostingLanguage == "" {
			continue
		}
		prefix := summaryRole(row.Title)
		if prefix == "" {
			continue
		}
		key := strings.Join([]string{row.Source, row.Location, row.Category, row.Seniority, row.PostingLanguage, prefix}, "\x1e")
		groups[key] = append(groups[key], row)
	}
	skipped := make(map[int64]bool)
	for key, group := range groups {
		if len(group) > jobdedup.FuzzyMaxBucket {
			for _, row := range group {
				skipped[row.ID] = true
			}
			delete(groups, key)
		} else if len(group) < 2 {
			delete(groups, key)
		}
	}
	return groups, skipped
}

func summaryRole(title string) string {
	words := strings.FieldsFunc(strings.ToLower(title), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) })
	if len(words) < 3 {
		return ""
	}
	return strings.Join(words[:3], " ")
}

// A summary must retain 80% of its vocabulary in a substantially fuller posting (at least
// 25 shared words). The captured KI-platform pair has 38/46, versus Jaccard's 38/91.
func summaryMatches(short, full map[string]struct{}) bool {
	if len(short) < 25 || float64(len(short)) > 0.6*float64(len(full)) {
		return false
	}
	shared := 0
	for word := range short {
		if _, ok := full[word]; ok {
			shared++
		}
	}
	return shared >= 25 && float64(shared) >= 0.8*float64(len(short))
}

func collapseSummaries(
	rows []db.FuzzyDedupCandidateTitlesForCompanyRow,
	signatures, evidence map[int64]map[string]struct{},
	moves map[int64]int64,
) {
	roots := make([]db.FuzzyDedupCandidateTitlesForCompanyRow, 0, len(rows))
	for _, row := range rows {
		if moves[row.ID] == 0 {
			roots = append(roots, row)
		}
	}
	// Full postings win over summaries, regardless of which was ingested first.
	slices.SortFunc(roots, func(a, b db.FuzzyDedupCandidateTitlesForCompanyRow) int {
		if n := len(signatures[b.ID]) - len(signatures[a.ID]); n != 0 {
			return n
		}
		if a.ID < b.ID {
			return -1
		}
		if a.ID > b.ID {
			return 1
		}
		return 0
	})
	for i, short := range roots {
		var target int64
		for _, full := range roots[:i] {
			if moves[full.ID] != 0 || !summaryMatches(signatures[short.ID], signatures[full.ID]) {
				continue
			}
			if !summaryTitleEvidence(
				short.Title,
				full.Title,
				evidence[short.ID],
				evidence[full.ID],
			) {
				continue
			}
			if target != 0 {
				target = 0 // Two plausible full postings: no identity verdict.
				break
			}
			target = full.ID
		}
		if target != 0 {
			moves[short.ID] = target
		}
	}
}

// Generic role words and employer boilerplate cannot establish identity: both bodies
// must corroborate at least two title details beyond their common role prefix.
func summaryTitleEvidence(short, full string, shortBody, fullBody map[string]struct{}) bool {
	details := titleWords(short + " " + full)
	for word := range titleWords(summaryRole(short)) {
		delete(details, word)
	}
	corroborated := 0
	for word := range details {
		_, inShort := shortBody[word]
		_, inFull := fullBody[word]
		if inShort && inFull && strings.ContainsFunc(word, unicode.IsLetter) {
			corroborated++
		}
	}
	return corroborated >= 2 && titleInDescription(short, full, fullBody) &&
		titleInDescription(full, short, shortBody)
}

func titleInDescription(title, otherTitle string, body map[string]struct{}) bool {
	other := titleWords(otherTitle)
	for word := range titleWords(title) {
		if _, ok := other[word]; ok {
			continue
		}
		if _, ok := body[word]; !ok {
			return false
		}
	}
	return true
}

func titleWords(title string) map[string]struct{} {
	words := strings.FieldsFunc(strings.ToLower(html.UnescapeString(title)), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '+' && r != '#'
	})
	out := make(map[string]struct{}, len(words))
	for _, word := range words {
		switch word {
		case "and", "for", "the", "with", "und", "mit", "ref":
			continue
		}
		if (len(word) < 2 && word != "c" && word != "r") || !strings.ContainsFunc(word, func(r rune) bool {
			return unicode.IsLetter(r) || unicode.IsDigit(r)
		}) {
			continue
		}
		out[word] = struct{}{}
	}
	return out
}
